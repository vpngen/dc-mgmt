package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/netip"
	"os"
	"os/exec"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lmittmann/tint"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
)

const actionCreateBrigade = "create_brigade"

var (
	controlNetWindow  = netip.MustParsePrefix("10.0.0.0/8")
	endpointNetWindow = netip.MustParsePrefix("180.0.0.0/8")
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
	orderID, err := OrderCreateBrigade(ctx, logger, dbPool, SqFmt, cfg.BrigadeID, cfg.BrigadeName)
	if err != nil {
		logger.Error("error creating order", "error", err)

		log.Fatalf("Error creating order: %s", err)
	}

	// 2. create control/endpoint pair
	pairID, controlIP, endpointIPv4, err := CreatePair(ctx, logger, dbPool, SqFmt,
		cfg.PairsApp, orderID, cfg.MgmtRandomResponses)
	if err != nil {
		logger.Error("error creating pair", "error", err)

		log.Fatalf("Error creating pair: %s", err)
	}

	// 3. create brigade
	if err := CreateBrigade(ctx, logger, dbPool, SqFmt, orderID,
		cfg.DCIdent, pairID, controlIP, endpointIPv4,
		cfg.BrigadeID, cfg.BrigadeName,
		cfg.SubdomAPIHost, cfg.SubdomAPIToken,
		cfg.SSHKeyFile, cfg.DelegationSyncUser, cfg.DelegationSyncHost,
		cfg.NameServers, &vpnCfgs{
			wg:      cfg.WG,
			ovc:     cfg.OVC,
			ipsec:   cfg.IPsec,
			outline: cfg.Outline,
		},
		cfg.MgmtRandomResponses,
	); err != nil {
		logger.Error("error creating brigade", "error", err)

		log.Fatalf("Error creating brigade: %s", err)
	}

	logger.Info("brigade created", "brigade_id", cfg.BrigadeID, "brigade_name", cfg.BrigadeName)
}

func CreateBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
	dcident string, pairID uuid.UUID, controlIP netip.Addr, endpointIP netip.Addr,
	brigadeID uuid.UUID, brigadeName string,
	host, token string,
	sshkey, sshuser, server string,
	ns []string, vpnCfgs *vpnCfgs,
	doNotCreatePhy bool,
) error {
	err := setOrderFilling(ctx, logger, db, sqfmt, orderID)
	if err != nil {
		if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error setting order filling: %w", err)
	}

	sshconf, err := kdlib.CreateSSHConfig(sshkey, sshuser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating ssh configs: %w", err)
	}

	if err := createBrigade(ctx, db, logger, dcident,
		&brigadeOpts{
			id:   brigadeID.String(),
			name: brigadeName,
		},
		&pairOpts{
			pairID:       pairID,
			endpointIPv4: endpointIP,
			controlIP:    controlIP,
		},
		&delegationSync{
			sshconf: sshconf,
			server:  server,
		},
		&subdomAPI{
			host:  host,
			token: token,
		},
	); err != nil {
		if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating brigade: %w", err)
	}

	if !doNotCreatePhy {
		sshconf, err := kdlib.CreateSSHConfig(sshkey, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
		if err != nil {
			if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error creating ssh configs: %w", err)
		}

		if err := requestBrigade(ctx, db, logger, sshconf, brigadeID.String(), ns, vpnCfgs); err != nil {
			if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error requesting brigade: %w", err)
		}
	}

	if err := setOrderCompleted(ctx, logger, db, sqfmt, orderID); err != nil {
		return fmt.Errorf("error setting order completed: %w", err)
	}

	return nil
}

// OrderCreateBrigade creates an order to create a brigade.
func OrderCreateBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	brigadeID uuid.UUID, brigadeName string,
) (uuid.UUID, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	queryNum := sqfmt.Insert("pairs.pair_nums").
		Columns("update_time").
		Values(now).
		Suffix("RETURNING num_id")

	sql, args, err := queryNum.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	var numID int

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting number: %w", err)
	}

	query := sqfmt.Insert("pairs.pair_orders").
		Columns("num_id", "brigade_id", "brigade_name", "action", "created_at", "is_processing", "is_registering", "is_filling", "is_completed", "is_error", "message").
		Values(numID, brigadeID, brigadeName, actionCreateBrigade, now, false, false, false, false, false, "").
		Suffix("RETURNING order_id")

	sql, args, err = query.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	var orderID uuid.UUID

	if err := tx.QueryRow(ctx, sql, args...).Scan(&orderID); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("error committing transaction: %w", err)
	}

	return orderID, nil
}

