package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sq "github.com/Masterminds/squirrel"
)

func GetControlAddr(ctx context.Context, _ *slog.Logger,
	db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	brigadeID uuid.UUID,
) (netip.Addr, error) {
	// 1. get control ip from brigade
	// 2. return control ip

	tx, err := db.Begin(ctx)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("beginning transaction: %w", err)
	}

	defer tx.Rollback(ctx)

	query := sqfmt.Select("p.control_ip").
		From("brigades.brigades b").
		Join("pairs.pairs p ON b.pair_id = p.pair_id").
		Where(sq.Eq{"b.brigade_id": brigadeID})

	sql, args, err := query.ToSql()
	if err != nil {
		return netip.Addr{}, fmt.Errorf("building query: %w", err)
	}

	var controlIP netip.Addr

	if err := tx.QueryRow(ctx, sql, args...).Scan(&controlIP); err != nil {
		return netip.Addr{}, fmt.Errorf("querying control ip: %w", err)
	}

	return controlIP, nil
}
