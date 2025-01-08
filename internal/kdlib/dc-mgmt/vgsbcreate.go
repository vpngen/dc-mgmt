package dcmgmt

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strings"
	"sync"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgtype/zeronull"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	"github.com/vpngen/wordsgens/namesgenerator"
	"golang.org/x/crypto/ssh"
)

const (
	sshkeyRemoteUsername = "_serega_"
)

var ErrNotDelegated = errors.New("not delegated")

type subdomAPI struct {
	host    string
	token   string
	srvZone string
}

type delegationSync struct {
	sshconf *ssh.ClientConfig
	server  string
}

type VpnCfgs struct {
	Wg      string
	Ovc     string
	Ipsec   string
	Outline string
	Proto0  string
}

type brigadeOpts struct {
	id     string
	name   string
	person namesgenerator.Person
}

type pairOpts struct {
	pairID       uuid.UUID
	controlIP    netip.Addr
	endpointIPv4 netip.Addr
	domain       string
}

func VgsCreateBrigade(ctx context.Context, logger *slog.Logger, db *pgxpool.Pool, sqfmt sq.StatementBuilderType,
	orderID uuid.UUID,
	dcident string,
	host, token string,
	sshkey, sshuser, server string,
	ns []string, vpnCfgs *VpnCfgs,
	maxusers int,
	doNotCreatePhy bool,
) error {
	logger.Info("creating brigade", "order_id", orderID)

	_, zone, pairID, _, controlIP, endpointIP, brigadeID, brigadeName, err := vgsGetOrderMeta(ctx, logger, db, sqfmt, orderID, false)
	if err != nil {
		return fmt.Errorf("error getting order brigade meta: %w", err)
	}

	logger.Info("fetch order", "brigade_id", brigadeID, "brigade_name", brigadeName, "order_id", orderID, "control_ip", controlIP, "endpoint_ipv4", endpointIP)

	sshconf, err := kdlib.CreateSSHConfig(sshkey, sshuser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating ssh configs: %w", err)
	}

	if err := vgsCreateBrigade(ctx, db, logger, dcident,
		&brigadeOpts{
			id:   brigadeID.String(),
			name: brigadeName,
		},
		&pairOpts{
			pairID:       pairID,
			endpointIPv4: endpointIP,
			controlIP:    controlIP,
		},
		&delegationSync{
			sshconf: sshconf,
			server:  server,
		},
		&subdomAPI{
			host:    host,
			token:   token,
			srvZone: zone,
		},
	); err != nil {
		if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
			return fmt.Errorf("error setting order error: %w", err)
		}

		return fmt.Errorf("error creating brigade: %w", err)
	}

	if !doNotCreatePhy {
		sshconf, err := kdlib.CreateSSHConfig(sshkey, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
		if err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error creating ssh configs: %w", err)
		}

		if err := vgsRequestBrigade(ctx, db, logger, sshconf, brigadeID.String(), ns, vpnCfgs, maxusers); err != nil {
			if err := vgsSetOrderError(ctx, logger, db, sqfmt, orderID, err.Error()); err != nil {
				return fmt.Errorf("error setting order error: %w", err)
			}

			return fmt.Errorf("error requesting brigade: %w", err)
		}
	}

	if err := vgsSetOrderComplete(ctx, logger, db, sqfmt, orderID); err != nil {
		return fmt.Errorf("error setting order completed: %w", err)
	}

	logger.Info("brigade created", "brigade_id", brigadeID, "brigade_name", brigadeName, "order_id", orderID)

	return nil
}