func CreatePair(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	app string, orderID uuid.UUID, doNotCreatePhy bool,
) (uuid.UUID, netip.Addr, netip.Addr, error) {
	num, err := setOrderProcessing(ctx, logger, db, sqfmt, orderID)
	if err != nil {
		return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order processing: %w", err)
	}

	var (
		controlIP  netip.Addr
		endpointIP netip.Addr
	)

	switch doNotCreatePhy {
	case true:
		controlIP = kdlib.RandomAddrIPv4(controlNetWindow)
		endpointIP = kdlib.RandomAddrIPv4(endpointNetWindow)
	default:
		controlIP, endpointIP, err = processPair(ctx, logger, app, num)
		if err != nil {
			if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order error: %w", err)
			}

			return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error processing pair: %w", err)
		}
	}

	if err := setOrderRegistering(ctx, logger, db, sqfmt, orderID); err != nil {
		if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order error: %w", err)
		}

		return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order registering: %w", err)
	}

	pairID, err := registerPair(ctx, logger, db, sqfmt, controlIP, endpointIP, num)
	if err != nil {
		if err := setOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order error: %w", err)
		}

		return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error registering pair: %w", err)
	}

	return pairID, controlIP, endpointIP, nil
}

func registerPair(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	controlIP netip.Addr, endpointIP netip.Addr, num int,
) (uuid.UUID, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	id := uuid.New()

	query := sqfmt.Insert("pairs.pairs").
		Columns("pair_id", "control_ip", "is_active").
		Values(id, controlIP, true)

	sql, args, err := query.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting pair: %w", err)
	}

	queryNum := sqfmt.Insert("pairs.pair_num_links").
		Columns("pair_id", "num").
		Values(id, num)

	sql, args, err = queryNum.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting number: %w", err)
	}

	queryEndpointIP := sqfmt.Insert("pairs.pairs_endpoints_ipv4").
		Columns("pair_id", "endpoint_ip").
		Values(id, endpointIP)

	sql, args, err = queryEndpointIP.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting endpoint IP: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("error committing transaction: %w", err)
	}

	return id, nil
}

type PairsResult struct {
	Status  string `json:"status"`
	Result  string `json:"result,omitempty"`
	Control struct {
		IP netip.Addr `json:"ip"`
	} `json:"control,omitempty"`
	Endpoint struct {
		IP netip.Addr `json:"ip"`
	} `json:"endpoint,omitempty"`
}

func processPair(_ context.Context, _ *slog.Logger, app string, number int) (netip.Addr, netip.Addr, error) {
	var (
		stderr bytes.Buffer
		result PairsResult
	)

	cmd := exec.Command(app, "create", fmt.Sprintf("%d", number))

	// Create buffers to capture standard output and standard error
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("error creating stdout pipe: %w", err)
	}

	dec := json.NewDecoder(stdout)

	// Run the command
	if err := cmd.Start(); err != nil {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("error starting command: %w", err)
	}

	if err := dec.Decode(&result); err != nil {
		return netip.Addr{}, netip.Addr{}, fmt.Errorf("error decoding JSON: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		exe := &exec.ExitError{}
		if errors.As(err, &exe) {
			if err := json.NewDecoder(&stderr).Decode(&result); err != nil {
				return netip.Addr{}, netip.Addr{}, fmt.Errorf("error decoding JSON: %w", err)
			}

			return netip.Addr{}, netip.Addr{}, fmt.Errorf("error running command: %s", result.Result)
		}

		return netip.Addr{}, netip.Addr{}, fmt.Errorf("error waiting for command: %w", err)
	}

	return result.Control.IP, result.Endpoint.IP, nil
}

func setOrderProcessing(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
) (int, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	query := sqfmt.Update("pairs.pair_orders").
		Set("is_processing", true).
		Set("processing_started_at", now).
		Where(sq.Eq{"order_id": orderID})

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return 0, fmt.Errorf("error updating order: %w", err)
	}

	queryNum := sqfmt.Select("num_id").
		From("pairs.pair_orders").
		Where(sq.Eq{"order_id": orderID})

	sql, args, err = queryNum.ToSql()
	if err != nil {
		return 0, fmt.Errorf("error building SQL: %w", err)
	}

	var numID int

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID); err != nil {
		return 0, fmt.Errorf("error getting number ID: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("error committing transaction: %w", err)
	}

	return numID, nil
}

func setOrderRegistering(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	query := sqfmt.Update("pairs.pair_orders").
		Set("is_processing", false).
		Set("is_registering", true).
		Set("registering_started_at", now).
		Where(sq.Eq{"order_id": orderID})

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error updating order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func setOrderFilling(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	query := sqfmt.Update("pairs.pair_orders").
		Set("is_processing", false).
		Set("is_registering", false).
		Set("is_filling", true).
		Set("filling_started_at", now).
		Where(sq.Eq{"order_id": orderID})

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error updating order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func setOrderCompleted(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	query := sqfmt.Update("pairs.pair_orders").
		Set("is_processing", false).
		Set("is_registering", false).
		Set("is_filling", false).
		Set("is_completed", true).
		Set("completed_at", now).
		Where(sq.Eq{"order_id": orderID})

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error updating order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func setOrderError(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID, message string,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	query := sqfmt.Update("pairs.pair_orders").
		Set("is_processing", false).
		Set("is_registering", false).
		Set("is_filling", false).
		Set("is_error", true).
		Set("error_at", now).
		Set("message", message).
		Where(sq.Eq{"order_id": orderID})

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error updating order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}
