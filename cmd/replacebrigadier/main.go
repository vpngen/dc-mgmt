package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http/httputil"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"golang.org/x/crypto/ssh"
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

const sshTimeOut = time.Duration(75 * time.Second)

const (
	BrigadeCgnatPrefix = 24
	BrigadeUlaPrefix   = 64
)

const (
	sqlGetControlIP = `
	SELECT
		control_ip,
		keydesk_ipv6
	FROM %s
	WHERE
		brigade_id=$1
	FOR UPDATE
	`
)

var errInlalidArgs = errors.New("invalid args")

var LogTag = setLogTag()

const defaultLogTag = "replacebrigadier"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	var w io.WriteCloser

	chunked, brigadeID, id, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	sshKeyDir, dbname, schema, _, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	sshconf, err := createSSHConfig(sshKeyDir)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	// attention! id - uuid-style string.
	control_ip, keydesk_ipv6, err := checkBrigade(db, schema, id)
	if err != nil {
		log.Fatalf("%s: Can't check brigade: %s\n", LogTag, err)
	}

	// attention! brigadeID - base32-style.
	wgconfx, err := replaceBrigadier(db, schema, sshconf, brigadeID, control_ip)
	if err != nil {
		log.Fatalf("%s: Can't replace brigadier: %s\n", LogTag, err)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	_, err = fmt.Fprintln(w, keydesk_ipv6.String())
	if err != nil {
		log.Fatalf("%s: Can't print keydesk ipv6: %s\n", LogTag, err)
	}

	if wgconfx == nil {
		wgconfx = []byte{}
	}

	_, err = w.Write(wgconfx)
	if err != nil {
		log.Fatalf("%s: Can't print wgconfx: %s\n", LogTag, err)
	}
}

func checkBrigade(db *pgxpool.Pool, schema string, brigadeID string) (netip.Addr, netip.Addr, error) {
	ctx := context.Background()
	emptyIP := netip.Addr{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return emptyIP, emptyIP, fmt.Errorf("begin: %w", err)
	}

	var (
		control_ip   netip.Addr
		keydesk_ipv6 netip.Addr
	)

	err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlGetControlIP,
			(pgx.Identifier{schema, "meta_brigades"}.Sanitize()),
		),
		brigadeID,
	).Scan(
		&control_ip,
		&keydesk_ipv6,
	)
	if err != nil {
		tx.Rollback(ctx)

		return emptyIP, emptyIP, fmt.Errorf("brigade query: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return emptyIP, emptyIP, fmt.Errorf("commit: %w", err)
	}

	return control_ip, keydesk_ipv6, nil
}

func replaceBrigadier(db *pgxpool.Pool, schema string, sshconf *ssh.ClientConfig, brigadeID string, control_ip netip.Addr) ([]byte, error) {
	cmd := fmt.Sprintf("replace %s chunked", brigadeID)

	fmt.Fprintf(os.Stderr, "%s: %s#%s:22 -> %s\n", LogTag, sshkeyRemoteUsername, control_ip, cmd)

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", control_ip), sshconf)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var b, e bytes.Buffer

	session.Stdout = &b
	session.Stderr = &e

	if err := session.Run(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "%s: session errors:\n%s\n", LogTag, e.String())

		return nil, fmt.Errorf("ssh run: %w", err)
	}

	if errstr := e.String(); errstr != "" {
		fmt.Fprintf(os.Stderr, "%s: session errors:\n%s\n", LogTag, errstr)
	}

	wgconfx, err := io.ReadAll(httputil.NewChunkedReader(&b))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: readed data:\n%s\n", LogTag, wgconfx)

		return nil, fmt.Errorf("chunk read: %w", err)
	}

	return wgconfx, nil
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
