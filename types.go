package realmadmin

import (
	"net/netip"
	"time"

	snapCore "github.com/vpngen/keydesk-snap/core"
	"github.com/vpngen/keydesk/keydesk"
)

type Answer struct {
	keydesk.Answer
	KeydeskIPv6 netip.Addr `json:"keydesk_ipv6"`
	FreeSlots   int        `json:"free_slots"`
}

// AggrStatsVersion - current version of aggregated stats.
const AggrSnapsVersion = 2

// AggrSnaps - structure for aggregated stats with additional fields.
type AggrSnaps struct {
	Version    int       `json:"version"`
	UpdateTime time.Time `json:"update_time"`

	// Filtered is a filtered prefix if applicable.
	Filtered netip.Prefix `json:"filtered,omitempty"`

	// DatacenterID is a datacenter id.
	DatacenterID string `json:"datacenter_id"`

	// identification tag, using to ident whole snapshot.
	// 2023-01-01T00:00:00Z-regular-quarter-snapshot.
	Tag string `json:"tag"`

	// GlobalSnapAt is a time of the global snapshot start.
	// It is used to identify the snapshot.
	GlobalSnapAt time.Time `json:"global_snap_at"`

	// RealmKeyFP is a fingerprint of the realm public key.
	RealmKeyFP string `json:"realm_key_fp"`

	// AuthorityKeyFP is a fingerprint of the authority public key
	// with which the PSK was encrypted.
	AuthorityKeyFP string `json:"authority_key_fp"`
	// PSK is a secret, which is used to concatenate with
	// the main secret and LockerSecret to get the final secret.
	// We need to provide it to decrypt the payload.
	// PSK is encrypted with Realm public key or Authority
	// public key determined by situation.
	EncryptedPreSharedSecret string `json:"encrypted_psk"`

	Snaps []*snapCore.EncryptedBrigade `json:"snaps"`

	// TotalCount is a total count of the snapshots.
	TotalCount int `json:"total_count"`

	// ErrorsCount is a count of the errors during the snapshot collection.
	ErrorsCount int `json:"errors_count"`
}
