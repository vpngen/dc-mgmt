package dcmgmt

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	VgsOrderStatusAccepted   = "accepted"
	VgsOrderStatusProcessing = "processing"
	VgsOrderStatusFailed     = "failed"
	VgsOrderStatusCompleted  = "completed"
)

const (
	DefaultVgsOrderRetryAfter = 300 // seconds
)

func VgsCheckOrderStatus(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
) (uuid.UUID, string, int64, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	query := sqfmt.Select("brigade_id", "completed_at", "failed_at").
		From("pairs.pair_orders").
		Where(sq.Eq{"order_id": orderID})

	sql, args, err := query.ToSql()
	if err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error building SQL: %w", err)
	}

	var (
		completedAt pgtype.Timestamp
		failedAt    pgtype.Timestamp
		brigadeID   uuid.UUID
	)

	if err := tx.QueryRow(ctx, sql, args...).Scan(&brigadeID, &completedAt, &failedAt); err != nil {
		return uuid.Nil, "", 0, fmt.Errorf("error getting order status: %w", err)
	}

	if failedAt.Valid {
		return brigadeID, VgsOrderStatusFailed, 0, nil
	}

	if completedAt.Valid {
		return brigadeID, VgsOrderStatusCompleted, 0, nil
	}

	return brigadeID, VgsOrderStatusProcessing, 0, nil
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
		Set("failed_at", now).
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

func vgsSetOrderComplete(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	query := sqfmt.Update("pairs.pair_orders").
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

// vgsGetOrderMeta returns:
// - num (unique virtual machine number)
// - zone (availability zone)
// - pairID (unique pair ID)
// - controlIP (control IP address)
// - endpointIP (endpoint IP address)
// - brigadeID (unique brigade ID)
// - brigadeName (brigade name)
func vgsGetOrderMeta(ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID, short bool,
) (int, string, uuid.UUID, netip.Addr, netip.Addr, uuid.UUID, string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, "", uuid.Nil, netip.Addr{}, netip.Addr{}, uuid.Nil, "", fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	queryNum := sqfmt.Select("endpoint_num", "zone", "brigade_id", "brigade_name").
		From("pairs.pair_orders").
		Where(
			sq.And{
				sq.Eq{"order_id": orderID},
				sq.Eq{"failed_at": nil},
			},
		)

	sql, args, err := queryNum.ToSql()
	if err != nil {
		return 0, "", uuid.Nil, netip.Addr{}, netip.Addr{}, uuid.Nil, "", fmt.Errorf("error building SQL: %w", err)
	}

	var (
		numID       int
		zone        string
		brigadeID   uuid.UUID
		brigadeName string
	)

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID, &zone, &brigadeID, &brigadeName); err != nil {
		return 0, "", uuid.Nil, netip.Addr{}, netip.Addr{}, uuid.Nil, "", fmt.Errorf("error getting number ID: %w", err)
	}

	if short {
		return numID, zone, uuid.Nil, netip.Addr{}, netip.Addr{}, brigadeID, brigadeName, nil
	}

	queryPair := sqfmt.Select("p.pair_id", "pei.endpoint_ipv4", "p.control_ip").
		From("pairs.endpoint_num_links enl").
		Join("pairs.pairs_endpoints_ipv4 pei ON enl.endpoint_ipv4 = pei.endpoint_ipv4").
		Join("pairs.pairs p ON pei.pair_id = p.pair_id").
		Where(sq.Eq{"enl.endpoint_num": numID})

	sql, args, err = queryPair.ToSql()
	if err != nil {
		return 0, "", uuid.Nil, netip.Addr{}, netip.Addr{}, uuid.Nil, "", fmt.Errorf("error building SQL: %w", err)
	}

	var (
		pairID     uuid.UUID
		endpointIP netip.Addr
		controlIP  netip.Addr
	)

	if err := tx.QueryRow(ctx, sql, args...).Scan(&pairID, &endpointIP, &controlIP); err != nil {
		return 0, "", uuid.Nil, netip.Addr{}, netip.Addr{}, uuid.Nil, "", fmt.Errorf("error getting pair ID: %w", err)
	}

	return numID, zone, pairID, controlIP, endpointIP, brigadeID, brigadeName, nil
}

