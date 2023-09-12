package realmadmin

import (
	"net/netip"

	"github.com/vpngen/keydesk/keydesk"
)

type Answer struct {
	keydesk.Answer
	KeydeskIPv6 netip.Addr `json:"keydesk_ipv6"`
	FreeSlots   int        `json:"free_slots"`
}
