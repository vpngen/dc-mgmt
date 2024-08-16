package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/lmittmann/tint"
	"github.com/vpngen/dc-mgmt/internal/kdlib"

	dcmgmtlib "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"
)

func main() {
	// 1. create order
	// 2. create control/endpoint pair
	// 3. create brigade

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
		logger.Error("error creating DB pool", "error", err)

		log.Fatalf("Error creating DB pool: %s", err)
	}

	SqFmt := sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

	// 1. create order
	orderID, err := dcmgmtlib.VgsOrderCreateBrigade(ctx, logger, dbPool, SqFmt, cfg.BrigadeID, cfg.BrigadeName)
	if err != nil {
		logger.Error("error creating order", "error", err)

		log.Fatalf("Error creating order: %s", err)
	}

	// 2. create control/endpoint pair
	pairID, controlIP, endpointIPv4, err := dcmgmtlib.VgsCreatePair(ctx, logger, dbPool, SqFmt,
		cfg.PairsApp, orderID, cfg.MgmtRandomResponses)
	if err != nil {
		logger.Error("error creating pair", "error", err)

		log.Fatalf("Error creating pair: %s", err)
	}

	// 3. create brigade
	if err := dcmgmtlib.VgsCreateBrigade(ctx, logger, dbPool, SqFmt, orderID,
		cfg.DCIdent, pairID, controlIP, endpointIPv4,
		cfg.BrigadeID, cfg.BrigadeName,
		cfg.SubdomAPIHost, cfg.SubdomAPIToken,
		cfg.SSHKeyFile, cfg.DelegationSyncUser, cfg.DelegationSyncHost,
		cfg.NameServers, &dcmgmtlib.VpnCfgs{
			Wg:      cfg.WG,
			Ovc:     cfg.OVC,
			Ipsec:   cfg.IPsec,
			Outline: cfg.Outline,
		},
		cfg.MaxUsers,
		cfg.MgmtRandomResponses,
	); err != nil {
		logger.Error("error creating brigade", "error", err)

		log.Fatalf("Error creating brigade: %s", err)
	}

	logger.Info("brigade created", "brigade_id", cfg.BrigadeID, "brigade_name", cfg.BrigadeName)
}