const (
	VgsOrderPopStatusAccepted         = "accepted"
	VgsOrderPopStatusPairCompleted    = "pair_completed"
	VgsOrderPopStatusBrigadeCompleted = "brigade_completed"
	VgsOrderPopStatusFailed           = "failed"
	VgsOrderPopStatusCompleted        = "completed"
)

var (
	ErrInvalidStatus = fmt.Errorf("invalid status")
	ErrInvalidAction = fmt.Errorf("invalid action")
)

// VgsPopOrder - pop order from the queue
// action:
// - vgsActionCreateBrigade
//   - status: accepted, pair_completed, failed, completed
//
// - vgsActionDeleteBrigade
//   - status: accepted, processing, failed, completed
func VgsPopOrder(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	action, status string,
) (uuid.UUID, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	query := sqfmt.Select("order_id").
		From("pairs.pair_orders")

	switch action {
	case VgsActionCreateBrigade:
		switch status {
		case VgsOrderPopStatusAccepted:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionCreateBrigade},
				sq.Eq{"brigade_completed_at": nil},
				sq.Eq{"failed_at": nil},
				sq.NotEq{"created_at": nil},
				sq.Eq{"pair_completed_at": nil},
				sq.Eq{"completed_at": nil},
			}).
				OrderBy("created_at ASC")
		case VgsOrderPopStatusPairCompleted:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionCreateBrigade},
				sq.Eq{"brigade_completed_at": nil},
				sq.Eq{"failed_at": nil},
				sq.NotEq{"created_at": nil},
				sq.NotEq{"pair_completed_at": nil},
				sq.Eq{"completed_at": nil},
			}).
				OrderBy("pair_completed_at ASC")
		case VgsOrderPopStatusCompleted:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionCreateBrigade},
				sq.Eq{"brigade_completed_at": nil},
				sq.Eq{"failed_at": nil},
				sq.NotEq{"created_at": nil},
				sq.NotEq{"pair_completed_at": nil},
				sq.NotEq{"completed_at": nil},
			}).
				OrderBy("completed_at ASC")
		case VgsOrderPopStatusFailed:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionCreateBrigade},
				sq.Eq{"brigade_completed_at": nil},
				sq.NotEq{"failed_at": nil},
			}).
				OrderBy("failed_at ASC")
		default:
			return uuid.Nil, fmt.Errorf("%w: %s", ErrInvalidStatus, status)
		}
	case VgsActionDeleteBrigade:
		switch status {
		case VgsOrderPopStatusAccepted:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionDeleteBrigade},
				sq.Eq{"pair_completed_at": nil},
				sq.Eq{"failed_at": nil},
				sq.NotEq{"created_at": nil},
				sq.Eq{"brigade_completed_at": nil},
				sq.Eq{"completed_at": nil},
			}).
				OrderBy("created_at ASC")
		case VgsOrderPopStatusBrigadeCompleted:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionDeleteBrigade},
				sq.Eq{"pair_completed_at": nil},
				sq.Eq{"failed_at": nil},
				sq.NotEq{"created_at": nil},
				sq.NotEq{"brigade_completed_at": nil},
				sq.Eq{"completed_at": nil},
			}).
				OrderBy("brigade_completed_at ASC")
		case VgsOrderPopStatusCompleted:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionDeleteBrigade},
				sq.Eq{"pair_completed_at": nil},
				sq.Eq{"failed_at": nil},
				sq.NotEq{"created_at": nil},
				sq.NotEq{"brigade_completed_at": nil},
				sq.NotEq{"completed_at": nil},
			}).
				OrderBy("completed_at ASC")
		case VgsOrderPopStatusFailed:
			query = query.Where(sq.And{
				sq.Eq{"action": VgsActionDeleteBrigade},
				sq.Eq{"pair_completed_at": nil},
				sq.NotEq{"failed_at": nil},
			}).
				OrderBy("failed_at ASC")
		default:
			return uuid.Nil, fmt.Errorf("%w: %s", ErrInvalidStatus, status)
		}

	default:
		return uuid.Nil, fmt.Errorf("%w: %s", ErrInvalidAction, action)
	}

	query = query.Limit(1)

	sql, args, err := query.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	var orderID uuid.UUID

	if err := tx.QueryRow(ctx, sql, args...).Scan(&orderID); err != nil {
		return uuid.Nil, fmt.Errorf("error getting order: %w", err)
	}

	return orderID, nil
}
