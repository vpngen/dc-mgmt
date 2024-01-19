package main

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// getBrigadesGroups - returns brigades lists on per pair basis.
func getBrigadesGroups(db *pgxpool.Pool, schema_pairs, schema_brigades string, filter string) (GroupsList, error) {
	const (
		sqlGetBrigadesGroups = `
	SELECT
		p.control_ip,
		ARRAY_AGG(b.brigade_id) AS brigade_group
	FROM
		%s AS p
	LEFT JOIN
		%s AS b ON p.pair_id = b.pair_id
	WHERE
		b.endpoint_ipv4 << $1::cidr
	GROUP BY
		p.pair_id
	HAVING
		COUNT(b.brigade_id) > 0;
	`
	)

	var prefix netip.Prefix

	switch filter {
	case "":
		prefix = netip.PrefixFrom(netip.IPv4Unspecified(), 0)
	default:
		var err error

		prefix, err = netip.ParsePrefix(filter)
		if err != nil {
			return nil, fmt.Errorf("parse prefix: %w", err)
		}
	}

	var list GroupsList

	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		fmt.Sprintf(sqlGetBrigadesGroups,
			(pgx.Identifier{schema_pairs, "pairs"}.Sanitize()),
			(pgx.Identifier{schema_brigades, "brigades"}.Sanitize()),
		),
		prefix,
	)
	if err != nil {
		return nil, fmt.Errorf("brigades groups: %w", err)
	}

	var group BrigadeGroup

	if _, err := pgx.ForEachRow(rows, []any{&group.ConnectAddr, &group.Brigades}, func() error {
		list = append(list, group)

		return nil
	}); err != nil {
		return nil, fmt.Errorf("brigade group row: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return list, nil
}
