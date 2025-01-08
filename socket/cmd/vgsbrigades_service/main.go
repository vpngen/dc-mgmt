package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsbrigades_service/queue"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsbrigades_service/server"

	sq "github.com/Masterminds/squirrel"
)

func main() {
	cfg, err := NewConfig()
	if err != nil {
		log.Fatalf("Error reading config: %s", err)
	}

	logger := slog.New(
		tint.NewHandler(os.Stderr, &tint.Options{
			Level:      cfg.LogLevel,
			TimeFormat: time.RFC3339,
		}),
	)

	cfg.PrintInfo(logger)

	ctx := context.Background()

	dbPool, err := kdlib.CreateDBPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("error creating DB pool", "error", err)

		log.Fatalf("Error creating DB pool: %s", err)
	}

	api := server.NewAPI()

	// add other handlers HERE.

	opts := &server.APIOpts{
		Logger: logger,

		API: api,

		Db:    dbPool,
		SqFmt: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),

		Testing: cfg.MgmtRandomResponses,
	}

	server.SetSecurityHandlers(ctx, opts, cfg.JWTSigningMethod, cfg.JWTVerifyKey)

	server.SetBrigadeHandlers(ctx, opts)

	// construct API handler.
	handler := server.MakeVGSocketRealmAPIHandler(api, cfg.PermissiveCORS)

	pollOpts := &queue.PollConfig{
		Db:    dbPool,
		SqFmt: sq.StatementBuilder.PlaceholderFormat(sq.Dollar),

		PairsApp: cfg.PairsApp,

		DCIdent: cfg.DCIdent,

		SubdomAPIHost:  cfg.SubdomAPIHost,
		SubdomAPIToken: cfg.SubdomAPIToken,

		DelegationSyncUser: cfg.DelegationSyncUser,
		DelegationSyncHost: cfg.DelegationSyncHost,

		NameServers: cfg.NameServers,

		WG:      cfg.WG,
		OVC:     cfg.OVC,
		IPsec:   cfg.IPsec,
		Outline: cfg.Outline,
		Proto0:  cfg.Proto0,

		SSHKeyFile: cfg.SSHKeyFile,
		MaxUsers:   cfg.MaxUsers,

		MgmtRandomResponses: cfg.MgmtRandomResponses,
	}

	// serve API
	server.ListenAndServeHTTP(ctx, handler, logger, cfg.Listen, pollOpts)

	slog.Info("exiting")
}
