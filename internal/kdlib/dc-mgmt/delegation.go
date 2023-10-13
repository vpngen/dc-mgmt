package dcmgmt

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"golang.org/x/crypto/ssh"
)

const (
	DelegationSyncFilename       = "domain-generate-%s.csv"
	DelegationSyncReloadFilename = "domain-generate.reload"
)

func SyncDelegationList(sshconf *ssh.ClientConfig, delegationSyncServer, ident, kdAddrList string) (func(string), error) {
	// fmt.Fprintf(os.Stderr, "%s: %s@%s\n", logtag, sshconf.User, delegationSyncServer)

	client, b, e, cleanup, err := kdlib.NewSSHCient(sshconf, delegationSyncServer)
	if err != nil {
		return cleanup, fmt.Errorf("new ssh client: %w", err)
	}

	defer client.Close()

	fn := fmt.Sprintf(DelegationSyncFilename, ident)
	cmdSync := fmt.Sprintf("dd status=none of=%s.tmp && mv -f %s.tmp %s", fn, fn, fn)
	// fmt.Fprintf(os.Stderr, "%s:       -> %s\n", logtag, cmdSync)

	if err := kdlib.SSHSessionStart(client, b, e, cmdSync, strings.NewReader(kdAddrList)); err != nil {
		return cleanup, fmt.Errorf("write remote file: %w", err)
	}

	cmdReload := fmt.Sprintf("touch %s", DelegationSyncReloadFilename)
	// fmt.Fprintf(os.Stderr, "%s:       -> %s\n", logtag, cmdReload)

	if err := kdlib.SSHSessionRun(client, b, e, cmdReload); err != nil {
		return cleanup, fmt.Errorf("touch remote file: %w", err)
	}

	return cleanup, nil
}

func NewDelegationList(ctx context.Context, db *pgxpool.Pool, schema string) (string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	sqlGetDelegationList := `
SELECT 
	domain_name,
	endpoint_ipv4 
FROM 
	%s
	`

	rows, err := tx.Query(
		ctx,
		fmt.Sprintf(
			sqlGetDelegationList,
			pgx.Identifier{schema, "domains_endpoints_ipv4"}.Sanitize(),
		),
	)
	if err != nil {
		return "", fmt.Errorf("delegation query: %w", err)
	}

	var (
		domain_name   string
		endpoint_ipv4 netip.Addr
		list          string
	)

	if _, err := pgx.ForEachRow(rows, []any{&domain_name, &endpoint_ipv4}, func() error {
		list += fmt.Sprintf("%s;%s\n", domain_name, endpoint_ipv4)

		return nil
	}); err != nil {
		return "", fmt.Errorf("delegation row: %w", err)
	}

	return list, nil
}
