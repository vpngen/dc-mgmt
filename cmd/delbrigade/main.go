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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	dcmgmt "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"
	"github.com/vpngen/keydesk/kdlib/lockedfile"

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

	deleteAttempts = 5
)

const (
	DefaultDelegationMutexPath = "/tmp/delegation_mutex.lock"
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

type opts struct {
	sshKeyFilename string

	dburl string

	subdomAPIHost  string
	subdomAPIToken string

	ident string

	delegationMutex string

	delegationUser   string
	delegationServer string

	kdAddrUser   string
	kdAddrServer string
}

func main() {
	var w io.WriteCloser

	chunked, secondary, force, base32String, uuidString, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	opts, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	sshconf, err := kdlib.CreateSSHConfig(opts.sshKeyFilename, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	delegationSyncSSHconf, err := kdlib.CreateSSHConfig(opts.sshKeyFilename, opts.delegationUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("Can't create delegation sync ssh config: %s\n", err)
	}

	kdAddrSyncSSHconf, err := kdlib.CreateSSHConfig(opts.sshKeyFilename, opts.kdAddrUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("Can't create keydesk address ssh config: %s\n", err)
	}

	db, err := createDBPool(opts.dburl)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	controlIP, instanceID, err := getBrigadeControlIP(db, uuidString, secondary)
	if err != nil {
		log.Fatalf("%s: Can't get control ip: %s\n", LogTag, err)
	}

	orphan := false

	ee := &ssh.ExitError{}
	// attention! brigadeID - base32-style.
	output, err := revokeBrigade(sshconf, base32String, controlIP)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: Can't revoke brigade: %s\n", LogTag, err)
		if (!force && errors.Is(err, ErrVIPBrigade)) || errors.Is(err, ErrDeleteAttemptsCountExceeded) ||
			(errors.As(err, &ee) && ee.ExitStatus() == 2) {
			fmt.Fprintf(os.Stderr, "%s: Will not remove brigade from database\n", LogTag)

			os.Exit(2)
		}

		orphan = true
	}

	// attention! id - uuid-style string.
	num, err := removeBrigade(
		db,
		uuidString,
		instanceID,
		opts.subdomAPIHost, opts.subdomAPIToken,
		opts.ident,
		opts.delegationMutex,
		opts.delegationServer, delegationSyncSSHconf,
		opts.kdAddrServer, kdAddrSyncSSHconf,
		orphan,
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

var (
	ErrReservedSlot       = errors.New("reserved slot")
	ErrBlockedSlot        = errors.New("blocked slot")
	ErrNoDeletebleBrigade = errors.New("no deleteble brigade")
)

func getBrigadeControlIP(db *pgxpool.Pool, brigadeID string, secondary bool) (netip.Addr, uuid.UUID, error) {
	ctx := context.Background()

	var (
		controlIP     netip.Addr
		instanceID    uuid.UUID
		reservationID pgtype.UUID
		enabled       bool
	)

	tx, err := db.Begin(ctx)
	if err != nil {
		return controlIP, instanceID, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	if !secondary {
		sqlGetBrigadesCount := `
		SELECT
			COUNT(*)
		FROM
			%s
		WHERE
			brigade_id=$1
			AND main=false
		`

		var count int32

		if err := tx.QueryRow(ctx,
			fmt.Sprintf(sqlGetBrigadesCount, pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize()),
			brigadeID,
		).Scan(&count); err != nil {
			return controlIP, instanceID, fmt.Errorf("brigade count: %w", err)
		}

		if count > 0 {
			return controlIP, instanceID, fmt.Errorf("%w: %s", ErrNoDeletebleBrigade, brigadeID)
		}

	}

	sqlGetControlIP := `
	SELECT
		mb.control_ip,
		mb.instance_id,
		rei.reservation_id,
		pei.enabled
	FROM 
		brigades.meta_brigades AS mb
	LEFT JOIN 
		brigades.reserved_endpoints_ipv4 AS rei ON mb.endpoint_ipv4 = rei.endpoint_ipv4
	LEFT JOIN
		pairs.pairs_endpoints_ipv4 AS pei ON mb.endpoint_ipv4 = pei.endpoint_ipv4
	WHERE
		mb.brigade_id=$1
	AND
		mb.main=$2
	LIMIT 1
	`

	if err := tx.QueryRow(ctx,
		sqlGetControlIP,
		brigadeID,
		!secondary,
	).Scan(
		&controlIP,
		&instanceID,
		&reservationID,
		&enabled,
	); err != nil {
		return controlIP, instanceID, fmt.Errorf("brigade query: %w", err)
	}

	if reservationID.Valid {
		return controlIP, instanceID, fmt.Errorf("%w: %s (%s)", ErrReservedSlot, brigadeID, uuid.UUID(reservationID.Bytes).String())
	}

	if !enabled {
		return controlIP, instanceID, fmt.Errorf("%w: %s", ErrBlockedSlot, brigadeID)
	}

	return controlIP, instanceID, nil
}

func removeBrigade(
	db *pgxpool.Pool,
	brigadeID string,
	instanceID uuid.UUID,
	subdomAPIHost, subdomAPIToken string,
	ident string,
	delegationMutex string,
	delegationSyncServer string, delegationSyncSSHconf *ssh.ClientConfig,
	kdAddrSyncServer string, kdAddrSyncSSHconf *ssh.ClientConfig,
	orphan bool,
) (int32, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	if orphan {
		sqlSetOrphan := `INSERT INTO %s (endpoint_ipv4) SELECT endpoint_ipv4 FROM %s WHERE brigade_id=$1`
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			sqlSetOrphan,
			pgx.Identifier{defaultBrigadesSchema, "orphaned_endpoints_ipv4"}.Sanitize(),
			pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize(),
		), brigadeID); err != nil {
			return 0, fmt.Errorf("orphan insert: %w", err)
		}
	}

	sqlDelBrigadesStats := `
	DELETE
		FROM %s
	WHERE 
		brigade_id=$1
	AND
		instance_id=$2
	`

	if _, err := tx.Exec(ctx,
		fmt.Sprintf(sqlDelBrigadesStats, pgx.Identifier{defaultBrigadesStatsSchema, "brigades_stats"}.Sanitize()),
		brigadeID, instanceID); err != nil {
		return 0, fmt.Errorf("brigades stats delete: %w", err)
	}

	var domain_name pgtype.Text

	sqlDelBrigade := `
	DELETE 
		FROM %s
	WHERE 
		brigade_id=$1
	AND
		instance_id=$2
	RETURNING domain_name
	`

	if err := tx.QueryRow(
		ctx,
		fmt.Sprintf(sqlDelBrigade, pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize()),
		brigadeID, instanceID,
	).Scan(&domain_name); err != nil {
		return 0, fmt.Errorf("brigade delete: %w", err)
	}

	num := int32(0)

	if err := tx.QueryRow(ctx, kdlib.GetFreeSlotsNumberStatement(defaultBrigadesSchema, true)).Scan(&num); err != nil {
		return 0, fmt.Errorf("free slots query: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	if delegationMutex == "" {
		delegationMutex = DefaultDelegationMutexPath
	}

	dmu := lockedfile.MutexAt(delegationMutex)

	unlock, err := dmu.Lock()
	if err != nil {
		return 0, fmt.Errorf("lock delegation mutex: %w", err)
	}

	defer unlock()

	if domain_name.Valid {
		if err := revokeSubdomain(ctx, db, subdomAPIHost, subdomAPIToken, domain_name.String); err != nil {
			return 0, fmt.Errorf("revoke subdomain: %w", err)
		}
	}

	// Sync delegation list.

	delegationList, err := dcmgmt.NewDelegationList(ctx, db, defaultBrigadesSchema)
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

	kdAddrList, err := dcmgmt.NewKdAddrList(ctx, db, defaultBrigadesSchema)
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

func revokeSubdomain(ctx context.Context, db *pgxpool.Pool, subdomAPIHost, subdomAPIToken string, domain_name string) error {
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
		pgx.Identifier{defaultBrigadesSchema, "domains_endpoints_ipv4"}.Sanitize()),
		domain_name,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.ConstraintName {
			case "domain_name_fk":

				return nil
			}
		}

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

var (
	ErrDeleteAttemptsCountExceeded = errors.New("delete attempts count exceeded")
	ErrVIPBrigade                  = errors.New("vip brigade")
)

func revokeBrigade(sshconf *ssh.ClientConfig, brigadeID string, control_ip netip.Addr) ([]byte, error) {
	cmd := fmt.Sprintf("destroy -id %s -ch", brigadeID)

	fmt.Fprintf(os.Stderr, "%s: %s#%s:22 -> %s\n", LogTag, sshkeyRemoteUsername, control_ip, cmd)

	for attemts := 0; attemts <= deleteAttempts; attemts++ {
		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", control_ip), sshconf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ssh dial: %s", err)

			continue
		}

		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ssh session: %s", err)

			continue
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
			for line := range strings.SplitSeq(b.String(), "\n") {
				if strings.Contains(line, `"code": 403`) {
					return nil, fmt.Errorf("%w: %s", ErrVIPBrigade, line)
				}
			}

			return nil, fmt.Errorf("ssh run: %w", err)
		}

		return nil, nil
	}

	return nil, fmt.Errorf("%w: %d", ErrDeleteAttemptsCountExceeded, deleteAttempts)
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

func parseArgs() (bool, bool, bool, string, string, error) {
	brigadeID := flag.String("id", "", "brigadier_id in base32 form")
	brigadeUUID := flag.String("uuid", "", "brigadier_id in uuid form")
	chunked := flag.Bool("ch", false, "chunked output")
	secondary := flag.Bool("s", false, "secondary brigade")
	force := flag.Bool("f", false, "force delete even if brigade is vip")

	flag.Parse()

	switch {
	case *brigadeID != "" && *brigadeUUID == "":
		// brigadeID must be base32 decodable.
		buf, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*brigadeID)
		if err != nil {
			return false, false, false, "", "", fmt.Errorf("id base32: %s: %w", *brigadeID, err)
		}

		id, err := uuid.FromBytes(buf)
		if err != nil {
			return false, false, false, "", "", fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		return *chunked, *secondary, *force, *brigadeID, id.String(), nil
	case *brigadeUUID != "" && *brigadeID == "":
		id, err := uuid.Parse(*brigadeUUID)
		if err != nil {
			return false, false, false, "", "", fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		bid := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:])

		return *chunked, *secondary, *force, bid, id.String(), nil
	default:
		return false, false, false, "", "", fmt.Errorf("both ids: %w", errInlalidArgs)
	}
}

