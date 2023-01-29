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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	dbnameFilename     = "dbname"
	schemaNameFilename = "schema" // shit!!!!
	etcDefaultPath     = "/etc/vgrealm"
)

const (
	maxPostgresqlNameLen = 63
	postgresqlSocket     = "/var/run/postgresql"
)

const (
	defaultFirstVisitLimit = 3
	defaultMaxResultRows   = 10
)

const (
	sqlGetWasted = `
	SELECT 
		brigade_id
	FROM 
		%s
	WHERE
		create_at < now() - ($1 * INTERVAL '1 days') 
	AND
		user_count=1
	AND 
		last_visit IS NULL
	ORDER BY 
		create_at ASC
	LIMIT $2::int
	`
)

var errInlalidArgs = errors.New("invalid args")

func main() {
	var w io.WriteCloser

	confDir := os.Getenv("CONFDIR")
	if confDir == "" {
		confDir = etcDefaultPath
	}

	executable, _ := os.Executable()
	exe := filepath.Base(executable)

	chunked, days, num, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", exe, err)
	}

	dbname, schema, err := readConfigs(confDir)
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", exe, err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", exe, err)
	}

	output, err := getWasted(db, schema, days, num)
	if err != nil {
		log.Fatalf("%s: Can't get brigades: %s\n", exe, err)
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
		fmt.Sprintf(sqlGetWasted, (pgx.Identifier{"stats", "brigades_stats"}.Sanitize())), // !!!!
		days,
		num,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigades query: %w", err)
	}

	// lock on brigades, register used nets

	var (
		id string
	)

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

func createDBPool(dbname string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(fmt.Sprintf("host=%s dbname=%s", postgresqlSocket, dbname))
	if err != nil {
		return nil, fmt.Errorf("conn string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return pool, nil
}

func parseArgs() (bool, int, int, error) {
	days := flag.Int("d", defaultFirstVisitLimit, "days limit to first visit")
	num := flag.Int("n", defaultMaxResultRows, "how many max rows will return")
	chunked := flag.Bool("ch", false, "chunked output")

	flag.Parse()

	if *num < 1 || *days < 1 {
		return false, 0, 0, fmt.Errorf("num/days: %w", errInlalidArgs)
	}

	return *chunked, *days, *num, nil
}

func readConfigs(path string) (string, string, error) {
	f, err := os.Open(filepath.Join(path, dbnameFilename))
	if err != nil {
		return "", "", fmt.Errorf("can't open: %s: %w", dbnameFilename, err)
	}

	dbname, err := io.ReadAll(io.LimitReader(f, maxPostgresqlNameLen))
	if err != nil {
		return "", "", fmt.Errorf("can't read: %s: %w", dbnameFilename, err)
	}

	f, err = os.Open(filepath.Join(path, schemaNameFilename))
	if err != nil {
		return "", "", fmt.Errorf("can't open: %s: %w", schemaNameFilename, err)
	}

	schema, err := io.ReadAll(io.LimitReader(f, maxPostgresqlNameLen))
	if err != nil {
		return "", "", fmt.Errorf("can't read: %s: %w", schemaNameFilename, err)
	}

	return string(dbname), string(schema), nil
}
