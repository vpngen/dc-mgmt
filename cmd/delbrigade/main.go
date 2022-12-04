package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http/httputil"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"golang.org/x/crypto/ssh"
)

const (
	dbnameFilename       = "dbname"
	schemaNameFilename   = "schema"
	sshkeyFilename       = "id_ecdsa"
	sshkeyRemoteUsername = "_serega_"
	etcDefaultPath       = "/etc/vgrealm"
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

const (
	sqlGetControlIP = `
	SELECT
		control_ip
	FROM %s
	WHERE
		brigade_id=$1
	FOR UPDATE
	`
	sqlDelBrigades = `
	DELETE 
		FROM %s
	WHERE brigade_id=$1
	`
)

func main() {
	var w io.WriteCloser

	confDir := os.Getenv("CONFDIR")
	if confDir == "" {
		confDir = etcDefaultPath
	}

	chunked, brigadeID, id, err := parseArgs()
	if err != nil {
		log.Fatalf("Can't parse args: %s\n", err)
	}

	dbname, schema, err := readConfigs(confDir)
	if err != nil {
		log.Fatalf("Can't read configs: %s\n", err)
	}

	sshconf, err := createSSHConfig(confDir)
	if err != nil {
		log.Fatalf("Can't create ssh configs: %s\n", err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("Can't create db pool: %s\n", err)
	}

	// attention! id - uuid-style string.
	control_ip, err := removeBrigade(db, schema, id)
	if err != nil {
		log.Fatalf("Can't create brigade: %s\n", err)
	}

	// attention! brigadeID - base32-style.
	output, err := revokeBrigade(db, schema, sshconf, brigadeID, control_ip)
	if err != nil {
		log.Fatalf("Can't request brigade: %s\n", err)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	_, err = w.Write(output)
	if err != nil {
		log.Fatalf("Can't print output: %s\n", err)
	}
}

func removeBrigade(db *pgxpool.Pool, schema string, brigadeID string) (netip.Addr, error) {
	ctx := context.Background()
	emptyIP := netip.Addr{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return emptyIP, fmt.Errorf("begin: %w", err)
	}

	var control_ip netip.Addr

	err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlGetControlIP,
			(pgx.Identifier{schema, "meta_brigades"}.Sanitize()),
		),
		brigadeID,
	).Scan(
		&control_ip,
	)
	if err != nil {
		tx.Rollback(ctx)

		return emptyIP, fmt.Errorf("brigade query: %w", err)
	}

	_, err = tx.Exec(ctx, fmt.Sprintf(sqlDelBrigades, (pgx.Identifier{schema, "brigades"}.Sanitize())), brigadeID)
	if err != nil {
		tx.Rollback(ctx)

		return emptyIP, fmt.Errorf("brigade delete: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return emptyIP, fmt.Errorf("commit: %w", err)
	}

	return control_ip, nil
}

func revokeBrigade(db *pgxpool.Pool, schema string, sshconf *ssh.ClientConfig, brigadeID string, control_ip netip.Addr) ([]byte, error) {
	/*
		ctx := context.Background()
		tx, err := db.Begin(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("begin: %w", err)
		}

		err = tx.Commit(ctx)
		if err != nil {
			return nil, "", fmt.Errorf("commit: %w", err)
		}
	*/

	cmd := fmt.Sprintf("destroy %s chunked", brigadeID)

	fmt.Fprintf(os.Stderr, "%s#%s:22 -> %s\n", sshkeyRemoteUsername, control_ip, cmd)

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
		fmt.Fprintf(os.Stderr, "session errors:\n%s\n", e.String())

		return nil, fmt.Errorf("ssh run: %w", err)
	}

	wgconfx, err := io.ReadAll(httputil.NewChunkedReader(&b))
	if err != nil {
		fmt.Fprintf(os.Stderr, "readed data:\n%s\n", wgconfx)

		return nil, fmt.Errorf("chunk read: %w", err)
	}

	return wgconfx, nil
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
	brigadeID := flag.String("id", "", "brigadier_id")
	chunked := flag.Bool("ch", false, "chunked output")

	flag.Parse()

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
