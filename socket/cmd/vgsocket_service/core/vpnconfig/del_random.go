package vpnconfig

import (
	"math/rand/v2"

	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core"
)

const (
	weightDelSuccess  = 70
	weightDelNotFound = 20
	weightDel503      = 10
	weightDelAll      = 100
)

func DeleteRandomUser() error {
	w := rand.Int64N(weightDelAll)

	switch {
	case w < weightDelSuccess:
		return nil
	case w < weightDelSuccess+weightDelNotFound:
		return core.ErrUserNotFound
	case w < weightDelSuccess+weightDelNotFound+weightDel503:
		return core.ErrTemporarilyUnavailable
	}

	return nil
}
