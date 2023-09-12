package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/keydesk/gen/models"
	"github.com/vpngen/keydesk/keydesk"
	realmadmin "github.com/vpngen/realm-admin"
	"github.com/vpngen/realm-admin/internal/kdlib"

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
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

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

const defaultWireguardConfigs = "native"

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

	chunked, jout, brigadeID, id, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	sshKeyFilename, dbname, schema, opts, err := readConfigs()
	if err != nil {
		fatal(w, jout, "%s: Can't read configs: %s\n", LogTag, err)
	}

	sshconf, err := kdlib.CreateSSHConfig(sshKeyFilename, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		fatal(w, jout, "%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		fatal(w, jout, "%s: Can't create db pool: %s\n", LogTag, err)
	}

	// attention! id - uuid-style string.
	controlIP, keydeskIPv6, err := checkBrigade(db, schema, id)
	if err != nil {
		fatal(w, jout, "%s: Can't check brigade: %s\n", LogTag, err)
	}

	// attention! brigadeID - base32-style.
	wgconf, err := replaceBrigadier(db, schema, sshconf, brigadeID, controlIP, opts)
	if err != nil {
		fatal(w, jout, "%s: Can't replace brigadier: %s\n", LogTag, err)
	}

	switch jout {
	case true:
		answ := realmadmin.Answer{
			Answer: keydesk.Answer{
				Code:    http.StatusCreated,
				Desc:    http.StatusText(http.StatusCreated),
				Status:  keydesk.AnswerStatusSuccess,
				Configs: *wgconf,
			},
			KeydeskIPv6: keydeskIPv6,
		}

		payload, err := json.Marshal(answ)
		if err != nil {
			fatal(w, jout, "%s: Can't marshal answer: %s\n", LogTag, err)
		}

		if _, err := w.Write(payload); err != nil {
			fatal(w, jout, "%s: Can't write answer: %s\n", LogTag, err)
		}
	default:
		if _, err = fmt.Fprintln(w, keydeskIPv6.String()); err != nil {
			log.Fatalf("%s: Can't print keydesk ipv6: %s\n", LogTag, err)
		}

		if _, err := fmt.Fprintln(w, *wgconf.WireguardConfig.FileName); err != nil {
			log.Fatalf("%s: Can't print wgconf filename: %s\n", LogTag, err)
		}

		if _, err := fmt.Fprintln(w, *wgconf.WireguardConfig.FileContent); err != nil {
			log.Fatalf("%s: Can't print wgconf content: %s\n", LogTag, err)
		}
	}
}

const fatalString = `{
	"code" : 500,
	"desc" : "Internal Server Error",
	"status" : "error",
	"message" : "%s"
}`

func fatal(w io.Writer, jout bool, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)

	switch jout {
	case true:
		fmt.Fprintf(w, fatalString, msg)
	default:
		fmt.Fprint(w, msg)
	}

	log.Fatal(msg)
}

func checkBrigade(db *pgxpool.Pool, schema string, brigadeID string) (netip.Addr, netip.Addr, error) {
	ctx := context.Background()
	emptyIP := netip.Addr{}

	tx, err := db.Begin(ctx)
	if err != nil {
		return emptyIP, emptyIP, fmt.Errorf("begin: %w", err)
	}

	var (
		controlIP   netip.Addr
		keydeskIPv6 netip.Addr
	)

	err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlGetControlIP,
			(pgx.Identifier{schema, "meta_brigades"}.Sanitize()),
		),
		brigadeID,
	).Scan(
		&controlIP,
		&keydeskIPv6,
	)
	if err != nil {
		tx.Rollback(ctx)

		return emptyIP, emptyIP, fmt.Errorf("brigade query: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return emptyIP, emptyIP, fmt.Errorf("commit: %w", err)
	}

	return controlIP, keydeskIPv6, nil
}

func replaceBrigadier(db *pgxpool.Pool, schema string, sshconf *ssh.ClientConfig, brigadeID string, control_ip netip.Addr, opts vpnCfgs) (*models.Newuser, error) {
	cmd := fmt.Sprintf("replace -id %s -ch -j", brigadeID)

	if opts.wg != "" {
		cmd += fmt.Sprintf(" -wg %s", opts.wg)
	}

	if opts.ovc != "" {
		cmd += fmt.Sprintf(" -ovc %s", opts.ovc)
	}

	if opts.ipsec != "" {
		cmd += fmt.Sprintf(" -ipsec %s", opts.ipsec)
	}

	if opts.outline != "" {
		cmd += fmt.Sprintf(" -outline %s", opts.outline)
	}

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

	payload, err := io.ReadAll(httputil.NewChunkedReader(&b))
	if err != nil {
		return nil, fmt.Errorf("chunk read: %w", err)
	}

	wgconf := &keydesk.Answer{}
	if err := json.Unmarshal(payload, &wgconf); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return &wgconf.Configs, nil
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

func parseArgs() (bool, bool, string, string, error) {
	brigadeID := flag.String("id", "", "brigadier_id in base32 form")
	brigadeUUID := flag.String("uuid", "", "brigadier_id in uuid form")
	chunked := flag.Bool("ch", false, "chunked output")
	jsonOut := flag.Bool("j", false, "json output")

	flag.Parse()

	switch {
	case *brigadeID != "" && *brigadeUUID == "":
		// brigadeID must be base32 decodable.
		buf, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*brigadeID)
		if err != nil {
			return false, false, "", "", fmt.Errorf("id base32: %s: %w", *brigadeID, err)
		}

		id, err := uuid.FromBytes(buf)
		if err != nil {
			return false, false, "", "", fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		return *chunked, *jsonOut, *brigadeID, id.String(), nil
	case *brigadeUUID != "" && *brigadeID == "":
		id, err := uuid.Parse(*brigadeUUID)
		if err != nil {
			return false, false, "", "", fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
		}

		bid := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:])

		return *chunked, *jsonOut, bid, id.String(), nil
	default:
		return false, false, "", "", fmt.Errorf("both ids: %w", errInlalidArgs)
	}
}

type vpnCfgs struct {
	wg      string
	ovc     string
	ipsec   string
	outline string
}

func readConfigs() (string, string, string, vpnCfgs, error) {
	opts := vpnCfgs{}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	brigadeSchema := os.Getenv("BRIGADES_SCHEMA")
	if brigadeSchema == "" {
		brigadeSchema = defaultBrigadesSchema
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshkeyDefaultPath)
	if err != nil {
		return "", "", "", opts, fmt.Errorf("lookup for ssh key: %w", err)
	}

	opts.wg = os.Getenv("WIREGUARD_CONFIGS")
	if opts.wg == "" {
		opts.wg = defaultWireguardConfigs
	}

	opts.ovc = os.Getenv("OVC_CONFIGS")
	opts.ipsec = os.Getenv("IPSEC_CONFIGS")
	opts.outline = os.Getenv("OUTLINE_CONFIGS")

	return sshKeyFilename, dbURL, brigadeSchema, opts, nil
}
