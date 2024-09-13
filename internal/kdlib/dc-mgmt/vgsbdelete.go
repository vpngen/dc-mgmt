package dcmgmt

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"golang.org/x/crypto/ssh"
)

const (
	deleteAttempts = 5
)

var ErrDeleteAttemptsCountExceeded = errors.New("delete attempts count exceeded")

func VgsDeleteBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
	dcident string,
	host, token string,
	sshkey, sshuser, server string,
	doNotCreatePhy bool,
) error {
	logger.Info("deleting brigade", "order_id", orderID)

	_, _, _, controlIP, _, brigadeID, _, err := vgsGetOrderMeta(ctx, logger, db, sqfmt, orderID, false)
	if err != nil {
		return fmt.Errorf("error getting order brigade meta: %w", err)
	}

	logger.Info("deleting brigade", "brigade_id", brigadeID, "order_id", orderID, "control_ip", controlIP)

	if !doNotCreatePhy {
		sshconf, err := kdlib.CreateSSHConfig(sshkey, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
		if err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error creating ssh configs: %w", err)
		}

		bid := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID[:])

		if err := vgsRevokeBrigade(ctx, logger, sshconf, bid, controlIP); err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error revoking brigade: %w", err)
		}
	}

	sshconf, err := kdlib.CreateSSHConfig(sshkey, sshuser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating ssh configs: %w", err)
	}

	if err := vgsRemoveBrigade(ctx, logger, db, sqfmt, dcident,
		brigadeID.String(),
		server, sshconf, host, token,
	); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error deleting brigade: %w", err)
	}

	logger.Info("brigade deleted", "brigade_id", brigadeID, "order_id", orderID)

	return nil
}

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

func vgsRemoveBrigade(
	ctx context.Context, _ *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
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

	queryDelStats := sqfmt.Delete("stats.brigades_stats").
		Where(sq.Eq{"brigade_id": brigadeID})

	sql, args, err := queryDelStats.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("brigades stats delete: %w", err)
	}

	var domain_name pgtype.Text

	queryGetDomain := sqfmt.Select("domain_name").
		From("brigades.brigades").
		Where(sq.Eq{"brigade_id": brigadeID, "main": true})

	sql, args, err = queryGetDomain.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if err := tx.QueryRow(ctx, sql, args...).Scan(&domain_name); err != nil {
		return fmt.Errorf("get domain name: %w", err)
	}

	queryDel := sqfmt.Delete("brigades.brigades").
		Where(sq.Eq{"brigade_id": brigadeID})

	sql, args, err = queryDel.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("brigade delete: %w", err)
	}

	queryComplete := sqfmt.Update("pairs.pair_orders").
		Set("brigade_completed_at", time.Now().UTC()).
		Where(sq.Eq{"brigade_id": brigadeID, "action": VgsActionDeleteBrigade})

	sql, args, err = queryComplete.ToSql()
	if err != nil {
		return fmt.Errorf("error building SQL: %w", err)
	}

	if _, err := tx.Exec(ctx, sql, args...); err != nil {
		return fmt.Errorf("brigade complete: %w", err)
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
