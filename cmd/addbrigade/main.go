package main

import (
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/vpngen/keydesk/user"
	"github.com/vpngen/wordsgens/namesgenerator"
)

const (
	dbnameFilename     = "dbname"
	schemaNameFilename = "schema"
	etcDefaultPath     = "/etc/vpngen"
)

const maxPostgresqlNameLen = 63

const postgresqlSocket = "/var/run/postgresql"

const (
	BrigadeCgnatPrefix = 24
	BrigadeUlaPrefix   = 64
)

const (
	sqlGetBrigades = `
SELECT
	endpoint_ipv4,
	keydesk_ipv6,
	ipv4_cgnat,
	ipv6_ula
FROM %s 
FOR UPDATE`

	sqlPickPair = `
SELECT
	pair_id,
	control_ip,
	endpoint_ipv4
FROM %s
WHERE
pair_id = (
		SELECT 
			pair_id 
		FROM %s 
		ORDER BY free_slots_count DESC 
		LIMIT 1
		)
ORDER BY pair_id DESC
LIMIT 1
`

	sqlPickCGNATNet = `
SELECT 
	ipv4_net
FROM %s
ORDER BY id, weight DESC
LIMIT 1
`

	sqlPickULANet = `
SELECT 
	ipv6_net
FROM %s
ORDER BY id, iweight ASC
LIMIT 1
`

	sqlPickKeydeskNet = `
SELECT 
	ipv6_net
FROM %s
ORDER BY id, iweight ASC
LIMIT 1
`
)

type brigadeOpts struct {
	id     string
	name   string
	person namesgenerator.Person
}

// Args errors.
var (
	ErrEmptyBrigadierName   = errors.New("empty brigadier name")
	ErrInvalidBrigadierName = errors.New("invalid brigadier name")
	ErrEmptyPersonName      = errors.New("empty person name")
	ErrEmptyPersonDesc      = errors.New("empty person desc")
	ErrEmptyPersonURL       = errors.New("empty person url")
	ErrInvalidPersonName    = errors.New("invalid person name")
	ErrInvalidPersonDesc    = errors.New("invalid person desc")
	ErrInvalidPersonURL     = errors.New("invalid person url")
)

