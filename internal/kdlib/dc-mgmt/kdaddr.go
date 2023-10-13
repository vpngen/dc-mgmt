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
	KdAddrSyncFilename       = "vpn-works-%s.csv"
	KdAddrSyncReloadFilename = "vpn-works-keydesks.reload"
)

func SyncKdAddrList(sshconf *ssh.ClientConfig, kdAddrSyncServer, ident, kdAddrList string) (func(string), error) {
	// fmt.Fprintf(os.Stderr, "%s: %s@%s\n", logtag, sshconf.User, kdAddrSyncServer)

	client, b, e, cleanup, err := kdlib.NewSSHCient(sshconf, kdAddrSyncServer)
	if err != nil {
		return cleanup, fmt.Errorf("new ssh client: %w", err)
	}

	defer client.Close()

	fn := fmt.Sprintf(KdAddrSyncFilename, ident)
	cmdSync := fmt.Sprintf("dd status=none of=%s.tmp && mv -f %s.tmp %s", fn, fn, fn)
	// fmt.Fprintf(os.Stderr, "%s:       -> %s\n", logtag, cmdSync)

	if err := kdlib.SSHSessionStart(client, b, e, cmdSync, strings.NewReader(kdAddrList)); err != nil {
		return cleanup, fmt.Errorf("write remote file: %w", err)
	}

	cmdReload := fmt.Sprintf("touch %s", KdAddrSyncReloadFilename)
	// fmt.Fprintf(os.Stderr, "%s:       -> %s\n", logtag, cmdReload)

	if err := kdlib.SSHSessionRun(client, b, e, cmdReload); err != nil {
		return cleanup, fmt.Errorf("touch remote file: %w", err)
	}

	return cleanup, nil
}

func NewKdAddrList(ctx context.Context, db *pgxpool.Pool, schema string) (string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	sqlGetKeydeskAddressList := `	
SELECT 
	endpoint_ipv4,
	keydesk_ipv6 
FROM 
	%s
	`

	rows, err := tx.Query(
		ctx,
		fmt.Sprintf(
			sqlGetKeydeskAddressList,
			pgx.Identifier{schema, "brigades"}.Sanitize(),
		),
	)
	if err != nil {
		return "", fmt.Errorf("keydesk-address query: %w", err)
	}

	var (
		endpoint_ipv4 netip.Addr
		keydesk_ipv6  netip.Addr
		list          string
	)

	if _, err := pgx.ForEachRow(rows, []any{&endpoint_ipv4, &keydesk_ipv6}, func() error {
		list += fmt.Sprintf("%s;%s\n", endpoint_ipv4, keydesk_ipv6)

		return nil
	}); err != nil {
		return "", fmt.Errorf("keydesk-address row: %w", err)
	}

	return list, nil
}
