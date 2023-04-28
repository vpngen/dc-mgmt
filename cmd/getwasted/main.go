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
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	defaultFirstVisitDaysLimit  = 1
	defaultActiveUsersDaysLimit = 27
	defaultMinActiveUsers       = 5
	defaultMaxResultRows        = 10
)

const (
	CommandNotVisited = "novisited"
	CommandInactive   = "inactive"
)

const (
	sqlGetNotVisited = `
	SELECT 
		brigade_id
	FROM 
		%s
	WHERE
		created_at < now() - ($1 * INTERVAL '1 days') 
	AND
		total_users_count=1
	AND 
		first_visit IS NULL
	ORDER BY 
		created_at ASC
	LIMIT $2::int
	`
	sqlGetInactive = `
	SELECT 
		brigade_id
	FROM 
		%s
	WHERE
		created_at < now() - ($1 * INTERVAL '1 days') 
	AND 
		first_visit IS NULL
	ORDER BY 
		created_at ASC
	LIMIT $3::int
	`
)

var errInlalidArgs = errors.New("invalid args")

func main() {
	var w io.WriteCloser

	executable, _ := os.Executable()
	exe := filepath.Base(executable)

	chunked, cmd, days, num, _, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", exe, err)
	}

	dbURL, schema, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", exe, err)
	}

	db, err := createDBPool(dbURL)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", exe, err)
	}

	var output []byte

	switch cmd {
	case CommandNotVisited:
		output, err = getWasted(db, schema, days, num)
		if err != nil {
			log.Fatalf("%s: Can't get brigades: %s\n", exe, err)
		}
	case CommandInactive:
		if time.Now().Day() != 1 {
			fmt.Fprintf(os.Stderr, "WARNING!!! This command should be run on the first day of the month\n")
		}
	default:
		log.Fatalf("%s: Unknown command: %s\n", exe, cmd)
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
		log.Fatalf("%s: Can't print output: %s\n", exe, err)
	}
}

func getWasted(db *pgxpool.Pool, schema string, days, num int) ([]byte, error) {
	ctx := context.Background()
	output := []byte{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	rows, err := tx.Query(ctx,
		fmt.Sprintf(sqlGetNotVisited, (pgx.Identifier{"stats", "brigades_stats"}.Sanitize())), // !!!!
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

	err = tx.Commit(ctx)
	if err != nil {
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

func parseArgs() (bool, string, int, int, int, error) {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s notvisited|inactive [options]\n", os.Args[0])
		flag.PrintDefaults()
	}

	chunked := flag.Bool("ch", false, "chunked output")
	flag.Parse()
	if len(flag.Args()) < 1 {
		return false, "", 0, 0, 0, fmt.Errorf("no command specified")
	}

	switch flag.Args()[0] {
	case CommandNotVisited:
		notVisitedFlags := flag.NewFlagSet(CommandNotVisited, flag.ExitOnError)
		days := notVisitedFlags.Int("d", defaultFirstVisitDaysLimit, "days limit to first visit")
		num := notVisitedFlags.Int("n", defaultMaxResultRows, "how many max rows will return")
		notVisitedFlags.Usage = func() {
			fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s [options]\n", CommandNotVisited, os.Args[0])
			notVisitedFlags.PrintDefaults()
		}

		notVisitedFlags.Parse(flag.Args()[1:])

		if *num < 1 || *days < 1 {
			return false, "", 0, 0, 0, fmt.Errorf("num/days: %w", errInlalidArgs)
		}

		return *chunked, CommandNotVisited, *days, *num, 0, nil
	case CommandInactive:
		inactiveFlags := flag.NewFlagSet(CommandInactive, flag.ExitOnError)
		days := inactiveFlags.Int("d", defaultActiveUsersDaysLimit, "days limit from registration")
		x := inactiveFlags.Int("x", defaultMinActiveUsers, "minmium active users count for live")
		num := inactiveFlags.Int("n", defaultMaxResultRows, "how many max rows will return")
		inactiveFlags.Usage = func() {
			fmt.Fprintf(flag.CommandLine.Output(), "usage: %s %s [options]\n", CommandInactive, os.Args[0])
			inactiveFlags.PrintDefaults()
		}

		inactiveFlags.Parse(flag.Args()[1:])

		if *num < 1 || *x < 1 {
			return false, "", 0, 0, 0, fmt.Errorf("num/x: %w", errInlalidArgs)
		}

		return *chunked, CommandInactive, *days, *num, *x, nil
	default:
		return false, "", 0, 0, 0, fmt.Errorf("unknown command: %w", errInlalidArgs)
	}
}

func readConfigs() (string, string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	brigadesStatsSchema := os.Getenv("BRIGADES_STATS_SCHEMA")
	if brigadesStatsSchema == "" {
		brigadesStatsSchema = defaultBrigadesStatsSchema
	}
	return dbURL, brigadesStatsSchema, nil
}
