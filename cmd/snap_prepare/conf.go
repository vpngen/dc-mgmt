package main

import (
	"crypto/rsa"
	"errors"
	"flag"
	"fmt"
	"os"

	snapCrypto "github.com/vpngen/keydesk-snap/core/crypto"
)

const (
	defaultRealmPrivkeyFilename    = "/etc/vg-keydesk-snap/priv/realm.pem"
	defaultAuthoritiesKeysFilename = "/etc/vg-keydesk-snap/authorities_keys"
)

type cfg struct {
	force        bool   // default behavior is to exit if errors_count > 0
	realmKeyfile string // realm private key file
	authKeyfile  string // authorities keys file
	authFP       string // authority key fingerprint
}

type opts struct {
	force   bool
	privKey *rsa.PrivateKey
	pubKey  *rsa.PublicKey
}

var ErrEmptyFP = errors.New("empty authority fingerprint")

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

	pub, err := snapCrypto.FindPubKeyInFile(c.authKeyfile, c.authFP)
	if err != nil {
		return nil, fmt.Errorf("can't find public key: %w", err)
	}

	return &opts{
		force:   c.force,
		privKey: priv,
		pubKey:  pub,
	}, nil
}

func ckconfdefs(c *cfg) error {
	if c.authFP == "" {
		return ErrEmptyFP
	}

	if c.realmKeyfile == "" {
		c.realmKeyfile = defaultRealmPrivkeyFilename
	}

	if c.authKeyfile == "" {
		c.authKeyfile = defaultAuthoritiesKeysFilename
	}

	return nil
}

func parseArgs(c *cfg) error {
	force := flag.Bool("force", false, "force to continue if errors_count > 0")
	fp := flag.String("fp", "", "authority key fingerprint")
	rpk := flag.String("k", "", "realm private key file")
	ak := flag.String("a", "", "authorities keys file")

	flag.Parse()

	if *fp == "" {
		return ErrEmptyFP
	}

	c.force = *force
	c.realmKeyfile = *rpk
	c.authKeyfile = *ak
	c.authFP = *fp

	return nil
}

func readEnv(c *cfg) error {
	realmKeyfile := os.Getenv("REALM_PRIV_KEY_FILE")
	if realmKeyfile == "" {
		realmKeyfile = defaultRealmPrivkeyFilename
	}

	c.realmKeyfile = realmKeyfile

	authsKeysPath := os.Getenv("AUTHORITIES_KEYS_FILE")
	if authsKeysPath == "" {
		authsKeysPath = defaultAuthoritiesKeysFilename
	}

	c.authKeyfile = authsKeysPath

	return nil
}
