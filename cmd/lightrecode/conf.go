package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vpngen/vpngine/naclkey"
)

const (
	defaultRouterKeyFilename = "router_pub.json"
	defaultMasterKeyFilename = "shuffler_priv.json"
)

type cfg struct {
	routerKey         string
	masterPrivKeyfile string

	infile  string
	outfile string

	home string
}

type opts struct {
	routerKey     [naclkey.NaclBoxKeyLength]byte
	masterPrivKey naclkey.NaclBoxKeypair

	infile  string
	outfile string
}

var (
	ErrEmptyInfile  = errors.New("empty in file")
	ErrEmptyOutfile = errors.New("empty out file")

	ErrNoRouterKey     = errors.New("no router key")
	ErrNoMasterPrivKey = errors.New("no master private key")
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

	routerKey, err := naclkey.ReadPublicKeyFile(c.routerKey)
	if err != nil {
		return nil, fmt.Errorf("can't read router key: %w", err)
	}

	masterPrivKey, err := naclkey.ReadKeypairFile(c.masterPrivKeyfile)
	if err != nil {
		return nil, fmt.Errorf("can't read master private key: %w", err)
	}

	return &opts{
		routerKey:     routerKey,
		masterPrivKey: masterPrivKey,

		infile:  c.infile,
		outfile: c.outfile,
	}, nil
}

func ckconfdefs(c *cfg) error {
	if c.infile == "" {
		return ErrEmptyInfile
	}

	if c.outfile == "" {
		return ErrEmptyOutfile
	}

	if c.routerKey == "" {
		return ErrNoRouterKey
	}

	if c.masterPrivKeyfile == "" {
		return ErrNoMasterPrivKey
	}

	return nil
}

func parseArgs(c *cfg) error {
	rkey := flag.String("rk", filepath.Join(c.home, ".secret", defaultRouterKeyFilename), "actual router nacl key.")
	mkey := flag.String("mk", filepath.Join(c.home, ".secret", defaultMasterKeyFilename), "master private nacl key.")
	insnap := flag.String("in", "-", "input brigade file. Default: - (stdin)")
	outplan := flag.String("out", "-", "output brigade file. Default: - (stdout)")

	flag.Parse()

	c.routerKey = *rkey
	c.masterPrivKeyfile = *mkey
	c.infile = *insnap
	c.outfile = *outplan

	return nil
}

func readEnv(c *cfg) error {
	mkey := os.Getenv("MASTER_PRIV_KEY_FILE")
	if mkey != "" {
		c.masterPrivKeyfile = mkey
	}

	var err error

	home := os.Getenv("HOME")
	if home == "" {
		home, err = filepath.Abs(".")
		if err != nil {
			return fmt.Errorf("can't get home dir: %w", err)
		}
	}

	c.home = home

	return nil
}
