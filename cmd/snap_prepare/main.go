package main

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	dcmgmt "github.com/vpngen/dc-mgmt"
	"golang.org/x/crypto/ssh"

	snapCore "github.com/vpngen/keydesk-snap/core"
	snapCrypto "github.com/vpngen/keydesk-snap/core/crypto"
)

var LogTag = setLogTag()

const defaultLogTag = "snap_prepare"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	opts, err := conf()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	if err := recode(opts); err != nil {
		log.Fatalf("%s: Can't recode: %s\n", LogTag, err)
	}
}

var (
	ErrInvalidSnapshotData = errors.New("invalid snapshot data")
	ErrSnapshotErrors      = errors.New("snapshot errors")
	ErrKeysMismatch        = errors.New("keys mismatch")

	ErrNoAuthorityKeyFP = errors.New("no authority key fingerprint")
)

func recode(o *opts) error {
	data := &dcmgmt.AggrSnaps{}

	if err := json.NewDecoder(os.Stdin).Decode(data); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if err := checkIn(data, o); err != nil {
		return fmt.Errorf("check in: %w", err)
	}

	if err := recodePSK(data, o.privKey, o.pubKey); err != nil {
		return fmt.Errorf("recode psk: %w", err)
	}

	for _, snapshot := range data.Snaps {
		if err := recodeLocker(snapshot, o.privKey, o.pubKey); err != nil {
			return fmt.Errorf("recode locker: %w", err)
		}
	}

	if err := json.NewEncoder(os.Stdout).Encode(data); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	return nil
}

func recodeLocker(data *snapCore.EncryptedBrigade, priv *rsa.PrivateKey, pub *rsa.PublicKey) error {
	esec, err := base64.StdEncoding.DecodeString(data.EncryptedLockerSecret)
	if err != nil {
		return fmt.Errorf("decode epsk: %w", err)
	}

	secret, err := snapCrypto.DecryptSecret(priv, esec)
	if err != nil {
		return fmt.Errorf("decrypt psk: %w", err)
	}

	esecRecoded, err := snapCrypto.EncryptSecret(pub, secret)
	if err != nil {
		return fmt.Errorf("encrypt epsk: %w", err)
	}

	pubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		return fmt.Errorf("new ssh public key: %w", err)
	}

	data.EncryptedLockerSecret = base64.StdEncoding.EncodeToString(esecRecoded)
	data.AuthorityKeyFP = ssh.FingerprintSHA256(pubKey)
	data.RealmKeyFP = ""

	if _, ok := data.Secrets[data.AuthorityKeyFP]; !ok {
		return fmt.Errorf("%w: %s", ErrNoAuthorityKeyFP, data.AuthorityKeyFP)
	}

	return nil
}

func recodePSK(data *dcmgmt.AggrSnaps, priv *rsa.PrivateKey, pub *rsa.PublicKey) error {
	epsk, err := base64.StdEncoding.DecodeString(data.EncryptedPreSharedSecret)
	if err != nil {
		return fmt.Errorf("decode epsk: %w", err)
	}

	psk, err := snapCrypto.DecryptSecret(priv, epsk)
	if err != nil {
		return fmt.Errorf("decrypt psk: %w", err)
	}

	epskRecoded, err := snapCrypto.EncryptSecret(pub, psk)
	if err != nil {
		return fmt.Errorf("encrypt epsk: %w", err)
	}

	pubKey, err := ssh.NewPublicKey(pub)
	if err != nil {
		return fmt.Errorf("new ssh public key: %w", err)
	}

	data.EncryptedPreSharedSecret = base64.StdEncoding.EncodeToString(epskRecoded)
	data.AuthorityKeyFP = ssh.FingerprintSHA256(pubKey)
	data.RealmKeyFP = ""

	return nil
}

func checkIn(data *dcmgmt.AggrSnaps, o *opts) error {
	if data.UpdateTime.IsZero() {
		return fmt.Errorf("%w: update time is zero", ErrInvalidSnapshotData)
	}

	if data.GlobalSnapAt.IsZero() {
		return fmt.Errorf("%w: global snap at is zero", ErrInvalidSnapshotData)
	}

	if data.Tag == "" {
		return fmt.Errorf("%w: empty tag", ErrInvalidSnapshotData)
	}

	if data.DatacenterID == "" {
		return fmt.Errorf("%w: empty datacenter id", ErrInvalidSnapshotData)
	}

	if data.RealmKeyFP == "" {
		return fmt.Errorf("%w: empty realm key fingerprint", ErrInvalidSnapshotData)
	}

	if data.AuthorityKeyFP != "" {
		return fmt.Errorf("%w: non-empty authority key fingerprint", ErrInvalidSnapshotData)
	}

	if data.EncryptedPreSharedSecret == "" {
		return fmt.Errorf("%w: empty encrypted pre-shared secret", ErrInvalidSnapshotData)
	}

	if len(data.Snaps) == 0 {
		return fmt.Errorf("%w: empty snaps", ErrInvalidSnapshotData)
	}

	if data.ErrorsCount > 0 {
		fmt.Fprintf(os.Stderr, "%s: errors count: %d\n", LogTag, data.ErrorsCount)

		if o.force {
			return fmt.Errorf("%w: %d", ErrSnapshotErrors, data.ErrorsCount)
		}
	}

	sshPub, err := ssh.NewPublicKey(o.privKey.Public())
	if err != nil {
		return fmt.Errorf("new ssh public key: %w", err)
	}

	if ssh.FingerprintSHA256(sshPub) != data.RealmKeyFP {
		return fmt.Errorf("realm: %w: %s != %s", ErrKeysMismatch,
			ssh.FingerprintSHA256(sshPub), data.RealmKeyFP)
	}

	return nil
}
