package snap

import (
	snapCore "github.com/vpngen/keydesk-snap/core"
)

// IncomingSnaps - structure for aggregated snapshots.
type IncomingSnaps struct {
	Snaps []*snapCore.EncryptedBrigade `json:"snaps"`

	TotalCount  int `json:"total_count"`
	ErrorsCount int `json:"errors_count"`
}
