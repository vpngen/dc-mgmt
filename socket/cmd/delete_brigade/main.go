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
	// 2. delete brigade
	// 3. delete control/endpoint pair

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
	orderID, pairID, controlIP, err := dcmgmtlib.VgsOrderDeleteBrigade(ctx, logger, dbPool, SqFmt, cfg.BrigadeID)
	if err != nil {
		logger.Error("error creating order", "error", err)

		log.Fatalf("Error creating order: %s", err)
	}

	// 2. delete brigade
	if err := dcmgmtlib.VgsDeleteBrigade(ctx, logger, dbPool, SqFmt, orderID,
		cfg.DCIdent, pairID, controlIP,
		cfg.BrigadeID,
		cfg.SubdomAPIHost, cfg.SubdomAPIToken,
		cfg.SSHKeyFile, cfg.DelegationSyncUser, cfg.DelegationSyncHost,
		cfg.MgmtRandomResponses,
	); err != nil {
		logger.Error("error creating brigade", "error", err)

		log.Fatalf("Error creating brigade: %s", err)
	}

	// 3. delete control/endpoint pair

	if err := dcmgmtlib.VgsDeletePair(ctx, logger, dbPool, SqFmt,
		cfg.PairsApp, orderID, pairID, cfg.MgmtRandomResponses); err != nil {
		logger.Error("error deleting pair", "error", err)

		log.Fatalf("Error deleting pair: %s", err)
	}

	logger.Info("brigade deleted", "brigade_id", cfg.BrigadeID)
}
