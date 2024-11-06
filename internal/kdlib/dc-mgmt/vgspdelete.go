package dcmgmt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sq "github.com/Masterminds/squirrel"
)

// created_at +=> brigade_completed_at +=> completed_at || failed_at

const VgsActionDeleteBrigade = "delete_brigade"

// VgsOrderDeleteBrigade creates an order to create a brigade.
func VgsOrderDeleteBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	brigadeID uuid.UUID,
) (uuid.UUID, string, int64, error) {
	logger.Info("deleting brigade order", "brigade_id", brigadeID)

	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	queryNumName := sqfmt.Select("l.endpoint_num", "b.brigadier", "p.pair_id", "p.control_ip", "p.zone").
		From("brigades.brigades b").
		Join("pairs.pairs p ON b.pair_id = p.pair_id").
		Join("pairs.endpoint_num_links l ON b.endpoint_ipv4 = l.endpoint_ipv4").
		Where(sq.Eq{"b.brigade_id": brigadeID})

	sql, args, err := queryNumName.ToSql()
	if err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error building SQL: %w", err)
	}

	var (
		numID       int
		brigadeName string
		pairID      uuid.UUID
		controlIP   netip.Addr
		zone        string
	)

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID, &brigadeName, &pairID, &controlIP, &zone); err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error reading number and name: %w", err)
	}

	query := sqfmt.Insert("pairs.pair_orders").
		Columns("endpoint_num", "brigade_id", "brigade_name", "zone", "action", "created_at", "message").
		Values(numID, brigadeID, brigadeName, zone, VgsActionDeleteBrigade, now, "").
		Suffix("RETURNING order_id")

	sql, args, err = query.ToSql()
	if err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error building SQL: %w", err)
	}

	var orderID uuid.UUID

	if err := tx.QueryRow(ctx, sql, args...).Scan(&orderID); err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error inserting order: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error committing transaction: %w", err)
	}

	logger.Info("brigade order created", "brigade_id", brigadeID, "brigade_name", brigadeName, "order_id", orderID, "endpoint_num", numID)

	return orderID, VgsOrderStatusAccepted, DefaultVgsOrderRetryAfter, nil
}

func VgsDeletePair(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	app string, orderID uuid.UUID, doNotCreatePhy bool,
) error {
	num, zone, pairID, _, endpointIPv4, _, _, err := vgsGetOrderMeta(ctx, logger, db, sqfmt, orderID, false)
	if err != nil {
		return fmt.Errorf("error setting order processing: %w", err)
	}

	logger.Info("deleting pair", "order_id", orderID, "endpoint_num", num)

	start := time.Now().UTC()

	switch doNotCreatePhy {
	case true:
	default:
		if err := vgsProcessPairDeleting(ctx, logger, app, num, zone); err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error processing pair: %w", err)
		}
	}

	logger.Info("pair physically deleted", "order_id", orderID, "endpoint_num", num, "duration", time.Since(start))

	if err := vgsUnregisterPair(ctx, logger, db, sqfmt, pairID, num, endpointIPv4); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error registering pair: %w", err)
	}

	if err := vgsSetOrderComplete(ctx, logger, db, sqfmt, orderID); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error setting order completed: %w", err)
	}

	logger.Info("pair deleted", "order_id", orderID, "pair_id", pairID, "endpoint_num", num)

	return nil
}

func vgsUnregisterPair(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	pairID uuid.UUID, num int, endpointIPv4 netip.Addr,
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
		Where(sq.And{
			sq.Eq{"pair_id": pairID},
			sq.Eq{"endpoint_ipv4": endpointIPv4},
		})

	sql, args, err = queryEndpointIP.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("error deleting endpoint IP: %w", err)
	}

	queryPairs := sqfmt.Select("COUNT(*)").
		From("pairs.pairs_endpoints_ipv4").
		Where(sq.Eq{"pair_id": pairID})

	sql, args, err = queryPairs.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	var count int

	if err := tx.QueryRow(ctx, sql, args...).Scan(&count); err != nil {
		return fmt.Errorf("error counting endpoint IPs: %w", err)
	}

	if count == 0 {
		query := sqfmt.Delete("pairs.pairs").
			Where(sq.Eq{"pair_id": pairID})

		sql, args, err := query.ToSql()
		if err != nil {
			return fmt.Errorf("error building SQL: %w", err)
		}

		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return fmt.Errorf("error deleting pair: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}

func vgsProcessPairDeleting(_ context.Context, logger *slog.Logger, app string, number int, zone string) error {
	var (
		stderr bytes.Buffer
		result VgsPairsResult
	)

	app = AssembleAppPath(app, zone)

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
		if err == io.EOF {
			return nil
		}

		return fmt.Errorf("error decoding JSON: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		exe := &exec.ExitError{}
		if errors.As(err, &exe) {
			if err := json.NewDecoder(&stderr).Decode(&result); err != nil && err != io.EOF {
				return fmt.Errorf("error decoding JSON: %w", err)
			}

			return fmt.Errorf("error running command: %s", result.Result)
		}

		return fmt.Errorf("error waiting for command: %w", err)
	}

	return nil
}
