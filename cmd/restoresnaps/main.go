package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	dcmgmt "github.com/vpngen/dc-mgmt"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"github.com/vpngen/keydesk/keydesk/storage"
	"github.com/vpngen/wordsgens/namesgenerator"
	"golang.org/x/crypto/ssh"

	snapCrypto "github.com/vpngen/keydesk-snap/core/crypto"
	snapSnap "github.com/vpngen/keydesk-snap/core/snap"

	"github.com/vpngen/keydesk/keydesk"
)

var LogTag = setLogTag()

const defaultLogTag = "restoresnaps"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

var (
	ErrInvalidSnapshotData = errors.New("invalid snapshot data")
	ErrKeysMismatch        = errors.New("keys mismatch")
)

func main() {
	opts, err := conf()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	if err := restore(opts); err != nil {
		log.Fatalf("%s: Can't recode: %s\n", LogTag, err)
	}
}

func restore(o *opts) error {
	plan := &dcmgmt.RestorePlan{}

	f, err := os.Open(o.planfile)
	if err != nil {
		return fmt.Errorf("open plan file: %w", err)
	}

	defer f.Close()

	if err := json.NewDecoder(f).Decode(plan); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if err := checkPlan(plan, o); err != nil {
		return fmt.Errorf("check in: %w", err)
	}

	// TODO: check reservation id

CTRL:
	for _, controls := range plan.Plan {
		caddr, err := netip.ParseAddr(controls.ControlIP)
		if err != nil {
			return fmt.Errorf("parse control ip: %w", err)
		}

		if !o.inet.Contains(caddr) {
			continue
		}

		controlPlan := &dcmgmt.ControlNodeRestorePlan{
			Plan: make([]*storage.Brigade, 0, len(controls.Snaps)),
		}

		for _, snap := range controls.Snaps {
			brigade, err := restoreSnap(&snap, o, caddr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: control_ip: %s restore snap: %s\n", controls.ControlIP, err)

				continue CTRL
			}

			controlPlan.Plan = append(controlPlan.Plan, brigade)
		}

		if o.onlyBase {
			continue
		}

		// request control to restore
		cleanup, err := putBrigadesBySSH(o.sshconf, caddr, *controlPlan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: control_ip: %s put brigades: %s\n", controls.ControlIP, err)
		}

		cleanup(LogTag + "|" + caddr.String())
	}

	return nil
}

// putBrigadesBySSH - put brigades by ssh.
func putBrigadesBySSH(sshconf *ssh.ClientConfig, addr netip.Addr, plan dcmgmt.ControlNodeRestorePlan) (func(string), error) {
	cmd := "restorebrigades"

	fmt.Fprintf(os.Stderr, "%s#%s:22 -> %s\n", sshkeyRemoteUsername, addr, cmd)

	client, b, e, cleanup, err := kdlib.NewSSHCient(sshconf, addr.String()+":22")
	if err != nil {
		return cleanup, fmt.Errorf("new ssh client: %w", err)
	}

	defer client.Close()

	data, err := json.Marshal(plan)
	if err != nil {
		return cleanup, fmt.Errorf("marshal plan: %w", err)
	}

	if err := kdlib.SSHSessionStart(client, b, e, cmd, bytes.NewReader(data)); err != nil {
		return cleanup, fmt.Errorf("write remote file: %w", err)
	}

	return cleanup, nil
}

