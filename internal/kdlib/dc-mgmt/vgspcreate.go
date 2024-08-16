package dcmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"

	sq "github.com/Masterminds/squirrel"
)

const vgsActionCreateBrigade = "create_brigade"

var (
	vgsControlNetWindow  = netip.MustParsePrefix("10.0.0.0/8")
	vgsEndpointNetWindow = netip.MustParsePrefix("180.0.0.0/8")
)

func VgsCreateBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
	dcident string, pairID uuid.UUID, controlIP netip.Addr, endpointIP netip.Addr,
	brigadeID uuid.UUID, brigadeName string,
	host, token string,
	sshkey, sshuser, server string,
	ns []string, vpnCfgs *VpnCfgs,
	maxusers int,
	doNotCreatePhy bool,
) error {
	logger.Info("creating brigade", "brigade_id", brigadeID, "brigade_name", brigadeName, "order_id", orderID, "control_ip", controlIP, "endpoint_ipv4", endpointIP)

	err := vgsSetOrderFilling(ctx, logger, db, sqfmt, orderID)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error setting order filling: %w", err)
	}

	sshconf, err := kdlib.CreateSSHConfig(sshkey, sshuser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating ssh configs: %w", err)
	}

	if err := vgsCreateBrigade(ctx, db, logger, dcident,
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
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating brigade: %w", err)
	}

	if !doNotCreatePhy {
		sshconf, err := kdlib.CreateSSHConfig(sshkey, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
		if err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error creating ssh configs: %w", err)
		}

		if err := vgsRequestBrigade(ctx, db, logger, sshconf, brigadeID.String(), ns, vpnCfgs, maxusers); err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error requesting brigade: %w", err)
		}
	}

	if err := vgsSetOrderCompleted(ctx, logger, db, sqfmt, orderID); err != nil {
		return fmt.Errorf("error setting order completed: %w", err)
	}

	logger.Info("brigade created", "brigade_id", brigadeID, "brigade_name", brigadeName, "order_id", orderID)

	return nil
}

// VgsOrderCreateBrigade creates an order to create a brigade.
func VgsOrderCreateBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	brigadeID uuid.UUID, brigadeName string,
) (uuid.UUID, error) {
	logger.Info("creating brigade order", "brigade_id", brigadeID, "brigade_name", brigadeName)

	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	queryNum := sqfmt.Insert("pairs.endpoint_nums").
		Columns("update_time").
		Values(now).
		Suffix("RETURNING endpoint_num")

	sql, args, err := queryNum.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	var numID int

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting number: %w", err)
	}

	query := sqfmt.Insert("pairs.pair_orders").
		Columns("endpoint_num", "brigade_id", "brigade_name", "action", "created_at", "is_processing", "is_registering", "is_filling", "is_completed", "is_error", "message").
		Values(numID, brigadeID, brigadeName, vgsActionCreateBrigade, now, false, false, false, false, false, "").
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

	logger.Info("brigade order created", "brigade_id", brigadeID, "brigade_name", brigadeName, "order_id", orderID, "endpoint_num", numID)

	return orderID, nil
}

func VgsCreatePair(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	app string, orderID uuid.UUID, doNotCreatePhy bool,
) (uuid.UUID, netip.Addr, netip.Addr, error) {
	num, err := vgsSetOrderProcessing(ctx, logger, db, sqfmt, orderID)
	if err != nil {
		return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order processing: %w", err)
	}

	logger.Info("creating pair", "order_id", orderID, "endpoint_num", num)

	var (
		controlIP  netip.Addr
		endpointIP netip.Addr
	)

	start := time.Now().UTC()

	switch doNotCreatePhy {
	case true:
		controlIP = kdlib.RandomAddrIPv4(vgsControlNetWindow)
		endpointIP = kdlib.RandomAddrIPv4(vgsEndpointNetWindow)
	default:
		controlIP, endpointIP, err = vgsProcessPair(ctx, logger, app, num)
		if err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order error: %w", err)
			}

			return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error processing pair: %w", err)
		}
	}

	logger.Info("pair physically created", "order_id", orderID, "endpoint_num", num, "control_ip", controlIP, "endpoint_ipv4", endpointIP, "duration", time.Since(start))

	if err := vgsSetOrderRegistering(ctx, logger, db, sqfmt, orderID); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order error: %w", err)
		}

		return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order registering: %w", err)
	}

	pairID, err := vgsRegisterPair(ctx, logger, db, sqfmt, controlIP, endpointIP, num)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error setting order error: %w", err)
		}

		return uuid.Nil, netip.Addr{}, netip.Addr{}, fmt.Errorf("error registering pair: %w", err)
	}

	logger.Info("pair created", "order_id", orderID, "endpoint_id", pairID, "endpoint_num", num, "control_ip", controlIP, "endpoint_ipv4", endpointIP)

	return pairID, controlIP, endpointIP, nil
}

func vgsRegisterPair(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	controlIP netip.Addr, endpointIP netip.Addr, num int,
) (uuid.UUID, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	newid := uuid.New()

	query := sqfmt.Insert("pairs.pairs").
		Columns("pair_id", "control_ip", "is_active").
		Values(newid, controlIP, true).
		Suffix("ON CONFLICT (control_ip) DO UPDATE SET is_active=TRUE RETURNING pair_id")

	sql, args, err := query.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	var id uuid.UUID

	if err := tx.QueryRow(ctx, sql, args...).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting pair: %w", err)
	}

	queryEndpointIP := sqfmt.Insert("pairs.pairs_endpoints_ipv4").
		Columns("pair_id", "endpoint_ipv4").
		Values(id, endpointIP)

	sql, args, err = queryEndpointIP.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting endpoint IP: %w", err)
	}

	queryNum := sqfmt.Insert("pairs.endpoint_num_links").
		Columns("endpoint_ipv4", "endpoint_num").
		Values(endpointIP, num)

	sql, args, err = queryNum.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting number: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("error committing transaction: %w", err)
	}

	return id, nil
}

type VgsPairsResult struct {
	Status  string `json:"status"`
	Result  string `json:"result,omitempty"`
	Control struct {
		IP netip.Addr `json:"ip"`
	} `json:"control,omitempty"`
	Endpoint struct {
		IP netip.Addr `json:"ip"`
	} `json:"endpoint,omitempty"`
}

func vgsProcessPair(_ context.Context, logger *slog.Logger, app string, number int) (netip.Addr, netip.Addr, error) {
	var (
		stderr bytes.Buffer
		result VgsPairsResult
	)

	cmd := exec.Command(app, "create", fmt.Sprintf("%d", number))

	logger.Debug("process pair", "command", cmd.String())

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

func vgsSetOrderProcessing(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
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

	queryNum := sqfmt.Select("endpoint_num").
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

func vgsSetOrderRegistering(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
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

func vgsSetOrderFilling(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
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

func vgsSetOrderCompleted(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
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

func vgsSetOrderError(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
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
