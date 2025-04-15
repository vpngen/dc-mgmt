package main

import (
	"context"
	"crypto/rsa"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	snapCrypto "github.com/vpngen/keydesk-snap/core/crypto"
	"golang.org/x/crypto/ssh"
)

const (
	defaultRealmPrivkeyFilename = "/etc/vg-dc-snaps/priv/realm.pem"
)

const (
	sshkeyRemoteUsername = "_tolik_"
	sshkeyDefaultPath    = "/etc/vg-dc-vpnapi"
)

const (
	defaultPairsSchema         = "pairs"
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	defaultDatabaseURL = "postgresql:///vgrealm"
)

type cfg struct {
	realmKeyfile string // realm private key file

	reservationID string // reservation id

	planfile string // plan file

	inet string // control network for filtering inside reservation
	enet string // external network for filtering inside reservation

	sshKeyFilename string // ssh key filename

	dburl string

	onlyBase bool
	patch    bool // replication (patch)
}

type opts struct {
	privKey *rsa.PrivateKey

	inet netip.Prefix
	enet netip.Prefix

	reservationID string

	planfile string

	sshconf *ssh.ClientConfig

	db *pgxpool.Pool

	onlyBase bool
	patch    bool // replication (patch)
}

var (
	ErrEmptyReservationID = errors.New("empty reservation id")
	ErrEmptyPlanFile      = errors.New("empty plan file")
	ErrNoSSHKeyFile       = errors.New("no ssh key file")
)

func conf() (*opts, error) {
	c := &cfg{}

	if err := readEnv(c); err != nil {
		return nil, fmt.Errorf("can't read env: %w", err)
	}

	if err := parseArgs(c); err != nil {
		return nil, fmt.Errorf("can't parse args: %w", err)
	}

	if err := ckconfdefs(c); err != nil {
		return nil, fmt.Errorf("config check failed: %w", err)
	}

	priv, err := snapCrypto.ReadPrivateSSHKeyFile(c.realmKeyfile)
	if err != nil {
		return nil, fmt.Errorf("can't read private key: %w", err)
	}

	inet, err := netip.ParsePrefix(c.inet)
	if err != nil {
		return nil, fmt.Errorf("can't parse control network: %w", err)
	}

	enet, err := netip.ParsePrefix(c.enet)
	if err != nil {
		return nil, fmt.Errorf("can't parse external network: %w", err)
	}

	sshconf, err := kdlib.CreateSSHConfig(c.sshKeyFilename, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		return nil, fmt.Errorf("can't create ssh configs: %w", err)
	}

	ctx := context.Background()

	db, err := kdlib.CreateDBPool(ctx, c.dburl)
	if err != nil {
		return nil, fmt.Errorf("can't create db pool: %w", err)
	}

	return &opts{
		privKey: priv,
		inet:    inet,
		enet:    enet,

		reservationID: c.reservationID,

		planfile: c.planfile,

		sshconf: sshconf,

		db: db,

		onlyBase: c.onlyBase,
	}, nil
}

func ckconfdefs(c *cfg) error {
	if c.realmKeyfile == "" {
		c.realmKeyfile = defaultRealmPrivkeyFilename
	}

	if c.reservationID == "" {
		return ErrEmptyReservationID
	}

	if c.planfile == "" {
		return ErrEmptyPlanFile
	}

	if c.sshKeyFilename == "" {
		return ErrNoSSHKeyFile
	}

	return nil
}

func parseArgs(c *cfg) error {
	rpk := flag.String("k", "", "realm private key file")
	reservation := flag.String("r", "", "reservation or replication id")
	replication := flag.Bool("p", false, "replication (patch)")
	inet := flag.String("inet", "0.0.0.0/0", "control network for filtering inside reservation")
	enet := flag.String("enet", "0.0.0.0/0", "external network for filtering inside reservation")
	planfile := flag.String("f", "", "plan file")
	sshKeyFilename := flag.String("s", "", "ssh key filename")
	onlyBase := flag.Bool("o", false, "only base")

	flag.Parse()

	if *reservation == "" {
		return ErrEmptyReservationID
	}

	if *planfile == "" {
		return ErrEmptyPlanFile
	}

	if *sshKeyFilename != "" {
		c.sshKeyFilename = *sshKeyFilename
	}

	c.realmKeyfile = *rpk
	c.reservationID = *reservation
	c.planfile = *planfile
	c.inet = *inet
	c.enet = *enet
	c.onlyBase = *onlyBase
	c.patch = *replication

	return nil
}

func readEnv(c *cfg) error {
	realmKeyfile := os.Getenv("REALM_PRIV_KEY_FILE")
	if realmKeyfile == "" {
		realmKeyfile = defaultRealmPrivkeyFilename
	}

	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	c.dburl = dbURL

	c.realmKeyfile = realmKeyfile

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshkeyDefaultPath)
	if err == nil {
		c.sshKeyFilename = sshKeyFilename
	}

	return nil
}