func main() {
	confDir := os.Getenv("CONFDIR")
	if confDir == "" {
		confDir = etcDefaultPath
	}

	opts, err := parseArgs()
	if err != nil {
		log.Fatalf("Can't parse args: %s\n", err)
	}

	dbname, schema, err := readConfigs(confDir)
	if err != nil {
		log.Fatalf("Can't read configs: %s\n", err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("Can't create db pool: %s\n", err)
	}

	err = createBrigade(db, schema, opts)
	if err != nil {
		log.Fatalf("Can't create brigade: %s\n", err)
	}

	// wgconfx = chunked (wgconf + keydesk IP)
	wgconfx, err := requestBrigade(db, schema, opts)
	if err != nil {
		log.Fatalf("Can't request brigade: %s\n", err)
	}

	_, err = os.Stdout.Write(wgconfx)
	if err != nil {
		log.Fatalf("Can't print brigade: %s\n", err)
	}
}

func createBrigade(db *pgxpool.Pool, schema string, opts *brigadeOpts) error {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	//schema := (pgx.Identifier{schema}).Sanitize()

	rows, err := tx.Query(ctx, fmt.Sprintf(sqlGetBrigades, (pgx.Identifier{schema, "brigades"}.Sanitize())))
	if err != nil {
		tx.Rollback(ctx)

		return fmt.Errorf("brigades query: %w", err)
	}

	// lock on brigades, register used nets

	kd6 := make(map[string]struct{})
	cgnat := make(map[string]struct{})
	ula := make(map[string]struct{})

	var (
		keydesk_ipv6 netip.Addr
		ipv4_cgnat   netip.Prefix
		ipv6_ula     netip.Prefix
	)

	_, err = pgx.ForEachRow(rows, []any{&keydesk_ipv6, &ipv4_cgnat, &ipv6_ula}, func() error {
		fmt.Fprintf(os.Stderr, "Brigade:\nkeydesk_ipv6: %v\n ipv4_cgnat: %v\nipv6_ula: %v\n", keydesk_ipv6, ipv4_cgnat, ipv6_ula)

		kd6[keydesk_ipv6.String()] = struct{}{}
		cgnat[ipv4_cgnat.Masked().Addr().String()] = struct{}{}
		ula[ipv6_ula.Masked().Addr().String()] = struct{}{}

		return nil
	})
	if err != nil {
		tx.Rollback(ctx)

		return fmt.Errorf("brigade row: %w", err)
	}

	// pick up a less used pair

	var (
		pair_id            []byte
		pair_endpoint_ipv4 netip.Addr
		pair_control_ip    netip.Addr
	)

	err = tx.QueryRow(ctx, fmt.Sprintf(sqlPickPair, (pgx.Identifier{schema, "slots"}.Sanitize()), (pgx.Identifier{schema, "active_pairs"}.Sanitize()))).Scan(&pair_id, &pair_control_ip, &pair_endpoint_ipv4)
	if err != nil {
		tx.Rollback(ctx)

		return fmt.Errorf("pair query: %w", err)
	}

	fmt.Fprintf(os.Stderr, "ep: %s ctrl: %s\n", pair_endpoint_ipv4, pair_control_ip)

	rand.Seed(time.Now().Unix() + int64(time.Now().Nanosecond()))

	// pick up cgnat

	var (
		cgnat_gnet netip.Prefix
		cgnat_net  netip.Prefix
	)

	err = tx.QueryRow(ctx, fmt.Sprintf(sqlPickCGNATNet, (pgx.Identifier{schema, "ipv4_cgnat_nets_weight"}.Sanitize()))).Scan(&cgnat_gnet)
	if err != nil {
		tx.Rollback(ctx)

		return fmt.Errorf("cgnat weight query: %w", err)
	}

	for {
		addr := user.RandomAddrIPv4(cgnat_gnet)
		cgnat_net = netip.PrefixFrom(addr, BrigadeCgnatPrefix)
		if cgnat_net.Masked().Addr() == addr || user.LastPrefixIPv6(cgnat_net.Masked()) == addr {
			continue
		}
		if _, ok := cgnat[cgnat_net.Masked().Addr().String()]; !ok {
			break
		}
	}

	fmt.Fprintf(os.Stderr, "cgnat_gnet: %s cgnat_net: %s\n", cgnat_gnet, cgnat_net)

	// pick up ula

	var (
		ula_gnet netip.Prefix
		ula_net  netip.Prefix
	)

	err = tx.QueryRow(ctx, fmt.Sprintf(sqlPickULANet, (pgx.Identifier{schema, "ipv6_ula_nets_iweight"}.Sanitize()))).Scan(&ula_gnet)
	if err != nil {
		tx.Rollback(ctx)

		return fmt.Errorf("ula weight query: %w", err)
	}

	for {
		addr := user.RandomAddrIPv6(ula_gnet)
		ula_net = netip.PrefixFrom(addr, BrigadeUlaPrefix)
		if ula_net.Masked().Addr() == addr || user.LastPrefixIPv6(ula_net.Masked()) == addr {
			continue
		}

		if _, ok := ula[ula_net.Masked().Addr().String()]; !ok {
			break
		}
	}

	fmt.Fprintf(os.Stderr, "ula_gnet: %s ula_net: %s\n", ula_gnet, ula_net)

	// pick up keydesk

	var (
		keydesk_gnet netip.Prefix
		keydesk      netip.Addr
	)

	err = tx.QueryRow(ctx, fmt.Sprintf(sqlPickKeydeskNet, (pgx.Identifier{schema, "ipv6_keydesk_nets_iweight"}.Sanitize()))).Scan(&keydesk_gnet)
	if err != nil {
		tx.Rollback(ctx)

		return fmt.Errorf("keydesk iweight query: %w", err)
	}

	for {
		keydesk = user.RandomAddrIPv6(keydesk_gnet)

		if i := binary.BigEndian.Uint16((keydesk.AsSlice())[14:]); i == 0 {
			continue
		}

		if _, ok := kd6[keydesk.String()]; !ok {
			break
		}
	}

	fmt.Fprintf(os.Stderr, "keydesk_gnet: %s keydesk: %s\n", keydesk_gnet, keydesk)

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func requestBrigade(db *pgxpool.Pool, schema string, opts *brigadeOpts) ([]byte, error) {
	return nil, nil
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

func parseArgs() (*brigadeOpts, error) {
	brigadeID := flag.String("id", "", "brigadier_id")
	brigadierName := flag.String("name", "", "brigadierName :: base64")
	personName := flag.String("person", "", "personName :: base64")
	personDesc := flag.String("desc", "", "personDesc :: base64")
	personURL := flag.String("url", "", "personURL :: base64")

	flag.Parse()

	opts := &brigadeOpts{}

	// brigadeID must be base32 decodable.
	id, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*brigadeID)
	if err != nil {
		return nil, fmt.Errorf("id base32: %s: %w", *brigadeID, err)
	}

	_, err = uuid.FromBytes(id)
	if err != nil {
		return nil, fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
	}

	opts.id = *brigadeID

	// brigadierName must be not empty and must be a valid UTF8 string
	if *brigadierName == "" {
		return nil, ErrEmptyBrigadierName
	}

	buf, err := base64.StdEncoding.DecodeString(*brigadierName)
	if err != nil {
		return nil, fmt.Errorf("brigadier name: %w", err)
	}

	if !utf8.Valid(buf) {
		return nil, ErrInvalidBrigadierName
	}

	opts.name = string(buf)

	// personName must be not empty and must be a valid UTF8 string
	if *personName == "" {
		return nil, ErrEmptyPersonName
	}

	buf, err = base64.StdEncoding.DecodeString(*personName)
	if err != nil {
		return nil, fmt.Errorf("person name: %w", err)
	}

	if !utf8.Valid(buf) {
		return nil, ErrInvalidPersonName
	}

	opts.person.Name = string(buf)

	// personDesc must be not empty and must be a valid UTF8 string
	if *personDesc == "" {
		return nil, ErrEmptyPersonDesc
	}

	buf, err = base64.StdEncoding.DecodeString(*personDesc)
	if err != nil {
		return nil, fmt.Errorf("person desc: %w", err)
	}

	if !utf8.Valid(buf) {
		return nil, ErrInvalidPersonDesc
	}

	opts.person.Desc = string(buf)

	// personURL must be not empty and must be a valid UTF8 string
	if *personURL == "" {
		return nil, ErrEmptyPersonURL
	}

	buf, err = base64.StdEncoding.DecodeString(*personURL)
	if err != nil {
		return nil, fmt.Errorf("person url: %w", err)
	}

	if !utf8.Valid(buf) {
		return nil, ErrInvalidPersonURL
	}

	u := string(buf)

	_, err = url.Parse(u)
	if err != nil {
		return nil, fmt.Errorf("parse person url: %w", err)
	}

	opts.person.URL = u

	return opts, nil
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
