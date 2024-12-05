package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	defaultFirstVisitDaysLimit        = 1
	defaultActiveCreatedAtMonthsLimit = 1
	defaultMinActiveUsers             = 5
	defaultLastSeenDaysLimit          = 7
	defaultMaxResultRows              = 10
)

const (
	CommandNotVisited = "notvisited"
	CommandInactive   = "inactive"
	CommandNotUsed    = "notused"
)

const updateTimeFreshness = 2 // hours

var errInlalidArgs = errors.New("invalid args")

var LogTag = setLogTag()

const defaultLogTag = "getwasted"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	var w io.WriteCloser

	chunked, igrp, cmd, days, months, num, x, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	dbURL, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbURL)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	var output []byte

	switch cmd {
	case CommandNotVisited:
		output, err = getNotVisited(db, igrp, days, num)
		if err != nil {
			log.Fatalf("%s: Can't get brigades: %s\n", LogTag, err)
		}
	case CommandInactive:
		if time.Now().Day() != 1 {
			fmt.Fprintf(os.Stderr, "WARNING!!! This command should be run on the first day of the month\n")
		}

		output, err = getInactive(db, igrp, months, num, x)
		if err != nil {
			log.Fatalf("%s: Can't get brigades: %s\n", LogTag, err)
		}
	case CommandNotUsed:
		output, err = getNotUsed(db, igrp, x, days, num)
		if err != nil {
			log.Fatalf("%s: Can't get brigades: %s\n", LogTag, err)
		}
	default:
		log.Fatalf("%s: Unknown command: %s\n", LogTag, cmd)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	if output == nil {
		output = []byte{}
	}

	_, err = w.Write(output)
	if err != nil {
		log.Fatalf("%s: Can't print output: %s\n", LogTag, err)
	}
}

// getInactive - returns list of inactive brigades.
func getInactive(db *pgxpool.Pool, igrp bool, months, num, min int) ([]byte, error) {
	t := time.Now().UTC()
	firstDayOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	maxCreatedAt := firstDayOfMonth.AddDate(0, -months, 0)

	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	sqlGetInactive := `
	SELECT 
		bs.brigade_id, p.igrp_id
	FROM 
		stats.brigades_stats bs
	JOIN 
		brigades.brigades b ON bs.brigade_id = b.brigade_id
	JOIN 
		pairs.pairs AS p ON b.pair_id = p.pair_id
	LEFT JOIN
		brigades.brigades b2 ON b.brigade_id = b2.brigade_id AND b2.main = false
	LEFT JOIN 
		brigades.reserved_endpoints_ipv4 AS rei ON b.endpoint_ipv4 = rei.endpoint_ipv4
	WHERE
		(
			(bs.update_time > now() - ($1 * INTERVAL '1 days'))
		OR
			-- it's for resolve corrupted brigade deletion
			((bs.update_time < now() - ($1 * INTERVAL '1 days')) AND (bs.update_time>=$2))
		)
	AND
		(bs.created_at < $3 OR bs.first_visit < $3) -- it's for resolve migrated brigades
	AND 
		bs.active_users_count < $4::int
	AND
		b.main = true
	AND
		rei.endpoint_ipv4 IS NULL
	AND
		b2.brigade_id IS NULL
	ORDER BY 
		p.igrp_id ASC,
		bs.created_at ASC
	LIMIT $5::int
	`
	rows, err := tx.Query(ctx,
		sqlGetInactive,
		updateTimeFreshness,
		firstDayOfMonth,
		maxCreatedAt,
		min,
		num,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigades query: %w", err)
	}

	// lock on brigades, register used nets

	var (
		id     string
		igrpID pgtype.UUID
	)

	output := []byte{}

	_, err = pgx.ForEachRow(rows, []any{&id, &igrpID}, func() error {
		if !igrp {
			output = fmt.Appendln(output, id)

			return nil
		}

		output = fmt.Appendln(output, id+";"+uuid.UUID(igrpID.Bytes).String())

		return nil
	})
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigade row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return output, nil
}

