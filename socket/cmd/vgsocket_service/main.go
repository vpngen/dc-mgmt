package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"github.com/vpngen/dc-mgmt/socket/cmd/vgsocket_service/server"

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

		AccessKey: cfg.KdAccessKey,

		Testing:   cfg.RandomResponses,
		KdTesting: cfg.KdRandomResponses,
	}

	server.SetSecurityHandlers(ctx, opts, cfg.JWTSigningMethod, cfg.JWTVerifyKey)

	server.SetUserHandlers(ctx, opts)

	// construct API handler.
	handler := server.MakeVGSocketRealmAPIHandler(api, cfg.PermissiveCORS)

	// serve API
	server.ListenAndServeHTTP(handler, logger, cfg.Listen)

	slog.Info("exiting")
}
