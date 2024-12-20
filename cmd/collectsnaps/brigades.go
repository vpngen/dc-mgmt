package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// getBrigadesGroups - returns brigades lists on per pair basis.
func getBrigadesGroups(db *pgxpool.Pool, extFilter, ctrlFilter string) (GroupsList, error) {
	const (
		sqlGetBrigadesGroups = `
	SELECT
		p.control_ip,
		ARRAY_AGG(b.brigade_id) AS brigade_group,
		ARRAY_AGG(b.instance_id) AS instance_group
	FROM
		pairs.pairs AS p
	LEFT JOIN
		brigades.brigades AS b ON p.pair_id = b.pair_id
	WHERE
		b.endpoint_ipv4 <<= ANY($1::cidr[])
	AND
		p.control_ip <<= ANY($2::cidr[])
	GROUP BY
		p.pair_id
	HAVING
		COUNT(b.brigade_id) > 0;
	`
	)

	extPrefixes, err := getFilter(extFilter)
	if err != nil {
		return nil, fmt.Errorf("get ext filter: %w", err)
	}

	ctrlPrefixes, err := getFilter(ctrlFilter)
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

	rows, err := tx.Query(ctx, sqlGetBrigadesGroups, extPrefixes, ctrlPrefixes)
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

func getFilter(filter string) ([]netip.Prefix, error) {
	filter = strings.TrimSpace(filter)

	if filter == "" {
		return []netip.Prefix{netip.PrefixFrom(netip.IPv4Unspecified(), 0)}, nil
	}

	nets := strings.Split(filter, ",")

	prefixes := make([]netip.Prefix, 0, len(nets))

	for _, p := range nets {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("parse prefix: %w", err)
		}

		prefixes = append(prefixes, prefix)
	}

	return prefixes, nil
}
