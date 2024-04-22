package realmadmin

import (
	"net/netip"
	"time"

	snapCore "github.com/vpngen/keydesk-snap/core"
	"github.com/vpngen/keydesk/keydesk"
	"github.com/vpngen/keydesk/keydesk/storage"
)

// InstancedSnaps - structure for encrypted brigade with additional fields.
type InstancedSnaps struct {
	Snaps []*EncryptedBrigade `json:"snaps"`

	TotalCount  int `json:"total_count"`
	ErrorsCount int `json:"errors_count"`
}

// EncryptedBrigade - structure for encrypted brigade with additional fields.
type EncryptedBrigade struct {
	snapCore.EncryptedBrigade
	InstanceID string `json:"instance_id"`
}

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

	// ExternalIPFiltered is a filtered prefix if applicable.
	ExternalIPFiltered netip.Prefix `json:"ext_ip_filtered,omitempty"`

	// ControlNodeFiltered is a filtered prefix if applicable.
	ControlNodeFiltered netip.Prefix `json:"ctrl_node_filtered,omitempty"`

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

	Snaps []*EncryptedBrigade `json:"snaps"`

	// TotalCount is a total count of the snapshots.
	TotalCount int `json:"total_count"`

	// ErrorsCount is a count of the errors during the snapshot collection.
	ErrorsCount int `json:"errors_count"`
}

// ReservationNodeConfig - reservation node config.
type ReservationNodeConfig struct {
	ControlIP        string   `json:"control_ip"`
	RouterNACLPubKey string   `json:"router_nacl_pubkey"`
	Slots            []string `json:"slots"`
}

// ReservationConfig - reservation config.
type ReservationConfig struct {
	ReservationID string                  `json:"reservation_id"`
	Plan          []ReservationNodeConfig `json:"plan"`
}

// PreparedSnap - prepared snapshot.
type PreparedSnap struct {
	BrigadeID       string   `json:"brigade_id"`
	EndpointIPv4    string   `json:"endpoint_ipv4"`
	DomainNames     []string `json:"domain_names"`
	Payload         string   `json:"payload"`          // encrypted by realm public key
	EncryptedSecret string   `json:"encrypted_secret"` // encrypted by target realm public key
}

// RestoreNodeConfig - restore node config.
type RestoreNodeConfig struct {
	ControlIP string         `json:"control_ip"`
	Snaps     []PreparedSnap `json:"snaps"`
}

// RestorePlan - restore plan.
type RestorePlan struct {
	RealmFP       string              `json:"realm_fp"`
	ReservationID string              `json:"reservation_id"`
	Plan          []RestoreNodeConfig `json:"plan"`
}

// ControlNodeRestorePlan - control node restore plan.
type ControlNodeRestorePlan struct {
	Plan []*storage.Brigade `json:"plan"`
}
