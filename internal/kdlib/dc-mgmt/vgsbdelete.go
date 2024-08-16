package dcmgmt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"
)

const (
	deleteAttempts = 5
)

var ErrDeleteAttemptsCountExceeded = errors.New("delete attempts count exceeded")

func vgsRevokeBrigade(_ context.Context, logger *slog.Logger, sshconf *ssh.ClientConfig, brigadeID string, controlIP netip.Addr) error {
	cmd := fmt.Sprintf("destroy -id %s -ch", brigadeID)

	logger.Debug("revoke brigade", "cmd", cmd)

	for attemts := 0; attemts <= deleteAttempts; attemts++ {
		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", controlIP), sshconf)
		if err != nil {
			logger.Debug("revoke brigade", "ssh dial", err)

			continue
		}

		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			logger.Debug("revoke brigade", "ssh session", err)

			continue
		}

		defer session.Close()

		var b, e bytes.Buffer

		session.Stdout = &b
		session.Stderr = &e

		defer func() {
			switch errstr := e.String(); errstr {
			case "":
				logger.Debug("revoke brigade", "ssh session stderr", "empty")
			default:
				for _, line := range strings.Split(errstr, "\n") {
					logger.Warn("revoke brigade", "ssherr", line)
				}
			}
		}()

		if err := session.Run(cmd); err != nil {
			return fmt.Errorf("ssh run: %w", err)
		}

		return nil
	}

	return fmt.Errorf("%w: %d", ErrDeleteAttemptsCountExceeded, deleteAttempts)
}

func vgsRemoveBrigadeFull(
	ctx context.Context, db *pgxpool.Pool, _ *slog.Logger,
	ident string,
	brigadeID string,
	delegationSyncServer string, delegationSyncSSHconf *ssh.ClientConfig,
	subdomAPIHost, subdomAPIToken string,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	sqlDelBrigadesStats := `
	DELETE
		FROM stats.brigades_stats
	WHERE 
		brigade_id=$1
	`

	if _, err := tx.Exec(ctx, sqlDelBrigadesStats, brigadeID); err != nil {
		return fmt.Errorf("brigades stats delete: %w", err)
	}

	var domain_name pgtype.Text

	getDomainName := `SELECT domain_name FROM brigades.brigades WHERE brigade_id=$1 AND is_main=true`

	if err := tx.QueryRow(ctx, getDomainName, brigadeID).Scan(&domain_name); err != nil {
		return fmt.Errorf("get domain name: %w", err)
	}

	sqlDelBrigade := `DELETE FROM brigades.brigades	WHERE brigade_id=$1`

	if _, err := tx.Exec(ctx, sqlDelBrigade, brigadeID); err != nil {
		return fmt.Errorf("brigade delete: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if domain_name.Valid {
		if err := RevokeSubdomain(ctx, db, subdomAPIHost, subdomAPIToken, domain_name.String); err != nil {
			return fmt.Errorf("revoke subdomain: %w", err)
		}
	}

	// Sync delegation list.

	delegationList, err := NewDelegationList(ctx, db, "brigades")
	if err != nil {
		return fmt.Errorf("delegation list: %w", err)
	}

	cleanup, err := SyncDelegationList(delegationSyncSSHconf, delegationSyncServer, ident, delegationList)
	cleanup("remove brigade")

	if err != nil {
		return fmt.Errorf("delegation sync: %w", err)
	}

	return nil
}
