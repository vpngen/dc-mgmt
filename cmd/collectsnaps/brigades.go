package main

import (
	"context"
	"encoding/base32"
	"fmt"
	"net/netip"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	dcroot "github.com/vpngen/dc-mgmt"
)

// getBrigadesGroups - returns brigades lists on per pair basis.
func getBrigadesGroups(db *pgxpool.Pool, extPrefixes, ctrlPrefixes []netip.Prefix) (GroupsList, error) {
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

// getBrigadesGroupsFromPlan - returns brigades lists on per pair basis from the plan.
func getBrigadesGroupsFromPlan(db *pgxpool.Pool, plan *dcroot.SnapshotPlan) (GroupsList, error) {
	const (
		sqlGetBrigade = `
	SELECT
		b.instance_id
	FROM
		pairs.pairs AS p
	JOIN
		brigades.brigades AS b ON p.pair_id = b.pair_id
	WHERE
		b.brigade_id = $1
	AND
		b.endpoint_ipv4 = $2
	AND
		p.control_ip = $3;
	`
	)

	var list GroupsList

	ctx := context.Background()

	for _, s := range plan.Plan {
		cip, err := netip.ParseAddr(s.ControlIP)
		if err != nil {
			fmt.Fprintf(os.Stderr, "control ip %s is not valid: %s\n", s.ControlIP, err)

			continue
		}

		bg := BrigadeGroup{
			ConnectAddr: cip,
			Brigades:    make(map[uuid.UUID]uuid.UUID),
		}

		for _, r := range s.Snaps {
			buf, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(r.BrigadeID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "brigade id %s is not valid: %s\n", r.BrigadeID, err)

				continue
			}

			bid := uuid.UUID(buf)

			var iid uuid.UUID

			if err := db.QueryRow(ctx, sqlGetBrigade, bid, r.EndpointIPv4, s.ControlIP).Scan(&iid); err != nil {
				if err != pgx.ErrNoRows {
					return nil, fmt.Errorf("brigade group: %w", err)
				}
			}

			bg.Brigades[bid] = bid
		}

		list = append(list, bg)
	}

	return list, nil
}
