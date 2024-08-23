package vpnconfig

import (
	"github.com/google/uuid"
	"github.com/vpngen/keydesk/gen/shuffler"
)

type SocketNewUser struct {
	ID      uuid.UUID          `json:"id"`
	Name    string             `json:"name,omitempty"`
	Configs shuffler.VPNConfig `json:"configs"`
}
