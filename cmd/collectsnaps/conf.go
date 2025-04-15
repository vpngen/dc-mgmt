package main

import (
	"crypto/rsa"
	"encoding/json"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	dcroot "github.com/vpngen/dc-mgmt"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	snapsCrypto "github.com/vpngen/keydesk-snap/core/crypto"
)

const (
	defaultPairsSchema         = "pairs"
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	DefaultRealmsKeysDir = "/etc/vg-dc-snaps"
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

	extFilter  []netip.Prefix
	ctrlFilter []netip.Prefix

	keep rotateConfig

	plan *dcroot.SnapshotPlan
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
	extFilter := flag.String("net", "", "filter by prefix")
	ctrlFilter := flag.String("ctrl", "", "filter by control nodes")
	plan := flag.String("plan", "", "plan file")

	keepDaily := flag.Int("keep-daily", 0, "keep daily snapshots")
	keepWeekly := flag.Int("keep-weekly", 0, "keep weekly snapshots")
	keepMonthly := flag.Int("keep-monthly", 0, "keep monthly snapshots")
	keepYearly := flag.Int("keep-yearly", 0, "keep yearly snapshots")
	keepHourly := flag.Int("keep-hourly", 0, "keep hourly snapshots")
	keepLast := flag.Int("keep-last", 0, "keep last snapshots")
	keepWithin := flag.String("keep-within", "", "keep snapshots within duration (1h, 1d, 1w, 1m, 1y)")

	flag.Parse()

	opts.keep = rotateConfig{
		keepDaily:   *keepDaily,
		keepWeekly:  *keepWeekly,
		keepMonthly: *keepMonthly,
		keepYearly:  *keepYearly,
		keepHourly:  *keepHourly,
		keepLast:    *keepLast,
	}

	if *keepWithin != "" {
		dur, err := parseDuration(*keepWithin)
		if err != nil {
			return fmt.Errorf("parse keep-within: %w", err)
		}

		opts.keep.keepWithin = dur
	}

	if *tag == "" {
		return ErrEmptyTag
	}

	opts.tag = *tag
	opts.addDate = *addDate
	opts.replace = *replace

	opts.maintenanceMode = *maintenance

	ef, err := getFilter(*extFilter)
	if err != nil {
		return fmt.Errorf("get ext filter: %w", err)
	}

	cf, err := getFilter(*ctrlFilter)
	if err != nil {
		return fmt.Errorf("get ctrl filter: %w", err)
	}

	opts.extFilter = ef
	opts.ctrlFilter = cf

	opts.plan, err = readPlan(*plan, ef, cf)
	if err != nil {
		return fmt.Errorf("read plan: %w", err)
	}

	return nil
}

func readPlan(plan string, extPrefixes, ctrlPrefixes []netip.Prefix) (*dcroot.SnapshotPlan, error) {
	if plan == "" {
		return nil, nil
	}

	f, err := os.OpenFile(plan, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}

	defer f.Close()

	var p dcroot.SnapshotPlan

	dec := json.NewDecoder(f)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("decode plan: %w", err)
	}

	for i, s := range p.Plan {
		cip, _ := netip.ParseAddr(s.ControlIP)
		if !inPrexixes(ctrlPrefixes, cip) {
			p.Plan = append(p.Plan[:i], p.Plan[i+1:]...)

			fmt.Fprintf(os.Stderr, "control ip %s is not in the control prefix\n", cip)

			continue
		}

		for j, r := range s.Snaps {
			rip, _ := netip.ParseAddr(r.EndpointIPv4)
			if !inPrexixes(extPrefixes, rip) {
				p.Plan[i].Snaps = append(p.Plan[i].Snaps[:j], p.Plan[i].Snaps[j+1:]...)

				fmt.Fprintf(os.Stderr, "endpoint ip %s is not in the external prefix\n", rip)
			}
		}
	}

	return &p, nil
}

func inPrexixes(prefixes []netip.Prefix, addr netip.Addr) bool {
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}

	return false
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

	realmRSA, err := snapsCrypto.FindPubKeyInFile(filepath.Join(realmsKeysPath, snapsCrypto.DefaultRealmsKeysFileName), realmFP)
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

func getFilter(filter string) ([]netip.Prefix, error) {
	filter = strings.TrimSpace(filter)

	if filter == "" {
		return []netip.Prefix{netip.PrefixFrom(netip.IPv4Unspecified(), 0)}, nil
	}

	nets := strings.Split(filter, ",")

	prefixes := make([]netip.Prefix, 0, len(nets))

	for _, p := range nets {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("parse prefix: %w", err)
		}

		prefixes = append(prefixes, prefix)
	}

	return prefixes, nil
}

var (
	ErrUnknownDurationUnit   = fmt.Errorf("unknown duration unit")
	ErrInvalidDurationFormat = fmt.Errorf("invalid duration format")
)

func parseDuration(durationStr string) (time.Duration, error) {
	var (
		duration time.Duration
		err      error
	)

	durationStr = strings.TrimSpace(durationStr)
	if durationStr == "" {
		return 0, nil
	}

	// Remove spaces and convert to lowercase
	re := regexp.MustCompile(`(\d+)([hdwmy])`)
	matches := re.FindStringSubmatch(durationStr)

	if len(matches) != 3 {
		return 0, fmt.Errorf("%w: %s", ErrInvalidDurationFormat, durationStr)
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, err
	}

	switch matches[2] {
	case "h":
		duration = time.Hour * time.Duration(value)
	case "d":
		duration = time.Hour * 24 * time.Duration(value)
	case "w":
		duration = time.Hour * 24 * 7 * time.Duration(value)
	case "m":
		duration = time.Hour * 24 * 30 * time.Duration(value) // Approximate month as 30 days
	case "y":
		duration = time.Hour * 24 * 365 * time.Duration(value) // Approximate year as 365 days
	default:
		return 0, fmt.Errorf("%w: %s", ErrUnknownDurationUnit, matches[2])
	}

	return duration, nil
}
