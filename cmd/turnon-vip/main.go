package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"

	"golang.org/x/crypto/ssh"
)

const (
	defaultBrigadesSchema = "brigades"
)

const (
	sshkeyRemoteUsername = "_serega_"
	sshkeyDefaultPath    = "/etc/vg-dc-vpnapi"
)

const (
	defaultDatabaseURL = "postgresql:///vgrealm"
)

var errInlalidArgs = errors.New("invalid args")

var LogTag = setLogTag()

const defaultLogTag = "turnon-vip"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	brigadeID, id, vipon, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	sshKeyFilename, dbname, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	sshconf, err := kdlib.CreateSSHConfig(sshKeyFilename, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	// attention! id - uuid-style string.
	controlIP, err := checkBrigade(db, id)
	if err != nil {
		log.Fatalf("%s: Can't check brigade: %s\n", LogTag, err)
	}

	// attention! brigadeID - base32-style.
	if err := viparize(sshconf, brigadeID, controlIP, vipon); err != nil {
		log.Fatalf("%s: Can't viparize: %s\n", LogTag, err)
	}
}

func checkBrigade(db *pgxpool.Pool, brigadeID string) (netip.Addr, error) {
	ctx := context.Background()
	emptyIP := netip.Addr{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return emptyIP, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	var controlIP netip.Addr

	sqlGetControlIP := `
	SELECT
		control_ip
	FROM %s
	WHERE
		brigade_id=$1
	AND
		main=true
	`

	if err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlGetControlIP,
			(pgx.Identifier{defaultBrigadesSchema, "meta_brigades"}.Sanitize()),
		),
		brigadeID,
	).Scan(
		&controlIP,
	); err != nil {
		return emptyIP, fmt.Errorf("brigade query: %w", err)
	}

	return controlIP, nil
}

func viparize(sshconf *ssh.ClientConfig, brigadeID string, control_ip netip.Addr, vipon bool) error {
	cmd := fmt.Sprintf("vipoff -id %s", brigadeID)
	if vipon {
		cmd = fmt.Sprintf("vipon -id %s", brigadeID)
	}

	fmt.Fprintf(os.Stderr, "%s: %s#%s:22 -> %s\n", LogTag, sshkeyRemoteUsername, control_ip, cmd)

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", control_ip), sshconf)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
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
		return fmt.Errorf("ssh run: %w", err)
	}

	return nil
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

var ErrOnlyOneAction = errors.New("only one action: on or off")

func parseArgs() (string, string, bool, error) {
	brigadeID := flag.String("id", "", "brigadier_id in base32 form")
	brigadeUUID := flag.String("uuid", "", "brigadier_id in uuid form")
	actionOn := flag.Bool("on", false, "set vip brigade")
	actionOff := flag.Bool("off", false, "unset vip brigade")

	flag.Parse()

	if (*actionOn && *actionOff) || (!*actionOn && !*actionOff) {
		return "", "", false, ErrOnlyOneAction
	}

	switch {
	case *brigadeID != "" && *brigadeUUID == "":
		// brigadeID must be base32 decodable.
		buf, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*brigadeID)
		if err != nil {
			return "", "", false, fmt.Errorf("id base32: %s: %w", *brigadeID, err)
		}

		id, err := uuid.FromBytes(buf)
		if err != nil {
			return "", "", false, fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		return *brigadeID, id.String(), *actionOn, nil
	case *brigadeUUID != "" && *brigadeID == "":
		id, err := uuid.Parse(*brigadeUUID)
		if err != nil {
			return "", "", false, fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		bid := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:])

		return bid, id.String(), *actionOn, nil
	default:
		return "", "", false, fmt.Errorf("both ids: %w", errInlalidArgs)
	}
}

func readConfigs() (string, string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshkeyDefaultPath)
	if err != nil {
		return "", "", fmt.Errorf("lookup for ssh key: %w", err)
	}

	return sshKeyFilename, dbURL, nil
}
