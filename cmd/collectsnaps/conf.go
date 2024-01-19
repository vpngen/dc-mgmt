package main

import (
	"crypto/rsa"
	"flag"
	"fmt"
	"os"

	"github.com/vpngen/dc-mgmt/internal/kdlib"
	snapsCrypto "github.com/vpngen/keydesk-snap/core/crypto"
)

const (
	defaultPairsSchema         = "pairs"
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	DefaultRealmsKeysDir = "/etc/vg-keydesk-snap"
)

const (
	sshkeyRemoteUsername = "_onotole_"
	defautStoreDir       = "vg-snapshots"
)

const (
	defaultDatabaseURL = "postgresql:///vgrealm"
)

type config struct {
	dcName string
	dcID   string

	dbURL          string
	pairsSchema    string
	brigadesSchema string

	storageDir string
	tag        string
	addDate    bool
	replace    bool

	sshKeyFilename       string
	sshKeyRemoteUsername string

	realmFP        string
	realmsKeysPath string
	realmRSA       *rsa.PublicKey

	maintenanceMode int64

	cidrFilter string
}

var (
	ErrEmptyTag     = fmt.Errorf("empty tag")
	ErrEmptyRealmFP = fmt.Errorf("empty realm fingerprint")
	ErrUnknownDC    = fmt.Errorf("unknown dc")
)

func parseArgs(opts *config) error {
	tag := flag.String("tag", "", "snapshot tag")
	addDate := flag.Bool("ad", false, "add date to snapshot tag")
	replace := flag.Bool("r", false, "replace prev snapshot")
	maintenance := flag.Int64("mnt", 0, "maintenance mode")
	filter := flag.String("net", "", "filter by prefix")

	flag.Parse()

	if *tag == "" {
		return ErrEmptyTag
	}

	opts.tag = *tag
	opts.addDate = *addDate
	opts.replace = *replace

	opts.maintenanceMode = *maintenance

	opts.cidrFilter = *filter

	return nil
}

// readConfigs - reads configs from environment variables.
func readConfigs() (*config, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	pairsSchema := os.Getenv("PAIRS_SCHEMA")
	if pairsSchema == "" {
		pairsSchema = defaultPairsSchema
	}

	brigadesSchema := os.Getenv("BRIGADES_SCHEMA")
	if brigadesSchema == "" {
		brigadesSchema = defaultBrigadesSchema
	}

	storage := os.Getenv("SNAPSHOTS_BASE_DIR")
	if storage == "" {
		storage = defautStoreDir
	}

	dcID := os.Getenv("DC_ID")
	dcName := os.Getenv("DC_NAME")
	if dcID == "" || dcName == "" {
		return nil, fmt.Errorf("%w: id: %s, name: %s", ErrUnknownDC, dcID, dcName)
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), "")
	if err != nil {
		return nil, fmt.Errorf("ssh key: %w", err)
	}

	realmFP := os.Getenv("REALM_FP")
	if realmFP == "" {
		return nil, ErrEmptyRealmFP
	}

	realmsKeysPath := os.Getenv("REALMS_KEYS_PATH")
	if realmsKeysPath == "" {
		realmsKeysPath = DefaultRealmsKeysDir
	}

	realmRSA, err := snapsCrypto.FindPubKeyInFile(realmsKeysPath, realmFP)
	if err != nil {
		return nil, fmt.Errorf("realm key: %w", err)
	}

	return &config{
		dcName: dcName,
		dcID:   dcID,

		dbURL:          dbURL,
		pairsSchema:    pairsSchema,
		brigadesSchema: brigadesSchema,

		storageDir: storage,

		sshKeyFilename:       sshKeyFilename,
		sshKeyRemoteUsername: sshkeyRemoteUsername,

		realmFP:        realmFP,
		realmsKeysPath: realmsKeysPath,
		realmRSA:       realmRSA,
	}, nil
}
