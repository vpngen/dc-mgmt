package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"golang.org/x/crypto/ssh"

	"github.com/vpngen/realm-admin/internal/kdlib"
	"github.com/vpngen/wordsgens/namesgenerator"
)

const (
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	sshkeyFilename       = "id_ecdsa"
	sshkeyRemoteUsername = "_serega_"
	etcDefaultPath       = "/etc/vg-dc-mgmt"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const sshTimeOut = time.Duration(15 * time.Second)

const (
	BrigadeCgnatPrefix = 24
	BrigadeUlaPrefix   = 64
)

const (
	sqlGetBrigades = `
SELECT
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

	sqlPickPairForcedIP = `
SELECT
	pair_id,
	control_ip,
	endpoint_ipv4
FROM %s
WHERE
	control_ip=$1
LIMIT 1
`

	sqlPickCGNATNet = `
SELECT 
	ipv4_net
FROM %s
ORDER BY weight DESC, id
LIMIT 1
`

	sqlPickULANet = `
SELECT 
	ipv6_net
FROM %s
ORDER BY iweight ASC, id
LIMIT 1
`

	sqlPickKeydeskNet = `
SELECT 
	ipv6_net
FROM %s
ORDER BY iweight ASC, id
LIMIT 1
`

	sqlCreateBrigade = `
INSERT INTO %s
		(
			brigade_id,  
			pair_id,    
			brigadier,           
			endpoint_ipv4,       
			dns_ipv4,            
			dns_ipv6,            
			keydesk_ipv6,        
			ipv4_cgnat,          
			ipv6_ula,            
			person              
		)
VALUES 
		(
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10
		)
`

	sqlFetchBrigade = `
SELECT
	meta_brigades.brigade_id,
	meta_brigades.brigadier,
	meta_brigades.endpoint_ipv4,
	meta_brigades.dns_ipv4,
	meta_brigades.dns_ipv6,
	meta_brigades.keydesk_ipv6,
	meta_brigades.ipv4_cgnat,
	meta_brigades.ipv6_ula,
	meta_brigades.person,
	meta_brigades.control_ip
FROM %s
WHERE
	meta_brigades.brigade_id=$1
`
	sqlInsertStats = `INSERT INTO %s (brigade_id) VALUES ($1);`
)

type brigadeOpts struct {
	id      string
	name    string
	forceIP netip.Addr
	person  namesgenerator.Person
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

var LogTag = setLogTag()

const defaultLogTag = "addbrigade"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	var w io.WriteCloser

	chunked, opts, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	sshKeyDir, dbURL, brigadesSchema, brigadesStatsSchema, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	sshconf, err := createSSHConfig(sshKeyDir)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbURL)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	num, err := createBrigade(db, brigadesSchema, brigadesStatsSchema, opts)
	if err != nil {
		log.Fatalf("%s: Can't create brigade: %s\n", LogTag, err)
	}

	// wgconfx = chunked (wgconf + keydesk IP)
	wgconfx, keydesk, err := requestBrigade(db, brigadesSchema, sshconf, opts)
	if err != nil {
		log.Fatalf("%s: Can't request brigade: %s\n", LogTag, err)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	if _, err := fmt.Fprintf(w, "%d\n", num); err != nil {
		log.Fatalf("%s: Can't print free slots num: %s\n", LogTag, err)
	}

	if _, err = fmt.Fprintln(w, keydesk); err != nil {
		log.Fatalf("%s: Can't print keydesk ipv6: %s\n", LogTag, err)
	}

	if _, err = w.Write(wgconfx); err != nil {
		log.Fatalf("%s: Can't print wgconfx: %s\n", LogTag, err)
	}
}

func createBrigade(db *pgxpool.Pool, schema, schemaStats string, opts *brigadeOpts) (int32, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, fmt.Sprintf(sqlGetBrigades, (pgx.Identifier{schema, "brigades"}.Sanitize())))
	if err != nil {
		return 0, fmt.Errorf("brigades query: %w", err)
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

	if _, err := pgx.ForEachRow(rows, []any{&keydesk_ipv6, &ipv4_cgnat, &ipv6_ula}, func() error {
		// fmt.Fprintf(os.Stderr, "Brigade:\n  keydesk_ipv6: %v\n  ipv4_cgnat: %v\n  ipv6_ula: %v\n", keydesk_ipv6, ipv4_cgnat, ipv6_ula)

		kd6[keydesk_ipv6.String()] = struct{}{}
		cgnat[ipv4_cgnat.Masked().Addr().String()] = struct{}{}
		ula[ipv6_ula.Masked().Addr().String()] = struct{}{}

		return nil
	}); err != nil {
		return 0, fmt.Errorf("brigade row: %w", err)
	}

	// pick up a less used pair

	var (
		pair_id            string
		pair_endpoint_ipv4 netip.Addr
		pair_control_ip    netip.Addr
	)

	switch opts.forceIP {
	case netip.Addr{}:
		err = tx.QueryRow(ctx, fmt.Sprintf(sqlPickPair, (pgx.Identifier{schema, "slots"}.Sanitize()), (pgx.Identifier{schema, "active_pairs"}.Sanitize()))).Scan(&pair_id, &pair_control_ip, &pair_endpoint_ipv4)
	default:
		err = tx.QueryRow(ctx, fmt.Sprintf(sqlPickPairForcedIP, (pgx.Identifier{schema, "slots"}.Sanitize())), opts.forceIP.String()).Scan(&pair_id, &pair_control_ip, &pair_endpoint_ipv4)
	}

	if err != nil {
		return 0, fmt.Errorf("pair query: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s: ep: %s ctrl: %s\n", LogTag, pair_endpoint_ipv4, pair_control_ip)

	// pick up cgnat

	var (
		cgnat_gnet netip.Prefix
		cgnat_net  netip.Prefix
	)

	if err := tx.QueryRow(ctx, fmt.Sprintf(sqlPickCGNATNet, (pgx.Identifier{schema, "ipv4_cgnat_nets_weight"}.Sanitize()))).Scan(&cgnat_gnet); err != nil {
		return 0, fmt.Errorf("cgnat weight query: %w", err)
	}

	for {
		addr := kdlib.RandomAddrIPv4(cgnat_gnet)
		if kdlib.IsZeroEnding(addr) {
			continue
		}

		cgnat_net = netip.PrefixFrom(addr, BrigadeCgnatPrefix)
		if cgnat_net.Masked().Addr() == addr || kdlib.LastPrefixIPv4(cgnat_net.Masked()) == addr {
			continue
		}
		if _, ok := cgnat[cgnat_net.Masked().Addr().String()]; !ok {
			break
		}
	}

	fmt.Fprintf(os.Stderr, "%s: cgnat_gnet: %s cgnat_net: %s\n", LogTag, cgnat_gnet, cgnat_net)

	// pick up ula

	var (
		ula_gnet netip.Prefix
		ula_net  netip.Prefix
	)

	if err := tx.QueryRow(ctx, fmt.Sprintf(sqlPickULANet, (pgx.Identifier{schema, "ipv6_ula_nets_iweight"}.Sanitize()))).Scan(&ula_gnet); err != nil {
		return 0, fmt.Errorf("ula weight query: %w", err)
	}

	for {
		addr := kdlib.RandomAddrIPv6(ula_gnet)
		if kdlib.IsZeroEnding(addr) {
			continue
		}

		ula_net = netip.PrefixFrom(addr, BrigadeUlaPrefix)
		if ula_net.Masked().Addr() == addr || kdlib.LastPrefixIPv6(ula_net.Masked()) == addr {
			continue
		}

		if _, ok := ula[ula_net.Masked().Addr().String()]; !ok {
			break
		}
	}

	fmt.Fprintf(os.Stderr, "%s: ula_gnet: %s ula_net: %s\n", LogTag, ula_gnet, ula_net)

	// pick up keydesk

	var (
		keydesk_gnet netip.Prefix
		keydesk      netip.Addr
	)

	if err := tx.QueryRow(ctx, fmt.Sprintf(sqlPickKeydeskNet, (pgx.Identifier{schema, "ipv6_keydesk_nets_iweight"}.Sanitize()))).Scan(&keydesk_gnet); err != nil {
		return 0, fmt.Errorf("keydesk iweight query: %w", err)
	}

	for {
		keydesk = kdlib.RandomAddrIPv6(keydesk_gnet)
		if kdlib.IsZeroEnding(keydesk) {
			continue
		}

		if _, ok := kd6[keydesk.String()]; !ok {
			break
		}
	}

	fmt.Fprintf(os.Stderr, "%s: keydesk_gnet: %s keydesk: %s\n", LogTag, keydesk_gnet, keydesk)

	// create brigade

	if _, err := tx.Exec(ctx,
		fmt.Sprintf(sqlCreateBrigade, (pgx.Identifier{schema, "brigades"}.Sanitize())),
		opts.id,
		pair_id,
		opts.name,
		pair_endpoint_ipv4.String(),
		cgnat_net.Addr().String(),
		ula_net.Addr().String(),
		keydesk.String(),
		cgnat_net.String(),
		ula_net.String(),
		opts.person,
	); err != nil {
		return 0, fmt.Errorf("create brigade: %w", err)
	}

	if _, err = tx.Exec(ctx,
		fmt.Sprintf(sqlInsertStats, (pgx.Identifier{schemaStats, "brigades_stats"}.Sanitize())),
		opts.id,
	); err != nil {
		return 0, fmt.Errorf("create stats: %w", err)
	}

	num := int32(0)

	if err := tx.QueryRow(ctx,
		kdlib.GetFreeSlotsNumberStatement(schema, true),
	).Scan(&num); err != nil {
		return 0, fmt.Errorf("slots query: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return num, nil
}

func requestBrigade(db *pgxpool.Pool, schema string, sshconf *ssh.ClientConfig, opts *brigadeOpts) ([]byte, string, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("begin: %w", err)
	}

	var (
		brigade_id    []byte
		fullname      string
		endpoint_ipv4 netip.Addr
		dns_ipv4      netip.Addr
		dns_ipv6      netip.Addr
		keydesk_ipv6  netip.Addr
		ipv4_cgnat    netip.Prefix
		ipv6_ula      netip.Prefix
		pjson         []byte
		control_ip    netip.Addr
	)

	err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlFetchBrigade,
			(pgx.Identifier{schema, "meta_brigades"}.Sanitize()),
		),
		opts.id,
	).Scan(
		&brigade_id,
		&fullname,
		&endpoint_ipv4,
		&dns_ipv4,
		&dns_ipv6,
		&keydesk_ipv6,
		&ipv4_cgnat,
		&ipv6_ula,
		&pjson,
		&control_ip,
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, "", fmt.Errorf("brigade query: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("commit: %w", err)
	}

	person := &namesgenerator.Person{}
	err = json.Unmarshal(pjson, &person)
	if err != nil {
		return nil, "", fmt.Errorf("person: %w", err)
	}

	cmd := fmt.Sprintf("create %s %s %s %s %s %s %s %s %s %s %s chunked",
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigade_id),
		endpoint_ipv4,
		ipv4_cgnat,
		ipv6_ula,
		dns_ipv4,
		dns_ipv6,
		keydesk_ipv6,
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(fullname)),
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.Name)),
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.Desc)),
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.URL)),
	)

	fmt.Fprintf(os.Stderr, "%s: %s#%s:22 -> %s\n", LogTag, sshkeyRemoteUsername, control_ip, cmd)

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", control_ip), sshconf)
	if err != nil {
		return nil, "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, "", fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var b, e bytes.Buffer

	session.Stdout = &b
	session.Stderr = &e

	defer func() {
		switch errstr := e.String(); errstr {
		case "":
			fmt.Fprintf(os.Stderr, "%s: SSH Session StdErr: empty\n", LogTag)
		default:
			fmt.Fprintf(os.Stderr, "%s: SSH Session StdErr:\n", LogTag)
			for _, line := range strings.Split(errstr, "\n") {
				fmt.Fprintf(os.Stderr, "%s: | %s\n", LogTag, line)
			}
		}
	}()

	if err := session.Run(cmd); err != nil {
		return nil, "", fmt.Errorf("ssh run: %w", err)
	}

	wgconfx, err := io.ReadAll(httputil.NewChunkedReader(&b))
	if err != nil {
		return nil, "", fmt.Errorf("chunk read: %w", err)
	}

	return wgconfx, keydesk_ipv6.String(), nil
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

func parseArgs() (bool, *brigadeOpts, error) {
	brigadeID := flag.String("id", "", "brigadier_id")
	brigadierName := flag.String("name", "", "brigadierName :: base64")
	personName := flag.String("person", "", "personName :: base64")
	personDesc := flag.String("desc", "", "personDesc :: base64")
	personURL := flag.String("url", "", "personURL :: base64")
	chunked := flag.Bool("ch", false, "chunked output")
	nodeIP := flag.String("ip", "", "control IP for debug")

	flag.Parse()

	opts := &brigadeOpts{}

	// brigadeID must be base32 decodable.
	buf, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*brigadeID)
	if err != nil {
		return false, nil, fmt.Errorf("id base32: %s: %w", *brigadeID, err)
	}

	id, err := uuid.FromBytes(buf)
	if err != nil {
		return false, nil, fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
	}

	opts.id = id.String()

	// brigadierName must be not empty and must be a valid UTF8 string
	if *brigadierName == "" {
		return false, nil, ErrEmptyBrigadierName
	}

	buf, err = base64.StdEncoding.DecodeString(*brigadierName)
	if err != nil {
		return false, nil, fmt.Errorf("brigadier name: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, nil, ErrInvalidBrigadierName
	}

	opts.name = string(buf)

	// personName must be not empty and must be a valid UTF8 string
	if *personName == "" {
		return false, nil, ErrEmptyPersonName
	}

	buf, err = base64.StdEncoding.DecodeString(*personName)
	if err != nil {
		return false, nil, fmt.Errorf("person name: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, nil, ErrInvalidPersonName
	}

	opts.person.Name = string(buf)

	// personDesc must be not empty and must be a valid UTF8 string
	if *personDesc == "" {
		return false, nil, ErrEmptyPersonDesc
	}

	buf, err = base64.StdEncoding.DecodeString(*personDesc)
	if err != nil {
		return false, nil, fmt.Errorf("person desc: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, nil, ErrInvalidPersonDesc
	}

	opts.person.Desc = string(buf)

	// personURL must be not empty and must be a valid UTF8 string
	if *personURL == "" {
		return false, nil, ErrEmptyPersonURL
	}

	buf, err = base64.StdEncoding.DecodeString(*personURL)
	if err != nil {
		return false, nil, fmt.Errorf("person url: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, nil, ErrInvalidPersonURL
	}

	u := string(buf)

	_, err = url.Parse(u)
	if err != nil {
		return false, nil, fmt.Errorf("parse person url: %w", err)
	}

	opts.person.URL = u

	if *nodeIP != "" {
		opts.forceIP, _ = netip.ParseAddr(*nodeIP)
	}

	return *chunked, opts, nil
}

func readConfigs() (string, string, string, string, error) {
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

	sshKeyDir := os.Getenv("CONFDIR")
	if sshKeyDir == "" {
		sysUser, err := user.Current()
		if err != nil {
			return "", "", "", "", fmt.Errorf("user: %w", err)
		}

		sshKeyDir = filepath.Join(sysUser.HomeDir, ".ssh")
	}

	if fstat, err := os.Stat(sshKeyDir); err != nil || !fstat.IsDir() {
		sshKeyDir = etcDefaultPath
	}

	return sshKeyDir, dbURL, brigadeSchema, brigadesStatsSchema, nil
}

func createSSHConfig(path string) (*ssh.ClientConfig, error) {
	// var hostKey ssh.PublicKey

	key, err := os.ReadFile(filepath.Join(path, sshkeyFilename))
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: sshkeyRemoteUsername,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		// HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshTimeOut,
	}

	return config, nil
}
