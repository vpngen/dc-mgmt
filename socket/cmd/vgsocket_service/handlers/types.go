package handlers

import "github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/core"

type Options struct {
	core.Options

	Testing bool
}
