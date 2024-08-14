package dcmgmt

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
)

const (
	subdomainAPIAttempts = 5
	subdomainAPISleep    = 2 * time.Second
)

func ApplySubdomain(ctx context.Context, db *pgxpool.Pool, apihost, apitoken string, brigadeID string, endpointIPv4 netip.Addr) error {
	if apitoken == NoUseSubdomainAPIToken {
		fmt.Fprintf(os.Stderr, "subdomain api token is set to dry-run\n")

		return nil
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	var (
		domainName pgtype.Text
		subdomain  string
	)

	for i := 0; i < subdomainAPIAttempts; i++ {
		subdomain, err = kdlib.SubdomainPick(apihost, apitoken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Can't pick subdomain (%d): %s\n", i+1, err)

			if i == subdomainAPIAttempts-1 {
				return fmt.Errorf("pick subdomain: %w", err)
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("context done: %w", ctx.Err())
			case <-time.After(subdomainAPISleep):
			}

			continue
		}

		break
	}

	if err := domainName.Scan(subdomain); err != nil {
		return fmt.Errorf("scan subdomain: %w", err)
	}

	sqlInsertPairDomain := `INSERT INTO brigades.domains_endpoints_ipv4 (domain_name, endpoint_ipv4) VALUES ($1,$2)`

	if _, err := tx.Exec(ctx, sqlInsertPairDomain, domainName, endpointIPv4); err != nil {
		return fmt.Errorf("pair domain update: %w", err)
	}

	sqlUpdateBrigadeDomain := `UPDATE brigades.brigades SET domain_name=$1 WHERE brigade_id=$2`

	if _, err := tx.Exec(ctx, sqlUpdateBrigadeDomain, domainName, brigadeID); err != nil {
		return fmt.Errorf("brigade domain update: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}
