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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

var errInlalidArgs = errors.New("invalid args")

var LogTag = setLogTag()

const defaultLogTag = "checkbrigade"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	var w io.WriteCloser

	chunked, bid32, id, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	dbname, schemaBrigades, schemaStats, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	// attention! id - uuid-style string.
	brigadeGetID, totalUsersCount, activeUsersCount, createdAt, firstVisit, err := checkBrigade(db, schemaBrigades, schemaStats, id)
	if err != nil {
		log.Fatalf("%s: Can't check brigade: %s\n", LogTag, err)
	}

	if brigadeGetID != id {
		log.Fatalf("%s: Brigade ID not matched: %s vs %s\n", LogTag, id, brigadeGetID)
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
	fmt.Fprintln(w, totalUsersCount)
	fmt.Fprintln(w, activeUsersCount)
	fmt.Fprintln(w, createdAt)
	fmt.Fprintln(w, firstVisit)
}

func checkBrigade(db *pgxpool.Pool, schemaBrigades, schemaStats, brigadeID string) (string, int, int, string, string, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", 0, 0, "", "", fmt.Errorf("begin: %w", err)
	}

	var (
		bid              string
		totalUsersCount  int
		activeUsersCount int
		createdAt        pgtype.Timestamp
		firstVisit       pgtype.Timestamp
	)

	sqlGetBrigade := `
	SELECT
		b.brigade_id,
		s.total_users_count,
		s.active_users_count,
		s.created_at,
		s.first_visit
	FROM %s b LEFT JOIN %s s ON b.brigade_id=s.brigade_id
	WHERE
		b.brigade_id=$1
	`

	err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlGetBrigade,
			(pgx.Identifier{schemaBrigades, "brigades"}.Sanitize()),
			(pgx.Identifier{schemaStats, "brigades_stats"}.Sanitize()),
		),
		brigadeID,
	).Scan(
		&bid,
		&totalUsersCount,
		&activeUsersCount,
		&createdAt,
		&firstVisit,
	)
	if err != nil {
		tx.Rollback(ctx)

		return "", 0, 0, "", "", fmt.Errorf("brigade query: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return "", 0, 0, "", "", fmt.Errorf("commit: %w", err)
	}

	return bid, totalUsersCount, activeUsersCount, createdAt.Time.String(), firstVisit.Time.String(), nil
}

func createDBPool(dburl string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dburl)
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

func readConfigs() (string, string, string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	brigadeSchema := os.Getenv("BRIGADES_SCHEMA")
	if brigadeSchema == "" {
		brigadeSchema = defaultBrigadesSchema
	}

	brigadesStatsSchema := os.Getenv("BRIGADES_STATS_SCHEMA")
	if brigadesStatsSchema == "" {
		brigadesStatsSchema = defaultBrigadesStatsSchema
	}

	return dbURL, brigadeSchema, brigadesStatsSchema, nil
}
