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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	defaultFirstVisitDaysLimit        = 1
	defaultActiveCreatedAtMonthsLimit = 1
	defaultMinActiveUsers             = 5
	defaultMaxResultRows              = 10
)

const (
	CommandNotVisited = "notvisited"
	CommandInactive   = "inactive"
)

const updateTimeFreshness = 1 // hours

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

	chunked, cmd, days, months, num, _, err := parseArgs()
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
		output, err = getNotVisited(db, days, num)
		if err != nil {
			log.Fatalf("%s: Can't get brigades: %s\n", LogTag, err)
		}
	case CommandInactive:
		if time.Now().Day() != 1 {
			fmt.Fprintf(os.Stderr, "WARNING!!! This command should be run on the first day of the month\n")
		}

		output, err = getInactive(db, months, num, defaultMinActiveUsers)
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
func getInactive(db *pgxpool.Pool, months, num, min int) ([]byte, error) {
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
		brigade_id
	FROM 
		%s
	WHERE
		(update_time > now() - ($1 * INTERVAL '1 days'))
	OR
		-- it's for resolve corrupted brigade deletion
		((update_time < now() - ($1 * INTERVAL '1 days')) AND (update_time>=$2))
	AND
		created_at < $3
	AND 
		active_users_count < $4::int
	ORDER BY 
		created_at ASC
	LIMIT $4::int
	`
	rows, err := tx.Query(ctx,
		fmt.Sprintf(sqlGetInactive, (pgx.Identifier{"stats", "brigades_stats"}.Sanitize())), // !!!!
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

	var id string

	output := []byte{}

	_, err = pgx.ForEachRow(rows, []any{&id}, func() error {
		output = fmt.Appendln(output, id)

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

func getNotVisited(db *pgxpool.Pool, days, num int) ([]byte, error) {
	ctx := context.Background()
	output := []byte{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	sqlGetNotVisited := `
	SELECT 
		brigade_id
	FROM 
		%s
	WHERE
		update_time > now() - ($1 * INTERVAL '1 hours')
	AND
		created_at < now() - ($2 * INTERVAL '1 days') 
	AND
		total_users_count=1
	AND 
		first_visit IS NULL
	ORDER BY 
		created_at ASC
	LIMIT $3::int
	`
	rows, err := tx.Query(ctx,
		fmt.Sprintf(sqlGetNotVisited, (pgx.Identifier{"stats", "brigades_stats"}.Sanitize())), // !!!!
		updateTimeFreshness,
		days,
		num,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigades query: %w", err)
	}

	// lock on brigades, register used nets

	var id string

	_, err = pgx.ForEachRow(rows, []any{&id}, func() error {
		output = fmt.Appendln(output, id)

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

func parseArgs() (bool, string, int, int, int, int, error) {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s|%s [options]\n", os.Args[0], CommandNotVisited, CommandInactive)
		flag.PrintDefaults()
	}

	chunked := flag.Bool("ch", false, "chunked output")
	flag.Parse()
	if len(flag.Args()) < 1 {
		return false, "", 0, 0, 0, 0, fmt.Errorf("no command specified")
	}

	switch flag.Args()[0] {
	case CommandNotVisited:
		notVisitedFlags := flag.NewFlagSet(CommandNotVisited, flag.ExitOnError)
		days := notVisitedFlags.Int("d", defaultFirstVisitDaysLimit, "days limit to first visit")
		num := notVisitedFlags.Int("n", defaultMaxResultRows, "how many max rows will return")
		notVisitedFlags.Usage = func() {
			fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s [options]\n", os.Args[0], CommandNotVisited)
			notVisitedFlags.PrintDefaults()
		}

		notVisitedFlags.Parse(flag.Args()[1:])

		if *num < 1 || *days < 1 {
			return false, "", 0, 0, 0, 0, fmt.Errorf("num/days: %w", errInlalidArgs)
		}

		return *chunked, CommandNotVisited, *days, 0, *num, 0, nil
	case CommandInactive:
		inactiveFlags := flag.NewFlagSet(CommandInactive, flag.ExitOnError)
		months := inactiveFlags.Int("m", defaultActiveCreatedAtMonthsLimit, "months limit from registration")
		x := inactiveFlags.Int("x", defaultMinActiveUsers, "minmium active users count for live")
		num := inactiveFlags.Int("n", defaultMaxResultRows, "how many max rows will return")
		inactiveFlags.Usage = func() {
			fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s [options]\n", os.Args[0], CommandInactive)
			inactiveFlags.PrintDefaults()
		}

		inactiveFlags.Parse(flag.Args()[1:])

		if *num < 1 || *x < 1 {
			return false, "", 0, 0, 0, 0, fmt.Errorf("num/x: %w", errInlalidArgs)
		}

		return *chunked, CommandInactive, 0, *months, *num, *x, nil
	default:
		return false, "", 0, 0, 0, 0, fmt.Errorf("unknown command: %w", errInlalidArgs)
	}
}

func readConfigs() (string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	return dbURL, nil
}