func getNotVisited(db *pgxpool.Pool, igrp bool, days, num int) ([]byte, error) {
	ctx := context.Background()
	output := []byte{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	sqlGetNotVisited := `
	SELECT 
		bs.brigade_id, p.igrp_id
	FROM 
		%s bs
	JOIN 
		%s b ON bs.brigade_id = b.brigade_id
	JOIN 
		pairs.pairs AS p ON b.pair_id = p.pair_id
	LEFT JOIN
		%s b2 ON b.brigade_id = b2.brigade_id AND b2.main = false
	LEFT JOIN 
		%s AS rei ON b.endpoint_ipv4 = rei.endpoint_ipv4
	WHERE
		bs.update_time > now() - ($1 * INTERVAL '1 hours')
	AND
		bs.created_at < now() - ($2 * INTERVAL '1 days') 
	AND
		bs.total_users_count=1
	AND 
		bs.first_visit IS NULL
	AND
		rei.endpoint_ipv4 IS NULL
	AND
		b.main = true
	AND
		b2.brigade_id IS NULL
	ORDER BY 
		p.igrp_id ASC,
		bs.created_at ASC
	LIMIT $3::int
	`
	rows, err := tx.Query(ctx,
		fmt.Sprintf(sqlGetNotVisited,
			pgx.Identifier{defaultBrigadesStatsSchema, "brigades_stats"}.Sanitize(),      // !!!!
			pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize(),                 // !!!!
			pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize(),                 // !!!!
			pgx.Identifier{defaultBrigadesSchema, "reserved_endpoints_ipv4"}.Sanitize()), // !!!!
		updateTimeFreshness,
		days,
		num,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigades query: %w", err)
	}

	// lock on brigades, register used nets

	var (
		id     string
		igrpID pgtype.UUID
	)

	_, err = pgx.ForEachRow(rows, []any{&id, &igrpID}, func() error {
		if !igrp {
			output = fmt.Appendln(output, id)

			return nil
		}

		output = fmt.Appendln(output, id+";"+uuid.UUID(igrpID.Bytes).String())

		return nil
	})
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigade row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return output, nil
}

func getNotUsed(db *pgxpool.Pool, igrp bool, users, days, num int) ([]byte, error) {
	ctx := context.Background()
	output := []byte{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	sqlGetNotUsed := `
	SELECT 
		bs.brigade_id, p.igrp_id
	FROM 
		%s bs
	JOIN 
		%s b ON bs.brigade_id = b.brigade_id
	JOIN 
		pairs.pairs AS p ON b.pair_id = p.pair_id
	LEFT JOIN
		%s b2 ON b.brigade_id = b2.brigade_id AND b2.main = false
	LEFT JOIN 
		%s AS rei ON b.endpoint_ipv4 = rei.endpoint_ipv4
	WHERE
		bs.update_time > now() - ($1 * INTERVAL '1 hours')
	AND
		(bs.last_seen IS NOT NULL AND bs.last_seen < now() - ($2 * INTERVAL '1 days'))
	AND
		bs.total_users_count<$4
	AND
		rei.endpoint_ipv4 IS NULL
	AND
		b.main = true
	AND
		b2.brigade_id IS NULL
	ORDER BY 
		p.igrp_id ASC,
		bs.created_at ASC
	LIMIT $3::int
	`
	rows, err := tx.Query(ctx,
		fmt.Sprintf(sqlGetNotUsed,
			pgx.Identifier{defaultBrigadesStatsSchema, "brigades_stats"}.Sanitize(),      // !!!!
			pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize(),                 // !!!!
			pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize(),                 // !!!!
			pgx.Identifier{defaultBrigadesSchema, "reserved_endpoints_ipv4"}.Sanitize()), // !!!!
		updateTimeFreshness,
		days,
		num,
		users,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigades query: %w", err)
	}

	// lock on brigades, register used nets

	var (
		id     string
		igrpID pgtype.UUID
	)

	_, err = pgx.ForEachRow(rows, []any{&id, &igrpID}, func() error {
		if !igrp {
			output = fmt.Appendln(output, id)

			return nil
		}

		output = fmt.Appendln(output, id+";"+uuid.UUID(igrpID.Bytes).String())

		return nil
	})
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigade row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return output, nil
}

func createDBPool(dbURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("conn string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return pool, nil
}

func parseArgs() (bool, bool, string, int, int, int, int, error) {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s|%s [options]\n", os.Args[0], CommandNotVisited, CommandInactive)
		flag.PrintDefaults()
	}

	chunked := flag.Bool("ch", false, "chunked output")
	flag.Parse()
	if len(flag.Args()) < 1 {
		return false, false, "", 0, 0, 0, 0, fmt.Errorf("no command specified")
	}

	switch flag.Args()[0] {
	case CommandNotVisited:
		notVisitedFlags := flag.NewFlagSet(CommandNotVisited, flag.ExitOnError)
		days := notVisitedFlags.Int("d", defaultFirstVisitDaysLimit, "days limit to first visit")
		num := notVisitedFlags.Int("n", defaultMaxResultRows, "how many max rows will return")
		igrp := notVisitedFlags.Bool("igrp", false, "use isolated groups")
		notVisitedFlags.Usage = func() {
			fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s [options]\n", os.Args[0], CommandNotVisited)
			notVisitedFlags.PrintDefaults()
		}

		notVisitedFlags.Parse(flag.Args()[1:])

		if *num < 1 || *days < 1 {
			return false, false, "", 0, 0, 0, 0, fmt.Errorf("num/days: %w", errInlalidArgs)
		}

		return *chunked, *igrp, CommandNotVisited, *days, 0, *num, 0, nil
	case CommandInactive:
		inactiveFlags := flag.NewFlagSet(CommandInactive, flag.ExitOnError)
		months := inactiveFlags.Int("m", defaultActiveCreatedAtMonthsLimit, "months limit from registration")
		x := inactiveFlags.Int("x", defaultMinActiveUsers, "minmium active users count for live")
		num := inactiveFlags.Int("n", defaultMaxResultRows, "how many max rows will return")
		igrp := inactiveFlags.Bool("igrp", false, "use isolated groups")
		inactiveFlags.Usage = func() {
			fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s [options]\n", os.Args[0], CommandInactive)
			inactiveFlags.PrintDefaults()
		}

		inactiveFlags.Parse(flag.Args()[1:])

		if *num < 1 || *x < 1 || *months < 1 {
			return false, false, "", 0, 0, 0, 0, fmt.Errorf("num/x: %w", errInlalidArgs)
		}

		return *chunked, *igrp, CommandInactive, 0, *months, *num, *x, nil
	case CommandNotUsed:
		notusedFlags := flag.NewFlagSet(CommandNotUsed, flag.ExitOnError)
		days := notusedFlags.Int("d", defaultLastSeenDaysLimit, "days limit to last seen")
		x := notusedFlags.Int("x", defaultMinActiveUsers, "minmium active users count for live")
		num := notusedFlags.Int("n", defaultMaxResultRows, "how many max rows will return")
		igrp := notusedFlags.Bool("igrp", false, "use isolated groups")
		notusedFlags.Usage = func() {
			fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s [options]\n", os.Args[0], CommandNotUsed)
			notusedFlags.PrintDefaults()
		}

		if *x == 1 {
			*x = defaultMinActiveUsers
		}

		notusedFlags.Parse(flag.Args()[1:])

		if *num < 1 || *x < 1 || *days < 1 {
			return false, false, "", 0, 0, 0, 0, fmt.Errorf("num/x: %w", errInlalidArgs)
		}

		return *chunked, *igrp, CommandNotUsed, *days, 0, *num, *x, nil
	default:
		return false, false, "", 0, 0, 0, 0, fmt.Errorf("unknown command: %w", errInlalidArgs)
	}
}

func readConfigs() (string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	return dbURL, nil
}
