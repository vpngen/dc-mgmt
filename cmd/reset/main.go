package main

import (
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"

	"github.com/vpngen/keydesk/keydesk"
	"github.com/vpngen/keydesk/keydesk/storage"
	"github.com/vpngen/vpngine/naclkey"
	"golang.org/x/crypto/nacl/box"
)

var (
	// ErrInvalidArgs - invalid arguments.
	ErrInvalidArgs = errors.New("invalid arguments")

	// ErrCantDecrypt - can't decrypt.
	ErrCantDecrypt = errors.New("can't decrypt")
)

func main() {
	shufflerFile, routerFile, brigadeID, dbFile, epAddr, err := parseArgs()
	if err != nil {
		log.Fatalf("Can't init: %s\n", err)
	}

	fmt.Fprintf(os.Stderr, "Brigade: %s\n", brigadeID)
	fmt.Fprintf(os.Stderr, "DBFile: %s\n", dbFile)
	if epAddr.IsValid() {
		fmt.Fprintf(os.Stderr, "New Endpoint IPv4: %s\n", epAddr)
	}

	routerKey, shufflerKeys, err := readKeys(routerFile, shufflerFile)
	if err != nil {
		log.Fatalf("Can't read keys: %s\n", err)
	}

	db := &storage.BrigadeStorage{
		BrigadeID:       brigadeID,
		BrigadeFilename: dbFile,
		BrigadeSpinlock: dbFile + ".lock",
		APIAddrPort:     netip.AddrPort{},
		BrigadeStorageOpts: storage.BrigadeStorageOpts{
			MaxUsers:               keydesk.MaxUsers,
			MonthlyQuotaRemaining:  keydesk.MonthlyQuotaRemaining,
			MaxUserInctivityPeriod: keydesk.DefaultMaxUserInactivityPeriod,
		},
	}

	if err := db.SelfCheckAndInit(); err != nil {
		log.Fatalf("Storage initialization: %s\n", err)
	}

	if err := Do(db, routerKey, shufflerKeys, epAddr); err != nil {
		log.Fatalf("Do: %s\n", err)
	}
}

func parseArgs() (string, string, string, string, netip.Addr, error) {
	brigadeID := flag.String("id", "", "BrigadeID")
	dbFile := flag.String("f", "", "Brigade json file")
	endpointIPv4 := flag.String("ip", "", "Endpoint IPv4 address")
	shufflerFile := flag.String("sk", "", "Shuffler private key file")
	routerFile := flag.String("rk", "", "Router public key file")

	flag.Parse()

	if *endpointIPv4 != "" {
		epAddr, err := netip.ParseAddr(*endpointIPv4)
		if err != nil {
			return "", "", "", "", netip.Addr{}, fmt.Errorf("parse endpoint ip: %w", err)
		}

		if epAddr.IsValid() && epAddr.IsUnspecified() {
			return "", "", "", "", netip.Addr{}, fmt.Errorf("invalid endpoint ip: %w", ErrInvalidArgs)
		}

		return *shufflerFile, *routerFile, *brigadeID, *dbFile, epAddr, nil
	}

	return *shufflerFile, *routerFile, *brigadeID, *dbFile, netip.Addr{}, nil
}

// Do - do replay.
func Do(db *storage.BrigadeStorage, routerKey *[naclkey.NaclBoxKeyLength]byte, shufflerKeys *naclkey.NaclBoxKeypair, epAddr netip.Addr) error {
	f, data, err := db.OpenDbToModify()
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}

	defer f.Close()

	if epAddr.IsValid() {
		data.EndpointIPv4 = epAddr
	}

	wgPrivateRouterEnc, err := reEncrypt(routerKey, shufflerKeys, data.WgPrivateShufflerEnc)
	if err != nil {
		return fmt.Errorf("re-encrypt wg private: %w", err)
	}

	data.WgPrivateRouterEnc = wgPrivateRouterEnc

	for _, user := range data.Users {
		wgPSKRouterEnc, err := reEncrypt(routerKey, shufflerKeys, user.WgPSKShufflerEnc)
		if err != nil {
			return fmt.Errorf("re-encrypt wg psk: %w", err)
		}

		user.WgPSKRouterEnc = wgPSKRouterEnc
	}

	if err := f.Commit(data); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func reEncrypt(routerKey *[naclkey.NaclBoxKeyLength]byte, shufflerKeys *naclkey.NaclBoxKeypair, payload []byte) ([]byte, error) {
	fmt.Printf("Re-encrypting %d bytes\n", len(payload))

	fmt.Printf("Router key: %x\n", *routerKey)
	fmt.Printf("Shuffler public key: %x\n", shufflerKeys.Public)
	fmt.Printf("Shuffler private key: %x\n", shufflerKeys.Private)

	fmt.Printf("Payload: %x\n", payload)

	decrypted, ok := box.OpenAnonymous(nil, payload, &shufflerKeys.Public, &shufflerKeys.Private)

	if !ok {
		return nil, fmt.Errorf("open: %w", ErrCantDecrypt)
	}

	reEncrypted, err := box.SealAnonymous(nil, decrypted, routerKey, rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("seal: %w", err)
	}

	return reEncrypted, nil
}

func readKeys(routerFile, shufflerFile string) (*[naclkey.NaclBoxKeyLength]byte, *naclkey.NaclBoxKeypair, error) {
	routerKey, err := naclkey.ReadPublicKeyFile(routerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("router key: %w", err)
	}

	shufflerKeys, err := naclkey.ReadKeypairFile(shufflerFile)
	if err != nil {
		return nil, nil, fmt.Errorf("shuffler key: %w", err)
	}

	fmt.Printf("Router key: %x\n", routerKey)
	fmt.Printf("Shuffler public key: %x\n", shufflerKeys.Public)
	fmt.Printf("Shuffler private key: %x\n", shufflerKeys.Private)

	return &routerKey, &shufflerKeys, nil
}