func readConfigs() (*opts, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshKeyDefaultPath)
	if err != nil {
		return nil, fmt.Errorf("ssh key: %w", err)
	}

	subdomainAPIHost := os.Getenv("SUBDOMAIN_API_SERVER")
	if subdomainAPIHost == "" {
		return nil, errors.New("empty subdomapi host")
	}

	if _, err := netip.ParseAddrPort(subdomainAPIHost); err != nil {
		return nil, fmt.Errorf("parse subdomapi host: %w", err)
	}

	subdomainAPIToken := os.Getenv("SUBDOMAIN_API_TOKEN")
	if subdomainAPIToken == "" {
		return nil, errors.New("empty subdomapi token")
	}

	_, ident, err := dcmgmt.ParseDCNameEnv()
	if err != nil {
		return nil, fmt.Errorf("dc name: %w", err)
	}

	delegationMutex := os.Getenv("DELEGATION_MUTEX")
	if delegationMutex == "" {
		delegationMutex = DefaultDelegationMutexPath
	}

	delegationUser, delegationServer, err := dcmgmt.ParseConnEnv("DELEGATION_SYNC_CONNECT")
	if err != nil {
		return nil, fmt.Errorf("delegation sync connect: %w", err)
	}

	kdAddrUser, kdAddrServer, err := dcmgmt.ParseConnEnv("KEYDESK_ADDRESS_SYNC_CONNECT")
	if err != nil {
		return nil, fmt.Errorf("keydesk address sync connect: %w", err)
	}

	return &opts{
			sshKeyFilename: sshKeyFilename,

			dburl: dbURL,

			subdomAPIHost:  subdomainAPIHost,
			subdomAPIToken: subdomainAPIToken,

			ident: ident,

			delegationMutex: delegationMutex,

			delegationUser:   delegationUser,
			delegationServer: delegationServer,

			kdAddrUser:   kdAddrUser,
			kdAddrServer: kdAddrServer,
		},
		nil
}
