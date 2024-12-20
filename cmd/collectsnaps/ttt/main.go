package main

import (
	"context"
	"fmt"
	"log"
	"net/netip"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
)

func main() {
	ctx := context.Background()

	db, err := kdlib.CreateDBPool(ctx, "postgresql:///vgrealm")
	if err != nil {
		log.Fatalf("Can't create db pool: %s\n", err)
	}

	defer db.Close()

	const (
		query = `
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

	extPrefix, err := getFilter("49.13.215.145/32,167.235.205.64/32")
	if err != nil {
		log.Fatalf("get ext filter: %s", err)
	}

	ctrlPrefix, err := getFilter("10.100.0.2/32")
	if err != nil {
		log.Fatalf("get ctrl filter: %s", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		log.Fatalf("begin: %s", err)
	}

	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, query, extPrefix, ctrlPrefix)
	if err != nil {
		log.Fatalf("brigades groups: %s", err)
	}

	var (
		addr      netip.Addr
		brigades  []uuid.UUID
		instances []uuid.UUID
	)

	if _, err := pgx.ForEachRow(rows, []any{&addr, &brigades, &instances}, func() error {
		fmt.Fprintf(os.Stderr, "addr: %s, brigades: %v, instances: %v\n", addr, brigades, instances)

		return nil
	}); err != nil {
		log.Fatalf("brigade group row: %s", err)
	}
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
