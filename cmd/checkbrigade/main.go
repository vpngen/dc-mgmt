package main

import (
	"context"
	"encoding/base32"
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
	dbnameFilename     = "dbname"
	schemaNameFilename = "schema"
	etcDefaultPath     = "/etc/vgrealm"
)

const (
	maxPostgresqlNameLen = 63
	postgresqlSocket     = "/var/run/postgresql"
)

const sshTimeOut = time.Duration(5 * time.Second)

const (
	BrigadeCgnatPrefix = 24
	BrigadeUlaPrefix   = 64
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

	chunked, bid32, id, err := parseArgs()
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

	// attention! id - uuid-style string.
	brigadeGetID, userCount, createdAt, lastVisit, err := checkBrigade(db, schema, id)
	if err != nil {
		log.Fatalf("%s: Can't check brigade: %s\n", exe, err)
	}

	if brigadeGetID != id {
		log.Fatalf("Brigade ID not matched: %s vs %s\n", id, brigadeGetID)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	fmt.Fprintln(w, brigadeGetID)
	fmt.Fprintln(w, bid32)
	fmt.Fprintln(w, userCount)
	fmt.Fprintln(w, createdAt)
	fmt.Fprintln(w, lastVisit)
}

func checkBrigade(db *pgxpool.Pool, schema string, brigadeID string) (string, int, string, string, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", 0, "", "", fmt.Errorf("begin: %w", err)
	}

	var (
		bid  string
		ucnt int
		ct   pgtype.Timestamp
		lv   pgtype.Timestamp
	)

	sqlGetBrigade := `
	SELECT
		brigade_id,
		user_count,
		create_at,
		last_visit
	FROM %s AS s
	WHERE
		brigade_id=$1
	`

	err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlGetBrigade,
			(pgx.Identifier{"stats", "brigades_stats"}.Sanitize()),
		),
		brigadeID,
	).Scan(
		&bid,
		&ucnt,
		&ct,
		&lv,
	)
	if err != nil {
		tx.Rollback(ctx)

		return "", 0, "", "", fmt.Errorf("brigade query: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return "", 0, "", "", fmt.Errorf("commit: %w", err)
	}

	return bid, ucnt, ct.Time.String(), lv.Time.String(), nil
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

func parseArgs() (bool, string, string, error) {
	brigadeID := flag.String("id", "", "brigadier_id in base32 form")
	brigadeUUID := flag.String("uuid", "", "brigadier_id in uuid form")
	chunked := flag.Bool("ch", false, "chunked output")

	flag.Parse()

	switch {
	case *brigadeID != "" && *brigadeUUID == "":
		// brigadeID must be base32 decodable.
		buf, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*brigadeID)
		if err != nil {
			return false, "", "", fmt.Errorf("id base32: %s: %w", *brigadeID, err)
		}

		id, err := uuid.FromBytes(buf)
		if err != nil {
			return false, "", "", fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		return *chunked, *brigadeID, id.String(), nil
	case *brigadeUUID != "" && *brigadeID == "":
		id, err := uuid.Parse(*brigadeUUID)
		if err != nil {
			return false, "", "", fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		bid := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:])

		return *chunked, bid, id.String(), nil
	default:
		return false, "", "", fmt.Errorf("both ids: %w", errInlalidArgs)
	}
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