func restoreSnap(snap *dcmgmt.PreparedSnap, o *opts, caddr netip.Addr) (*storage.Brigade, error) {
	if snap == nil {
		return nil, fmt.Errorf("%w: nil snap", ErrInvalidSnapshotData)
	}

	addr, err := netip.ParseAddr(snap.EndpointIPv4)
	if err != nil {
		return nil, fmt.Errorf("parse ip: %w", err)
	}

	if !o.enet.Contains(addr) {
		return nil, nil
	}

	esecret, err := base64.StdEncoding.DecodeString(snap.EncryptedSecret)
	if err != nil {
		return nil, fmt.Errorf("decode secret: %w", err)
	}

	secret, err := snapCrypto.DecryptSecret(o.privKey, esecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt psk: %w", err)
	}

	finalsecret := make([]byte, 0, len([]byte(snap.BrigadeID))+len([]byte(o.reservationID))+len(secret))
	finalsecret = fmt.Append(finalsecret, snap.BrigadeID, o.reservationID, secret)

	dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(snap.Payload))

	buf, err := snapSnap.DecryptDecompressSnapshot(dec, finalsecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt decompress snapshot: %w", err)
	}

	brigade := &storage.Brigade{}

	if err := json.Unmarshal(buf, brigade); err != nil {
		return nil, fmt.Errorf("unmarshal brigade: %w", err)
	}

	if err := checkBrigadeConfig(brigade, snap.BrigadeID, snap.EndpointIPv4); err != nil {
		return nil, fmt.Errorf("check brigade: %w", err)
	}

	// recreateBrigade
	if err := recreateBrigade(o.db, o.reservationID, brigade, caddr, addr); err != nil {
		return nil, fmt.Errorf("recreate brigade: %w", err)
	}

	return brigade, nil
}

func checkBrigadeConfig(data *storage.Brigade, brigadeID string, endpointIPv4 string) error {
	if data == nil {
		return fmt.Errorf("%w: nil brigade", ErrInvalidSnapshotData)
	}

	if data.BrigadeID != brigadeID {
		return fmt.Errorf("%w: %s != %s", ErrInvalidSnapshotData, data.BrigadeID, brigadeID)
	}

	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(data.BrigadeID); err != nil {
		return fmt.Errorf("%w: decode brigade id: %s", ErrInvalidSnapshotData, err)
	}

	addr, err := netip.ParseAddr(endpointIPv4)
	if err != nil {
		return fmt.Errorf("%w: parse endpoint ip: %s", ErrInvalidSnapshotData, err)
	}

	if data.EndpointIPv4.String() != addr.String() {
		return fmt.Errorf("%w: %s != %s", ErrInvalidSnapshotData, data.EndpointIPv4.String(), addr.String())
	}

	// dnsIPv4 must be v4 IP
	if !data.DNSv4.Is4() {
		return fmt.Errorf("dns4 ip4: %w: %s", ErrInvalidSnapshotData, data.DNSv4.String())
	}

	// dnsIPv6 must be v6 IP
	if !data.DNSv6.Is6() {
		return fmt.Errorf("dns6 ip6: %w: %s", ErrInvalidSnapshotData, data.DNSv6.String())
	}

	// keydeskIPv6 must be v6 IP
	if !data.KeydeskIPv6.Is6() {
		return fmt.Errorf("keydesk ip6: %w: %s", ErrInvalidSnapshotData, data.KeydeskIPv6.String())
	}

	cgnatPrefix := netip.MustParsePrefix(keydesk.CGNATPrefix)
	if cgnatPrefix.Bits() < data.IPv4CGNAT.Bits() && !cgnatPrefix.Overlaps(data.IPv4CGNAT) {
		return fmt.Errorf("int4 ip4: %w: %s", ErrInvalidSnapshotData, data.IPv4CGNAT.String())
	}

	ulaPrefix := netip.MustParsePrefix(keydesk.ULAPrefix)
	if ulaPrefix.Bits() < data.IPv6ULA.Bits() && !ulaPrefix.Overlaps(data.IPv6ULA) {
		return fmt.Errorf("int6 ip6: %w: %s", ErrInvalidSnapshotData, data.IPv6ULA.String())
	}

	return nil
}

var ErrReservationMismatch = errors.New("reservation mismatch")

