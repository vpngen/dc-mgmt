package dcmgmt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sq "github.com/Masterminds/squirrel"
)

var ErrNoCommonPairs = errors.New("no common pairs found")

func tryCreateCommonBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	brigadeID uuid.UUID, brigadeName string, zone string,
) (uuid.UUID, error) {
	logger.Info("creating common brigade", "brigade_id", brigadeID, "brigade_name", brigadeName, "zone", zone)

	tx, err := db.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("error starting transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	popts, err := checkCommonPairs(ctx, logger, tx, sqfmt, zone)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Info("no common pairs found")

			return uuid.Nil, ErrNoCommonPairs
		}

		return uuid.Nil, fmt.Errorf("error checking common pairs: %w", err)
	}

	now := time.Now().UTC()

	// create a new number.
	queryNum := sqfmt.Insert("pairs.endpoint_nums").
		Columns("update_time", "zone").
		Values(now, zone).
		Suffix("RETURNING endpoint_num")

	sql, args, err := queryNum.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	var numID int

	if err := tx.QueryRow(ctx, sql, args...).Scan(&numID); err != nil {
		return uuid.Nil, fmt.Errorf("error inserting number (2): %w", err)
	}

	// link the number to the pair.
	queryLink := sqfmt.Update("pairs.pairs_endpoints_ipv4").
		Set("endpoint_num", numID).
		Where(sq.Eq{"endpoint_ipv4": popts.endpointIPv4})

	sql, args, err = queryLink.ToSql()
	if err != nil {
		return uuid.Nil, fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return uuid.Nil, fmt.Errorf("error linking number to pair: %w", err)
	}

	// create a new order.
	query := sqfmt.Insert("pairs.pair_orders").
		Columns("endpoint_num", "brigade_id", "brigade_name", "zone", "action", "created_at", "pair_completed_at", "message").
		Values(numID, brigadeID, brigadeName, zone, VgsActionCreateBrigade, now, now, "").
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

func checkCommonPairs(ctx context.Context, _ *slog.Logger, tx pgx.Tx, sqfmt sq.StatementBuilderType,
	zone string,
) (*pairOpts, error) {
	subQuery := sqfmt.Select("pair_id").
		From("brigades.active_pairs").
		Where(sq.Eq{"zone": zone}).
		OrderBy("free_slots_count DESC").
		Limit(1)

	subSql, subArgs, err := subQuery.ToSql()
	if err != nil {
		return nil, fmt.Errorf("error building SQL: %w", err)
	}

	var pair uuid.UUID

	if err := tx.QueryRow(ctx, subSql, subArgs...).Scan(&pair); err != nil {
		return nil, fmt.Errorf("error getting active pair: %w", err)
	}

	query := sqfmt.Select("pair_id", "control_ip", "endpoint_ipv4", "domain_name").
		From("brigades.slots").
		Where(sq.Eq{"pair_id": pair}).
		OrderBy("domain_name NULLS LAST").
		Limit(1)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("error building SQL: %w", err)
	}

	var (
		opts   pairOpts
		domain pgtype.Text
	)

	if err := tx.QueryRow(ctx, sql, args...).Scan(&opts.pairID, &opts.controlIP, &opts.endpointIPv4, &domain); err != nil {
		return nil, fmt.Errorf("error getting slot: %w", err)
	}

	opts.domain = domain.String

	return &opts, nil
}
