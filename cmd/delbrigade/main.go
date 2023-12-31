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
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	dcmgmt "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"

	"golang.org/x/crypto/ssh"
)

const (
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	sshkeyRemoteUsername = "_serega_"
	sshKeyDefaultPath    = "/etc/vg-dc-vpnapi"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	BrigadeCgnatPrefix = 24
	BrigadeUlaPrefix   = 64
)

const (
	subdomainAPIAttempts = 5
	subdomainAPISleep    = 2 * time.Second
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
	sqlDelBrigade = `
	DELETE 
		FROM %s
	WHERE brigade_id=$1
	RETURNING domain_name
	`
)

var errInlalidArgs = errors.New("invalid args")

var LogTag = setLogTag()

const defaultLogTag = "delbrigade"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	var w io.WriteCloser

	chunked, base32String, uuidString, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	sshKeyFilename,
		dbname, schema,
		subdomAPIHost, subdomAPIToken,
		ident,
		delegationUser, delegationServer,
		kdAddrUser, kdAddrServer,
		err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	sshconf, err := kdlib.CreateSSHConfig(sshKeyFilename, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	delegationSyncSSHconf, err := kdlib.CreateSSHConfig(sshKeyFilename, delegationUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("Can't create delegation sync ssh config: %s\n", err)
	}

	kdAddrSyncSSHconf, err := kdlib.CreateSSHConfig(sshKeyFilename, kdAddrUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("Can't create keydesk address ssh config: %s\n", err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	control_ip, err := getBrigadeControlIP(db, schema, uuidString)
	if err != nil {
		log.Fatalf("%s: Can't get control ip: %s\n", LogTag, err)
	}

	// attention! brigadeID - base32-style.
	output, err := revokeBrigade(sshconf, base32String, control_ip)
	if err != nil {
		log.Fatalf("%s: Can't revoke brigade: %s\n", LogTag, err)
	}

	// attention! id - uuid-style string.
	num, err := removeBrigade(
		db, schema,
		uuidString,
		subdomAPIHost, subdomAPIToken,
		ident,
		delegationServer, delegationSyncSSHconf,
		kdAddrServer, kdAddrSyncSSHconf,
	)
	if err != nil {
		log.Fatalf("%s: Can't remove brigade: %s\n", LogTag, err)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	fmt.Fprintf(w, "%d\n", num)

	if output == nil {
		output = []byte{}
	}

	_, err = w.Write(output)
	if err != nil {
		log.Fatalf("%s: Can't print output: %s\n", LogTag, err)
	}
}

func getBrigadeControlIP(db *pgxpool.Pool, schema string, brigadeID string) (netip.Addr, error) {
	ctx := context.Background()
	emptyIP := netip.Addr{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return emptyIP, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	var control_ip netip.Addr

	if err := tx.QueryRow(ctx,
		fmt.Sprintf(sqlGetControlIP,
			(pgx.Identifier{schema, "meta_brigades"}.Sanitize()),
		),
		brigadeID,
	).Scan(
		&control_ip,
	); err != nil {
		return emptyIP, fmt.Errorf("brigade query: %w", err)
	}

	return control_ip, nil
}

func removeBrigade(
	db *pgxpool.Pool, schema string,
	brigadeID string,
	subdomAPIHost, subdomAPIToken string,
	ident string,
	delegationSyncServer string, delegationSyncSSHconf *ssh.ClientConfig,
	kdAddrSyncServer string, kdAddrSyncSSHconf *ssh.ClientConfig,
) (int32, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	var domain_name pgtype.Text

	if err := tx.QueryRow(
		ctx,
		fmt.Sprintf(sqlDelBrigade, pgx.Identifier{schema, "brigades"}.Sanitize()),
		brigadeID,
	).Scan(&domain_name); err != nil {
		return 0, fmt.Errorf("brigade delete: %w", err)
	}

	num := int32(0)

	if err := tx.QueryRow(ctx, kdlib.GetFreeSlotsNumberStatement(schema, true)).Scan(&num); err != nil {
		return 0, fmt.Errorf("free slots query: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	if domain_name.Valid {
		if err := revokeSubdomain(ctx, db, schema, subdomAPIHost, subdomAPIToken, domain_name.String); err != nil {
			return 0, fmt.Errorf("revoke subdomain: %w", err)
		}
	}

	// Sync delegation list.

	delegationList, err := dcmgmt.NewDelegationList(ctx, db, schema)
	if err != nil {
		return 0, fmt.Errorf("delegation list: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s: %s@%s\n", LogTag, delegationSyncSSHconf.User, delegationSyncServer)
	cleanup, err := dcmgmt.SyncDelegationList(delegationSyncSSHconf, delegationSyncServer, ident, delegationList)
	cleanup(LogTag)

	if err != nil {
		return 0, fmt.Errorf("delegation sync: %w", err)
	}

	// Sync keydesk address list.

	kdAddrList, err := dcmgmt.NewKdAddrList(ctx, db, schema)
	if err != nil {
		return 0, fmt.Errorf("keydesk addr list: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s: %s@%s\n", LogTag, kdAddrSyncSSHconf.User, kdAddrSyncServer)
	cleanup, err = dcmgmt.SyncKdAddrList(kdAddrSyncSSHconf, kdAddrSyncServer, ident, kdAddrList)
	cleanup(LogTag)

	if err != nil {
		return 0, fmt.Errorf("keydesk address sync: %w", err)
	}

	return num, nil
}

func revokeSubdomain(ctx context.Context, db *pgxpool.Pool, schema string, subdomAPIHost, subdomAPIToken string, domain_name string) error {
	if subdomAPIToken == dcmgmt.NoUseSubdomainAPIToken {
		return nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	sqlDelPairDomain := `DELETE FROM %s WHERE domain_name=$1`
	commTag, err := tx.Exec(ctx, fmt.Sprintf(
		sqlDelPairDomain,
		pgx.Identifier{schema, "domains_endpoints_ipv4"}.Sanitize()),
		domain_name,
	)
	if err != nil {
		return fmt.Errorf("pair domain delete: %w", err)
	}

	if commTag.RowsAffected() == 0 {
		return fmt.Errorf("pair domain delete: no rows affected")
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	for i := 0; i < subdomainAPIAttempts; i++ {
		if err := kdlib.SubdomainDelete(subdomAPIHost, subdomAPIToken, domain_name); err != nil {
			fmt.Fprintf(os.Stderr, "%s: Can't delete subdomain %s: %s\n", LogTag, domain_name, err)
			if i == subdomainAPIAttempts-1 {
				return fmt.Errorf("delete subdomain: %w", err)
			}

			time.Sleep(subdomainAPISleep)

			continue
		}

		break
	}

	return nil
}

func revokeBrigade(sshconf *ssh.ClientConfig, brigadeID string, control_ip netip.Addr) ([]byte, error) {
	cmd := fmt.Sprintf("destroy -id %s -ch", brigadeID)

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
		return nil, fmt.Errorf("ssh run: %w", err)
	}

	return nil, nil
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

func readConfigs() (string, string, string, string, string, string, string, string, string, string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	brigadeSchema := os.Getenv("BRIGADES_SCHEMA")
	if brigadeSchema == "" {
		brigadeSchema = defaultBrigadesSchema
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshKeyDefaultPath)
	if err != nil {
		return "", "", "", "", "", "", "", "", "", "", fmt.Errorf("ssh key: %w", err)
	}

	subdomainAPIHost := os.Getenv("SUBDOMAIN_API_SERVER")
	if subdomainAPIHost == "" {
		return "", "", "", "", "", "", "", "", "", "", errors.New("empty subdomapi host")
	}

	if _, err := netip.ParseAddrPort(subdomainAPIHost); err != nil {
		return "", "", "", "", "", "", "", "", "", "", fmt.Errorf("parse subdomapi host: %w", err)
	}

	subdomainAPIToken := os.Getenv("SUBDOMAIN_API_TOKEN")
	if subdomainAPIToken == "" {
		return "", "", "", "", "", "", "", "", "", "", errors.New("empty subdomapi token")
	}

	_, ident, err := dcmgmt.ParseDCNameEnv()
	if err != nil {
		return "", "", "", "", "", "", "", "", "", "", fmt.Errorf("dc name: %w", err)
	}

	delegationUser, delegationServer, err := dcmgmt.ParseConnEnv("DELEGATION_SYNC_CONNECT")
	if err != nil {
		return "", "", "", "", "", "", "", "", "", "", fmt.Errorf("delegation sync connect: %w", err)
	}

	kdAddrUser, kdAddrServer, err := dcmgmt.ParseConnEnv("KEYDESK_ADDRESS_SYNC_CONNECT")
	if err != nil {
		return "", "", "", "", "", "", "", "", "", "", fmt.Errorf("keydesk address sync connect: %w", err)
	}

	return sshKeyFilename,
		dbURL, brigadeSchema,
		subdomainAPIHost, subdomainAPIToken,
		ident,
		delegationUser, delegationServer,
		kdAddrUser, kdAddrServer,
		nil
}
