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

const vgsActionDeleteBrigade = "delete_brigade"

func VgsDeleteBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
	dcident string, pairID uuid.UUID, controlIP netip.Addr,
	brigadeID uuid.UUID,
	host, token string,
	sshkey, sshuser, server string,
	doNotCreatePhy bool,
) error {
	logger.Info("deleting brigade", "brigade_id", brigadeID, "order_id", orderID, "control_ip", controlIP)

	err := vgsSetOrderFilling(ctx, logger, db, sqfmt, orderID)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error setting order filling: %w", err)
	}

	if !doNotCreatePhy {
		sshconf, err := kdlib.CreateSSHConfig(sshkey, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
		if err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error creating ssh configs: %w", err)
		}

		if err := vgsRevokeBrigade(ctx, logger, sshconf, brigadeID.String(), controlIP); err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error revoking brigade: %w", err)
		}
	}

	sshconf, err := kdlib.CreateSSHConfig(sshkey, sshuser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating ssh configs: %w", err)
	}

	if err := vgsRemoveBrigade(ctx, db, logger, dcident,
		brigadeID.String(), brigadeID,
		server, sshconf, host, token,
	); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error deleting brigade: %w", err)
	}

	logger.Info("brigade deleted", "brigade_id", brigadeID, "order_id", orderID)

	return nil
}

// VgsOrderDeleteBrigade creates an order to create a brigade.
func VgsOrderDeleteBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	brigadeID uuid.UUID,
) (uuid.UUID, uuid.UUID, netip.Addr, error) {
	logger.Info("deleting brigade order", "brigade_id", brigadeID)

	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, netip.Addr{}, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	queryNumName := sqfmt.Select("l.endpoint_num", "b.brigadier", "p.pair_id", "p.control_ip").
		From("brigades.brigades b").
		Join("pairs.pairs p ON b.pair_id = p.pair_id").
		Join("pairs.endpoint_num_links l ON b.endpoint_ipv4 = l.endpoint_ipv4").
		Where(sq.Eq{"b.brigade_id": brigadeID})

	sql, args, err := queryNumName.ToSql()
	if err != nil {
		return uuid.Nil, uuid.Nil, netip.Addr{}, fmt.Errorf("error building SQL: %w", err)
	}

	var (
		numID       int
		brigadeName string
		pairID      uuid.UUID
		controlIP   netip.Addr
	)

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID, &brigadeName, &pairID, &controlIP); err != nil {
		return uuid.Nil, uuid.Nil, netip.Addr{}, fmt.Errorf("error reading number and name: %w", err)
	}

	query := sqfmt.Insert("pairs.pair_orders").
		Columns("endpoint_num", "brigade_id", "brigade_name", "action", "created_at", "is_processing", "is_registering", "is_filling", "is_completed", "is_error", "message").
		Values(numID, brigadeID, brigadeName, vgsActionDeleteBrigade, now, false, false, false, false, false, "").
		Suffix("RETURNING order_id")

	sql, args, err = query.ToSql()
	if err != nil {
		return uuid.Nil, uuid.Nil, netip.Addr{}, fmt.Errorf("error building SQL: %w", err)
	}

	var orderID uuid.UUID

	if err := tx.QueryRow(ctx, sql, args...).Scan(&orderID); err != nil {
		return uuid.Nil, uuid.Nil, netip.Addr{}, fmt.Errorf("error inserting order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, uuid.Nil, netip.Addr{}, fmt.Errorf("error committing transaction: %w", err)
	}

	logger.Info("brigade order created", "brigade_id", brigadeID, "brigade_name", brigadeName, "order_id", orderID, "endpoint_num", numID)

	return orderID, pairID, controlIP, nil
}

func VgsDeletePair(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	app string, orderID uuid.UUID, pairID uuid.UUID, doNotCreatePhy bool,
) error {
	num, err := vgsSetOrderProcessing(ctx, logger, db, sqfmt, orderID)
	if err != nil {
		return fmt.Errorf("error setting order processing: %w", err)
	}

	logger.Info("deleting pair", "order_id", orderID, "endpoint_num", num)

	start := time.Now().UTC()

	switch doNotCreatePhy {
	case true:
	default:
		if err := vgsProcessPairDeleting(ctx, logger, app, num); err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error processing pair: %w", err)
		}
	}

	logger.Info("pair physically deleted", "order_id", orderID, "endpoint_num", num, "duration", time.Since(start))

	if err := vgsSetOrderRegistering(ctx, logger, db, sqfmt, orderID); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error setting order registering: %w", err)
	}

	if err := vgsUnregisterPair(ctx, logger, db, sqfmt, pairID, num); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error registering pair: %w", err)
	}

	if err := vgsSetOrderCompleted(ctx, logger, db, sqfmt, orderID); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error setting order completed: %w", err)
	}

	logger.Info("pair created", "order_id", orderID, "pair_id", pairID, "endpoint_num", num)

	return nil
}

func vgsUnregisterPair(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	pairID uuid.UUID, num int,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	queryNum := sqfmt.Delete("pairs.endpoint_num_links").
		Where(sq.Eq{"endpoint_num": num})

	sql, args, err := queryNum.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error unlinking number: %w", err)
	}

	queryEndpointIP := sqfmt.Delete("pairs.pairs_endpoints_ipv4").
		Where(sq.Eq{"pair_id": pairID})

	sql, args, err = queryEndpointIP.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error deleting endpoint IP: %w", err)
	}

	query := sqfmt.Delete("pairs.pairs").
		Where(sq.Eq{"pair_id": pairID})

	sql, args, err = query.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error deleting pair: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func vgsProcessPairDeleting(_ context.Context, logger *slog.Logger, app string, number int) error {
	var (
		stderr bytes.Buffer
		result VgsPairsResult
	)

	cmd := exec.Command(app, "delete", fmt.Sprintf("%d", number))

	logger.Debug("process pair deleting", "command", cmd.String())

	// Create buffers to capture standard output and standard error
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}

	dec := json.NewDecoder(stdout)

	// Run the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	if err := dec.Decode(&result); err != nil {
		return fmt.Errorf("error decoding JSON: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		exe := &exec.ExitError{}
		if errors.As(err, &exe) {
			if err := json.NewDecoder(&stderr).Decode(&result); err != nil {
				return fmt.Errorf("error decoding JSON: %w", err)
			}

			return fmt.Errorf("error running command: %s", result.Result)
		}

		return fmt.Errorf("error waiting for command: %w", err)
	}

	return nil
}