func checkReservation(ctx context.Context, tx pgx.Tx, caddr, addr netip.Addr, rid string) (uuid.UUID, error) {
	sqlCheckReservation := `
		SELECT
			p.pair_id
		FROM
			%s AS p -- pairs.pairs
			JOIN %s AS pei ON p.pair_id = pei.pair_id -- pairs.pairs_endpoints_ipv4
			JOIN %s AS re ON pei.endpoint_ipv4 = re.endpoint_ipv4 -- reservations.reserved_endpoints_ipv4
		WHERE
			p.control_ip=$1
		AND
			pei.endpoint_ipv4=$2
		AND
			re.reservation_id=$3
`

	var pairID uuid.UUID

	if err := tx.QueryRow(
		ctx,
		fmt.Sprintf(sqlCheckReservation,
			(pgx.Identifier{defaultPairsSchema, "pairs"}.Sanitize()),
			(pgx.Identifier{defaultPairsSchema, "pairs_endpoints_ipv4"}.Sanitize()),
			(pgx.Identifier{defaultBrigadesSchema, "reserved_endpoints_ipv4"}.Sanitize()),
		),
		caddr.String(), addr.String(), rid,
	).Scan(&pairID); err != nil {
		return pairID, fmt.Errorf("check reservation: %w", err)
	}

	if pairID == uuid.Nil {
		return pairID, ErrReservationMismatch
	}

	return pairID, nil
}

func checkBrigade(ctx context.Context, tx pgx.Tx,
	data *storage.Brigade, bid uuid.UUID,
) (bool, uuid.UUID, error) {
	sqlSelectBrigade := `
		SELECT
			brigade_id,
			instance_id,
			pair_id,
			endpoint_ipv4,
			dns_ipv4,
			dns_ipv6,
			keydesk_ipv6,
			ipv4_cgnat,
			ipv6_ula,
			main
		FROM
			%s
		WHERE
			brigade_id=$1
		AND
			endpoint_ipv4=$2
		`

	var (
		brigadeID, instanceID, pairID uuid.UUID
		endpointIPv4                  netip.Addr
		dnsIPv4, dnsIPv6              netip.Addr
		keydeskIPv6                   netip.Addr
		ipv4CGNAT, ipv6ULA            netip.Prefix
		main                          bool
	)

	if err := tx.QueryRow(ctx,
		fmt.Sprintf(sqlSelectBrigade, (pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize())),
		bid, data.EndpointIPv4,
	).Scan(&brigadeID, &instanceID, &pairID,
		&endpointIPv4,
		&dnsIPv4, &dnsIPv6,
		&keydeskIPv6,
		&ipv4CGNAT, &ipv6ULA,
		&main); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, instanceID, nil
		}

		return false, instanceID, fmt.Errorf("select brigade: %w", err)
	}

	if base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID[:]) == data.BrigadeID ||
		endpointIPv4.String() == data.EndpointIPv4.String() ||
		dnsIPv4.String() == data.DNSv4.String() ||
		dnsIPv6.String() == data.DNSv6.String() ||
		keydeskIPv6.String() == data.KeydeskIPv6.String() ||
		ipv4CGNAT.String() == data.IPv4CGNAT.String() ||
		ipv6ULA.String() == data.IPv6ULA.String() {
		return true, instanceID, nil
	}

	return false, instanceID, fmt.Errorf("%w: %s", ErrInvalidSnapshotData, brigadeID)
}

func getBrigadeName(ctx context.Context, tx pgx.Tx, bid uuid.UUID) (string, namesgenerator.Person, error) {
	sqlSelectBrigade := `
		SELECT
			brigadier,
			person
		FROM
			brigades.brigades
		WHERE
			brigade_id=$1
                        AND brigadier <> ''
                LIMIT 1
		`

	var (
		brigadier string
		person    namesgenerator.Person
	)

	if err := tx.QueryRow(ctx, sqlSelectBrigade, bid).Scan(&brigadier, &person); err != nil {
		return "", person, fmt.Errorf("select brigade: %w", err)
	}

	return brigadier, person, nil
}

