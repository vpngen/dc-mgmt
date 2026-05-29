package dcmgmt

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"golang.org/x/crypto/ssh"
)

const (
	DelegationSyncFilename       = "domain-generate-%s.csv"
	DelegationSyncReloadFilename = "domain-generate.reload"
)

const (
	DomainCheckPause         = 5 * time.Second
	DomainDelegationWaitTime = 120 * time.Second
)

var ErrCheckAttemptExceeded = errors.New("check attempt exceeded")

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
	d.domain_name,
	d.endpoint_ipv4 
FROM 
	brigades.domains_endpoints_ipv4 d
JOIN
	pairs.pairs_endpoints_ipv4 pei ON d.endpoint_ipv4 = pei.endpoint_ipv4
JOIN
	pairs.pairs p ON pei.pair_id = p.pair_id
WHERE
	p.zone NOT IN ('astra', 'bfst', 'bmecte', 'feygin', 'kovcheg', 'insider', 'theins', 'zicer', 'zona', 'gena', 'headquaters', 'naki', 'dobro')
	`

	rows, err := tx.Query(ctx, sqlGetDelegationList)
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

func WaitForDelegation(fqdn string, ip netip.Addr, ns ...string) (bool, error) {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()

	finish := time.Now().Add(DomainDelegationWaitTime)

	fmt.Fprintf(os.Stderr, "waiting for delegation: %s -> %s %v\n", fqdn, ip, ns)

	for ts := range timer.C {
		if ok, err := CheckForPresence(fqdn, ip, ns...); ok && err == nil {
			return ok, nil
		}

		if ts.After(finish) {
			return false, ErrCheckAttemptExceeded
		}

		timer.Reset(DomainCheckPause)
	}

	return false, nil
}
