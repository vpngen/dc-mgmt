package main

import (
	"crypto/rsa"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vpngen/dc-mgmt/internal/kdlib"
	snapsCrypto "github.com/vpngen/keydesk-snap/core/crypto"

	keydeskStorage "github.com/vpngen/keydesk/keydesk/storage"
)

const (
	DefaultRealmsKeysDir = "/etc/vg-keydesk-snap"
)

const (
	defautStoreDir = "vg-snapshots"
)

type config struct {
	dcName string
	dcID   string

	storageDir string

	sshKeyFilename string

	realmFP        string
	realmsKeysPath string
	realmRSA       *rsa.PublicKey

	tag string

	BrigadeID string
}

var (
	ErrEmptyTag     = fmt.Errorf("empty tag")
	ErrEmptyRealmFP = fmt.Errorf("empty realm fingerprint")
	ErrUnknownDC    = fmt.Errorf("unknown dc")
)

func parseArgs(opts *config) error {
	tag := flag.String("tag", "test", "snapshot tag")

	flag.Parse()

	if *tag == "" {
		return ErrEmptyTag
	}

	opts.tag = *tag

	return nil
}

// readConfigs - reads configs from environment variables.
func readConfigs() (*config, error) {
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

	realmRSA, err := snapsCrypto.FindPubKeyInFile(filepath.Join(realmsKeysPath, snapsCrypto.DefaultRealmsKeysFileName), realmFP)
	if err != nil {
		return nil, fmt.Errorf("realm key: %w", err)
	}

	f, err := os.Open(filepath.Join(storage, "brigade.json"))
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	defer f.Close()

	data := keydeskStorage.Brigade{}
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	if data.BrigadeID == "" {
		return nil, fmt.Errorf("empty brigade id")
	}

	return &config{
		dcName: dcName,
		dcID:   dcID,

		storageDir: storage,

		sshKeyFilename: sshKeyFilename,

		realmFP:        realmFP,
		realmsKeysPath: realmsKeysPath,
		realmRSA:       realmRSA,

		BrigadeID: data.BrigadeID,
	}, nil
}