func vgsCreateBrigade(
	ctx context.Context,
	db *pgxpool.Pool,
	logger *slog.Logger,
	dcident string,
	bopts *brigadeOpts,
	popts *pairOpts,
	delegationSync *delegationSync,
	subdomAPI *subdomAPI,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	logger.Debug("create brigade", "endpoint_ipv4", popts.endpointIPv4, "control_ip", popts.controlIP)

	if popts.domain != "" {
		logger.Debug("create brigade", "domain", popts.domain)
	}

	// pick up cgnat
	cgnatNet, err := RandomCGNAT24Net()
	if err != nil {
		return fmt.Errorf("random cgnat: %w", err)
	}

	logger.Debug("create brigade", "cgnat_net", cgnatNet)

	// pick up ula
	ulaNet, err := RandomULA64Net()
	if err != nil {
		return fmt.Errorf("random ula: %w", err)
	}

	logger.Debug("create brigade", "ula_net", ulaNet)

	// pick up keydesk
	keydesk, err := RandomKeydesk()
	if err != nil {
		return fmt.Errorf("random keydesk: %w", err)
	}

	logger.Debug("create brigade", "keydesk", keydesk)

	// create brigade
	sqlCreateBrigade := `
INSERT INTO brigades.brigades
		(
			brigade_id, brigadier, person,
			pair_id, endpoint_ipv4,	domain_name,       
			dns_ipv4, dns_ipv6, keydesk_ipv6,        
			ipv4_cgnat, ipv6_ula,            
			main              
		)
VALUES 
		(
			$1, $2,	$3,
			$4, $5,	$6,
			$7, $8,	$9,
			$10, $11,
			true
		)
RETURNING instance_id;
`

	var instanceID uuid.UUID

	if err = tx.QueryRow(ctx,
		sqlCreateBrigade,
		bopts.id, bopts.name, bopts.person,
		popts.pairID, popts.endpointIPv4, zeronull.Text(popts.domain),
		cgnatNet.Addr(), ulaNet.Addr(), keydesk, cgnatNet, ulaNet,
	).Scan(&instanceID); err != nil {
		return fmt.Errorf("create brigade: %w", err)
	}

	sqlInsertStats := `INSERT INTO stats.brigades_stats (brigade_id, instance_id) VALUES ($1,$2);`

	if _, err = tx.Exec(ctx, sqlInsertStats, bopts.id, instanceID); err != nil {
		return fmt.Errorf("create stats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Pick up subdomain.
	if popts.domain == "" {
		if err := ApplySubdomain(ctx, db, subdomAPI.host, subdomAPI.token, bopts.id, popts.endpointIPv4, subdomAPI.srvZone); err != nil {
			return fmt.Errorf("apply subdomain: %w", err)
		}
	}

	// Sync delegation list.
	delegationList, err := NewDelegationList(ctx, db, "brigades")
	if err != nil {
		return fmt.Errorf("delegation list: %w", err)
	}

	logger.Debug("create brigade", "delegation_list", delegationList)

	cleanup, err := SyncDelegationList(delegationSync.sshconf, delegationSync.server, dcident, delegationList)
	cleanup("create brigade")

	if err != nil {
		return fmt.Errorf("delegation sync: %w", err)
	}

	return nil
}

func vgsRequestBrigade(
	ctx context.Context,
	db *pgxpool.Pool,
	logger *slog.Logger,
	sshconf *ssh.ClientConfig,
	bid string,
	ns []string,
	vpnCfgs *VpnCfgs,
	maxusers int,
) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	var (
		brigadeID    []byte
		endpointIPv4 netip.Addr
		domain       pgtype.Text
		dnsIPv4      netip.Addr
		dnsIPv6      netip.Addr
		ipv4CGNAT    netip.Prefix
		ipv6ULA      netip.Prefix
		control_ip   netip.Addr
		kdIPv6       netip.Addr
		nameservers  pgtype.Text
	)

	sqlFetchBrigade := `
SELECT
	meta_brigades.brigade_id,
	meta_brigades.endpoint_ipv4,
	meta_brigades.domain_name,
	meta_brigades.dns_ipv4,
	meta_brigades.dns_ipv6,
	meta_brigades.ipv4_cgnat,
	meta_brigades.ipv6_ula,
	meta_brigades.control_ip,
	meta_brigades.keydesk_ipv6,
	meta_brigades.nameservers
FROM brigades.meta_brigades
WHERE
	meta_brigades.brigade_id=$1
`

	err = tx.QueryRow(ctx, sqlFetchBrigade, bid).Scan(
		&brigadeID,
		&endpointIPv4,
		&domain,
		&dnsIPv4,
		&dnsIPv6,
		&ipv4CGNAT,
		&ipv6ULA,
		&control_ip,
		&kdIPv6,
		&nameservers,
	)
	if err != nil {
		return fmt.Errorf("brigade query: %w", err)
	}

	// cmd := fmt.Sprintf("create -id %s -ep4 %s -int4 %s -int6 %s -dns4 %s -dns6 %s -kd6 %s -name %s -person %s -desc %s -url %s -dn %s -ch -j",
	cmd := fmt.Sprintf("create -id %s -ep4 %s -int4 %s -int6 %s -dns4 %s -dns6 %s -kd6 %s -dn %s -j -mode vgsocket -maxusers %d",
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID),
		endpointIPv4,
		ipv4CGNAT,
		ipv6ULA,
		dnsIPv4,
		dnsIPv6,
		kdIPv6,
		domain.String,
		maxusers,
	)

	if vpnCfgs != nil {
		if vpnCfgs.Wg != "" {
			cmd += fmt.Sprintf(" -wg %s", vpnCfgs.Wg)
		}

		if vpnCfgs.Ovc != "" {
			cmd += fmt.Sprintf(" -ovc %s", vpnCfgs.Ovc)
		}

		if vpnCfgs.Ipsec != "" {
			cmd += fmt.Sprintf(" -ipsec %s", vpnCfgs.Ipsec)
		}

		if vpnCfgs.Outline != "" {
			cmd += fmt.Sprintf(" -outline %s", vpnCfgs.Outline)
		}

		if vpnCfgs.Proto0 != "" {
			cmd += fmt.Sprintf(" -proto0 %s", vpnCfgs.Proto0)
		}
	}

	logger.Debug("request brigade", "cmd", cmd, "control_ip", control_ip)

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", control_ip), sshconf)
	if err != nil {
		return fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var b, e bytes.Buffer

	session.Stdout = &b
	session.Stderr = &e

	defer func() {
		switch errstr := e.String(); errstr {
		case "":
			logger.Debug("request brigade", "ssh session stderr", "empty")
		default:
			for _, line := range strings.Split(errstr, "\n") {
				logger.Debug("request brigade", "ssh session stderr", line)
			}
		}
	}()

	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("ssh run: %w", err)
	}

	// payload, err := io.ReadAll(httputil.NewChunkedReader(&b))
	if _, err := io.ReadAll(&b); err != nil {
		logger.Debug("request brigade", "chunk read", err)

		return fmt.Errorf("chunk read: %w", err)
	}

	logger.Debug("waiting for delegation", "domain_name", domain.String, "endpoint_ipv4", endpointIPv4)

	nss := strings.Split(nameservers.String, ",")
	if len(nss) == 0 {
		nss = ns
	}

	if !vgsWaitForAllDelegations(logger, domain.String, endpointIPv4, nss) {
		return fmt.Errorf("delegation: %w", ErrNotDelegated)
	}

	return nil
}

func vgsWaitForAllDelegations(logger *slog.Logger, domain string, endpointIPv4 netip.Addr, domainNS []string) bool {
	var domainOk bool

	wg := &sync.WaitGroup{}

	if domain != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ok, err := WaitForDelegation(domain, endpointIPv4, domainNS...)
			if err != nil {
				logger.Error("wait for delegation", "domain", domain, "endpoint_ipv4", endpointIPv4, "err", err)
			}

			domainOk = ok
		}()
	}

	wg.Wait()

	return domainOk
}
