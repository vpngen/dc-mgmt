package vpnconfig

import (
	"github.com/google/uuid"
	"github.com/vpngen/keydesk/gen/shuffler"
)

type SocketNewUser struct {
	ID        uuid.UUID          `json:"id"`
	Name      string             `json:"name"`
	Domain    string             `json:"domain"`
	Configs   shuffler.VPNConfig `json:"configs"`
	FreeSlots int                `json:"free_slots"`
}

type SocketDeleteUser struct {
	FreeSlots int `json:"free_slots"`
}
