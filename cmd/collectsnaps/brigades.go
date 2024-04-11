package main

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// getBrigadesGroups - returns brigades lists on per pair basis.
func getBrigadesGroups(db *pgxpool.Pool, schema_pairs, schema_brigades string, extFilter, ctrlFilter string) (GroupsList, error) {
	const (
		sqlGetBrigadesGroups = `
	SELECT
		p.control_ip,
		ARRAY_AGG(b.brigade_id) AS brigade_group
		ARRAY_AGG(b.instance_id) AS instance_group
	FROM
		%s AS p
	LEFT JOIN
		%s AS b ON p.pair_id = b.pair_id
	WHERE
		b.endpoint_ipv4 << $1::cidr
	AND
		p.control_ip << $2::cidr
	GROUP BY
		p.pair_id
	HAVING
		COUNT(b.brigade_id) > 0;
	`
	)

	extPrefix, err := getFilter(extFilter)
	if err != nil {
		return nil, fmt.Errorf("get ext filter: %w", err)
	}

	ctrlPrefix, err := getFilter(ctrlFilter)
	if err != nil {
		return nil, fmt.Errorf("get ctrl filter: %w", err)
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
		extPrefix,
		ctrlPrefix,
	)
	if err != nil {
		return nil, fmt.Errorf("brigades groups: %w", err)
	}

	var (
		addr      netip.Addr
		brigades  []uuid.UUID
		instances []uuid.UUID
	)

	if _, err := pgx.ForEachRow(rows, []any{&addr, &brigades, &instances}, func() error {
		group := BrigadeGroup{
			ConnectAddr: addr,
			Brigades:    make(map[uuid.UUID]uuid.UUID),
		}

		for i, b := range brigades {
			group.Brigades[b] = instances[i]
		}

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

func getFilter(filter string) (netip.Prefix, error) {
	if filter == "" {
		return netip.PrefixFrom(netip.IPv4Unspecified(), 0), nil
	}

	prefix, err := netip.ParsePrefix(filter)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("parse prefix: %w", err)
	}

	return prefix, nil
}
