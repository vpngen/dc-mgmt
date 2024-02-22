package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vpngen/dc-mgmt/internal/snap"
)

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

	stime := time.Now().UTC().Unix()
	snapFile := filepath.Join(opts.storageDir, fmt.Sprintf("brigades.snapshot.%s.json", opts.tag))

	psk, epsk, err := snap.GenPSK(opts.realmRSA)
	if err != nil {
		log.Fatalf("%s: Can't generate psk: %s\n", LogTag, err)
	}

	if err := pairsWalk(&walkConfig{
		snapFile: snapFile,
		stime:    stime,
		psk:      psk,
		epsk:     epsk,

		config: opts,
	}); err != nil {
		log.Fatalf("%s: Can't collect stats: %s\n", LogTag, err)
	}
}
