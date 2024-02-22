package main

import (
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"github.com/vpngen/dc-mgmt/internal/snap"
)

// BrigadeGroup - brigades in the same pair.
type BrigadeGroup struct {
	ConnectAddr netip.Addr
	Brigades    [][]byte
}

// GroupsList - list of brigades groups.
type GroupsList []BrigadeGroup

var LogTag = setLogTag()

const defaultLogTag = "collectsnaps"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	opts, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	if err := parseArgs(opts); err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	sshconf, err := kdlib.CreateSSHConfig(opts.sshKeyFilename, opts.sshKeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	db, err := kdlib.CreateDBPool(opts.dbURL)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	baseTag, stime := adjustTag(opts)
	snapFile, err := composeFilename(opts.storageDir, baseTag, opts.tag)
	if err != nil {
		log.Fatalf("%s: Can't compose filename: %s\n", LogTag, err)
	}

	psk, epsk, err := snap.GenPSK(opts.realmRSA)
	if err != nil {
		log.Fatalf("%s: Can't generate psk: %s\n", LogTag, err)
	}

	if err := pairsWalk(&walkConfig{
		db:      db,
		sshconf: sshconf,

		snapFile: snapFile,
		stime:    stime,
		psk:      psk,
		epsk:     epsk,

		config: opts,
	}); err != nil {
		log.Fatalf("%s: Can't collect stats: %s\n", LogTag, err)
	}

	if err := rotateSnapshots(opts.storageDir, baseTag, opts.tag); err != nil {
		log.Fatalf("%s: Can't rotate snapshots: %s\n", LogTag, err)
	}
}

// adjustTag - adjust tag with date if needed.
// Returns base tag and start time.
func adjustTag(opts *config) (string, int64) {
	stime := time.Now().UTC()

	baseTag := opts.tag

	if opts.addDate {
		opts.tag = fmt.Sprintf("%s-%s", opts.tag, stime.Format("20060102-150405"))
	}

	return baseTag, stime.Unix()
}

// composeFilename - compose filename from tag.
func composeFilename(basePath, baseTag, tag string) (string, error) {
	path := filepath.Join(basePath, baseTag)
	fn := fmt.Sprintf("%s.json", tag)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return "", fmt.Errorf("create path: %w", err)
		}
	}

	return filepath.Join(path, fn), nil
}

// rotateSnapshots - rotate snapshots.
func rotateSnapshots(basePath, baseTag, tag string) error {
	path := filepath.Join(basePath, baseTag)
	fn := fmt.Sprintf("%s.json", tag)

	list, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}

	for _, f := range list {
		if f.Name() == fn || f.Name() == "." || f.Name() == ".." {
			continue
		}

		if err := os.Remove(filepath.Join(path, f.Name())); err != nil {
			return fmt.Errorf("remove file: %w", err)
		}
	}

	return nil
}
