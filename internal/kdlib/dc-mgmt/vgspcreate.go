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

const VgsActionCreateBrigade = "create_brigade"

var (
	vgsControlNetWindow  = netip.MustParsePrefix("10.0.0.0/8")
	vgsEndpointNetWindow = netip.MustParsePrefix("180.0.0.0/8")
)

// created_at +=> pair_completed_at +=> completed_at || failed_at

// VgsOrderCreateBrigade creates an order to create a brigade.
func VgsOrderCreateBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	brigadeID uuid.UUID, brigadeName string, zone string,
) (uuid.UUID, string, int64, error) {
	logger.Info("creating brigade order", "brigade_id", brigadeID, "brigade_name", brigadeName, "zone", zone)

	if orderID, err := tryCreateCommonBrigade(ctx, logger, db, sqfmt, brigadeID, brigadeName, zone); err == nil ||
		!errors.Is(err, ErrNoCommonPairs) {
		if err != nil {
			return uuid.Nil, "", 0, fmt.Errorf("error trying to create common brigade: %w", err)
		}

		return orderID, VgsOrderStatusAccepted, DefaultVgsBrigadeOrderRetryAfter, nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	queryNum := sqfmt.Insert("pairs.endpoint_nums").
		Columns("update_time", "zone").
		Values(now, zone).
		Suffix("RETURNING endpoint_num")

	sql, args, err := queryNum.ToSql()
	if err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error building SQL: %w", err)
	}

	var numID int

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID); err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error inserting number (1): %w", err)
	}

	query := sqfmt.Insert("pairs.pair_orders").
		Columns("endpoint_num", "brigade_id", "brigade_name", "zone", "action", "created_at", "message").
		Values(numID, brigadeID, brigadeName, zone, VgsActionCreateBrigade, now, "").
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

func VgsCreatePair(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	app string, orderID uuid.UUID, doNotCreatePhy bool,
) error {
	num, zone, _, _, _, _, _, _, err := vgsGetOrderMeta(ctx, logger, db, sqfmt, orderID, true)
	if err != nil {
		return fmt.Errorf("error setting order processing: %w", err)
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
		controlIP, endpointIP, err = vgsProcessPair(ctx, logger, app, num, zone)
		if err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error processing pair: %w", err)
		}
	}

	logger.Info("pair created", "order_id", orderID, "endpoint_num", num, "control_ip", controlIP, "endpoint_ipv4", endpointIP, "duration", time.Since(start))

	pairID, err := vgsRegisterPair(ctx, logger, db, sqfmt, controlIP, endpointIP, orderID, num, zone)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error registering pair: %w", err)
	}

	logger.Info("pair created", "order_id", orderID, "endpoint_id", pairID, "endpoint_num", num, "control_ip", controlIP, "endpoint_ipv4", endpointIP)

	return nil
}

func vgsRegisterPair(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	controlIP netip.Addr, endpointIP netip.Addr, orderID uuid.UUID, num int, zone string,
) (uuid.UUID, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	newid := uuid.New()

	query := sqfmt.Insert("pairs.pairs").
		Columns("pair_id", "control_ip", "zone", "on_demand", "is_active").
		Values(newid, controlIP, zone, true, true).
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

	queryNum := sqfmt.Update("pairs.pairs_endpoints_ipv4").
		Set("endpoint_num", num).
		Where(sq.Eq{"endpoint_ipv4": endpointIP})

	sql, args, err = queryNum.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting number: %w", err)
	}

	queryStatus := sqfmt.Update("pairs.pair_orders").
		Set("pair_completed_at", time.Now().UTC()).
		Where(sq.Eq{"order_id": orderID})

	sql, args, err = queryStatus.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error updating order: %w", err)
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

func vgsProcessPair(_ context.Context, logger *slog.Logger, app string, number int, zone string) (netip.Addr, netip.Addr, error) {
	var (
		stderr bytes.Buffer
		result VgsPairsResult
	)

	app = AssembleAppPath(app, zone)

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

func AssembleAppPath(app string, zone string) string {
	if zone == "" {
		return fmt.Sprintf(app, zone)
	}

	return fmt.Sprintf(app, "-"+zone)
}