func insertBrigade(ctx context.Context, tx pgx.Tx, data *storage.Brigade,
	brigadeID uuid.UUID, pairID uuid.UUID,
	name string, person namesgenerator.Person,
) (uuid.UUID, error) {
	sqlInsertBrigade := `
		INSERT INTO %s
				(
					brigade_id,
					pair_id,
					brigadier,
					endpoint_ipv4,
					dns_ipv4,
					dns_ipv6,
					keydesk_ipv6,
					ipv4_cgnat,
					ipv6_ula,
					person,
					main
				)
		VALUES
				(
					$1,
					$2,
					$3,
					$4,
					$5,
					$6,
					$7,
					$8,
					$9,
					$10,
					false
				)
		RETURNING instance_id;
		`

	var instanceID uuid.UUID

	if err := tx.QueryRow(ctx,
		fmt.Sprintf(sqlInsertBrigade, (pgx.Identifier{defaultBrigadesSchema, "brigades"}.Sanitize())),
		brigadeID,
		pairID,
		name,
		data.EndpointIPv4,
		data.DNSv4,
		data.DNSv6,
		data.KeydeskIPv6,
		data.IPv4CGNAT,
		data.IPv6ULA,
		person,
	).Scan(&instanceID); err != nil {
		return instanceID, fmt.Errorf("insert brigade: %w", err)
	}

	return instanceID, nil
}

func recreateBrigade(db *pgxpool.Pool, rid string, data *storage.Brigade, caddr, addr netip.Addr) error {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	pairID, err := checkReservation(ctx, tx, caddr, addr, rid)
	if err != nil {
		return fmt.Errorf("check reservation: %w", err)
	}

	brigadeID, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(data.BrigadeID)
	if err != nil {
		return fmt.Errorf("%w: decode brigade id: %s", ErrInvalidSnapshotData, err)
	}

	name, person, err := getBrigadeName(ctx, tx, uuid.UUID(brigadeID))
	if err != nil {
		return fmt.Errorf("get brigade name: %w", err)
	}

	exists, instanceID, err := checkBrigade(ctx, tx, data, uuid.UUID(brigadeID))
	if err != nil {
		return fmt.Errorf("check brigade: %w", err)
	}

	if !exists {
		instanceID, err = insertBrigade(ctx, tx, data, uuid.UUID(brigadeID), pairID, name, person)
		if err != nil {
			return fmt.Errorf("insert brigade: %w", err)
		}
	}

	sqlInsertStats := `
		INSERT INTO 
			%s 
			(brigade_id, instance_id) 
		VALUES 
			($1,$2) 
		ON CONFLICT (brigade_id, instance_id) DO NOTHING;
		`

	if _, err = tx.Exec(ctx,
		fmt.Sprintf(sqlInsertStats, (pgx.Identifier{defaultBrigadesStatsSchema, "brigades_stats"}.Sanitize())),
		uuid.UUID(brigadeID), instanceID,
	); err != nil {
		return fmt.Errorf("create stats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func checkPlan(plan *dcmgmt.RestorePlan, o *opts) error {
	if plan.RealmFP == "" {
		return fmt.Errorf("%w: empty realm key fingerprint", ErrInvalidSnapshotData)
	}

	snapsCount := 0
	for _, controls := range plan.Plan {
		if _, err := netip.ParseAddr(controls.ControlIP); err != nil {
			return fmt.Errorf("parse control ip: %w", err)
		}

		snapsCount += len(controls.Snaps)
	}

	if snapsCount == 0 {
		return fmt.Errorf("%w: empty snaps", ErrInvalidSnapshotData)
	}

	sshPub, err := ssh.NewPublicKey(o.privKey.Public())
	if err != nil {
		return fmt.Errorf("new ssh public key: %w", err)
	}

	if ssh.FingerprintSHA256(sshPub) != plan.RealmFP {
		return fmt.Errorf("realm: %w: %s != %s", ErrKeysMismatch,
			ssh.FingerprintSHA256(sshPub), plan.RealmFP)
	}

	return nil
}
