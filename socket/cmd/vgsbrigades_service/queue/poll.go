package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	sq "github.com/Masterminds/squirrel"
	dcmgmt "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const period = 10 * time.Second

type PollConfig struct {
	Db    *pgxpool.Pool
	SqFmt sq.StatementBuilderType

	PairsApp string // Script which creates pairs

	DCIdent string // Datacenter identifier

	// Subdomain API.
	SubdomAPIHost  string // Subdomain API host
	SubdomAPIToken string // Subdomain API token

	// Delegation sync.
	DelegationSyncUser string // Delegation sync user
	DelegationSyncHost string // Delegation sync host

	// Name servers.
	NameServers []string // Domain nameservers

	// Config types.
	WG      string // Wireguard configs
	OVC     string // OVC configs
	IPsec   string // IPsec configs
	Outline string // Outline configs

	SSHKeyFile string // SSH key file
	MaxUsers   int

	MgmtRandomResponses bool
}

func Poll(ctx context.Context, logger *slog.Logger, stop <-chan struct{}, opts *PollConfig) {
	logger.Info("starting queue poller", "period", period)

	timer := time.NewTimer(period)

	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("cancel queue poller")

			return
		case <-stop:
			logger.Info("stop queue poller")

			return
		case t := <-timer.C:
			logger.Info("polling queue", "time", t)

			if err := handleQueue(ctx, logger, opts); err != nil {
				logger.Error("error handling queue", "error", err)
			}

			timer.Reset(period)
		}
	}
}

var ErrNOP = errors.New("nop")

func handleQueue(ctx context.Context, logger *slog.Logger, opts *PollConfig) error {
	err := checkNewCreateBrigadeOrder(ctx, logger, opts)
	if err != nil && !errors.Is(err, ErrNOP) {
		return fmt.Errorf("new create brigade order: %w", err)
	}

	if err == nil {
		return nil
	}

	err = checkPairCompletedCreateBrigadeOrder(ctx, logger, opts)
	if err != nil && !errors.Is(err, ErrNOP) {
		return fmt.Errorf("pair completed create brigade order: %w", err)
	}

	if err == nil {
		return nil
	}

	err = checkNewDeleteBrigadeOrder(ctx, logger, opts)
	if err != nil && !errors.Is(err, ErrNOP) {
		return fmt.Errorf("new delete brigade order: %w", err)
	}

	if err == nil {
		return nil
	}

	err = checkBrigadeCompletedDeleteBrigadeOrder(ctx, logger, opts)
	if err != nil && !errors.Is(err, ErrNOP) {
		return fmt.Errorf("brigade completed delete brigade order: %w", err)
	}

	return nil
}

func checkNewCreateBrigadeOrder(ctx context.Context, logger *slog.Logger, opts *PollConfig) error {
	orderID, err := dcmgmt.VgsPopOrder(ctx, logger, opts.Db, opts.SqFmt, dcmgmt.VgsActionCreateBrigade, dcmgmt.VgsOrderPopStatusAccepted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNOP
		}

		return fmt.Errorf("popping order: %w", err)
	}

	if err := dcmgmt.VgsCreatePair(ctx, logger, opts.Db, opts.SqFmt, opts.PairsApp, orderID, opts.MgmtRandomResponses); err != nil {
		return fmt.Errorf("creating pair: %w", err)
	}

	if err := dcmgmt.VgsCreateBrigade(ctx, logger, opts.Db, opts.SqFmt, orderID,
		opts.DCIdent, opts.SubdomAPIHost, opts.SubdomAPIToken, opts.SSHKeyFile,
		opts.DelegationSyncUser, opts.DelegationSyncHost,
		opts.NameServers, &dcmgmt.VpnCfgs{
			Wg:      opts.WG,
			Ovc:     opts.OVC,
			Ipsec:   opts.IPsec,
			Outline: opts.Outline,
		}, opts.MaxUsers, opts.MgmtRandomResponses); err != nil {
		return fmt.Errorf("creating brigade: %w", err)
	}

	return nil
}

func checkPairCompletedCreateBrigadeOrder(ctx context.Context, logger *slog.Logger, opts *PollConfig) error {
	orderID, err := dcmgmt.VgsPopOrder(ctx, logger, opts.Db, opts.SqFmt, dcmgmt.VgsActionCreateBrigade, dcmgmt.VgsOrderPopStatusPairCompleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNOP
		}

		return fmt.Errorf("popping order: %w", err)
	}

	if err := dcmgmt.VgsCreateBrigade(ctx, logger, opts.Db, opts.SqFmt, orderID,
		opts.DCIdent, opts.SubdomAPIHost, opts.SubdomAPIToken, opts.SSHKeyFile,
		opts.DelegationSyncUser, opts.DelegationSyncHost,
		opts.NameServers, &dcmgmt.VpnCfgs{
			Wg:      opts.WG,
			Ovc:     opts.OVC,
			Ipsec:   opts.IPsec,
			Outline: opts.Outline,
		}, opts.MaxUsers, opts.MgmtRandomResponses); err != nil {
		return fmt.Errorf("creating brigade: %w", err)
	}

	return nil
}

func checkNewDeleteBrigadeOrder(ctx context.Context, logger *slog.Logger, opts *PollConfig) error {
	orderID, err := dcmgmt.VgsPopOrder(ctx, logger, opts.Db, opts.SqFmt, dcmgmt.VgsActionDeleteBrigade, dcmgmt.VgsOrderPopStatusAccepted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNOP
		}

		return fmt.Errorf("popping order: %w", err)
	}

	if err := dcmgmt.VgsDeleteBrigade(ctx, logger, opts.Db, opts.SqFmt, orderID,
		opts.DCIdent, opts.SubdomAPIHost, opts.SubdomAPIToken, opts.SSHKeyFile,
		opts.DelegationSyncUser, opts.DelegationSyncHost,
		opts.MgmtRandomResponses); err != nil {
		return fmt.Errorf("deleting brigade: %w", err)
	}

	if err := dcmgmt.VgsDeletePair(ctx, logger, opts.Db, opts.SqFmt,
		opts.PairsApp, orderID, opts.MgmtRandomResponses); err != nil {
		return fmt.Errorf("deleting pair: %w", err)
	}

	return nil
}

func checkBrigadeCompletedDeleteBrigadeOrder(ctx context.Context, logger *slog.Logger, opts *PollConfig) error {
	orderID, err := dcmgmt.VgsPopOrder(ctx, logger, opts.Db, opts.SqFmt, dcmgmt.VgsActionDeleteBrigade, dcmgmt.VgsOrderPopStatusBrigadeCompleted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNOP
		}

		return fmt.Errorf("popping order: %w", err)
	}

	if err := dcmgmt.VgsDeletePair(ctx, logger, opts.Db, opts.SqFmt,
		opts.PairsApp, orderID, opts.MgmtRandomResponses); err != nil {
		return fmt.Errorf("deleting pair: %w", err)
	}

	return nil
}
