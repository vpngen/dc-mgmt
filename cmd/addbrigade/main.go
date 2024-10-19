package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgtype/zeronull"
	"github.com/jackc/pgx/v5/pgxpool"

	"golang.org/x/crypto/ssh"

	dcmgmt "github.com/vpngen/dc-mgmt"
	"github.com/vpngen/dc-mgmt/internal/kdlib"
	dcmgmtlib "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"
	"github.com/vpngen/keydesk/gen/models"
	"github.com/vpngen/keydesk/kdlib/lockedfile"
	"github.com/vpngen/keydesk/keydesk"
	"github.com/vpngen/wordsgens/namesgenerator"
)

const (
	brigadesSchema      = "brigades"
	brigadesStatsSchema = "stats"
)

const (
	sshkeyRemoteUsername = "_serega_"
	sshkeyDefaultPath    = "/etc/vg-dc-vpnapi"
)

const (
	defaultDatabaseURL = "postgresql:///vgrealm"
)

const (
	BrigadeCgnatPrefix = 24
	BrigadeUlaPrefix   = 64
)

const (
	DefaultDelegationMutexPath = "/tmp/delegation_mutex.lock"
)

const defaultWireguardConfigs = "native"

const DefaultServiceZone = ""

type brigadeOpts struct {
	id      string
	name    string
	forceIP netip.Addr
	person  namesgenerator.Person
}

type dbEnv struct {
	dbURL string
}

type dcEnv struct {
	ident string
}

type subdomainAccessEnv struct {
	subdomainAPIHost  string
	subdomainAPIToken string
}

type kdAddrSyncEnv struct {
	kdAddrUser   string
	kdAddrServer string
}

type delegationSyncEnv struct {
	delegationMutex  string
	delegationUser   string
	delegationServer string
}

type delegationCheckEnv struct {
	kdDomain string
	kdNS     []string
	domainNS []string
}

type vpnCfgs struct {
	wg      string
	ovc     string
	ipsec   string
	outline string
	proto0  string
}

type envOpts struct {
	dbEnv
	dcEnv
	subdomainAccessEnv
	kdAddrSyncEnv
	delegationSyncEnv
	delegationCheckEnv
	vpnCfgs
	Zone string
}

// Args errors.
var (
	ErrEmptyBrigadierName   = errors.New("empty brigadier name")
	ErrInvalidBrigadierName = errors.New("invalid brigadier name")
	ErrEmptyPersonName      = errors.New("empty person name")
	ErrEmptyPersonDesc      = errors.New("empty person desc")
	ErrEmptyPersonURL       = errors.New("empty person url")
	ErrInvalidPersonName    = errors.New("invalid person name")
	ErrInvalidPersonDesc    = errors.New("invalid person desc")
	ErrInvalidPersonURL     = errors.New("invalid person url")
	ErrNoSSHKeyFile         = errors.New("no ssh key file")
)

// SubdomAPI config errors.
var (
	ErrEmptySubdomAPIServer = errors.New("empty subdomapi host")
	ErrEmptySubdomAPIToken  = errors.New("empty subdomapi token")
)

var ErrNotDelegated = errors.New("not delegated")

var LogTag = setLogTag()

const defaultLogTag = "addbrigade"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	var w io.WriteCloser

	chunked, jout, opts, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	switch chunked {
	case true:
		w = httputil.NewChunkedWriter(os.Stdout)
		defer w.Close()
	default:
		w = os.Stdout
	}

	sshKeyFilename, env, err := readConfigs()
	if err != nil {
		fatal(w, jout, "%s: Can't read configs: %s\n", LogTag, err)
	}

	sshconf, err := kdlib.CreateSSHConfig(sshKeyFilename, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		fatal(w, jout, "%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	delegationSyncSSHconf, err := kdlib.CreateSSHConfig(sshKeyFilename, env.delegationUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		fatal(w, jout, "Can't create delegation sync ssh config: %s\n", err)
	}

	kdAddrSyncSSHconf, err := kdlib.CreateSSHConfig(sshKeyFilename, env.kdAddrUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		fatal(w, jout, "Can't create keydesk address ssh config: %s\n", err)
	}

	db, err := createDBPool(env.dbURL)
	if err != nil {
		fatal(w, jout, "%s: Can't create db pool: %s\n", LogTag, err)
	}

	freeSlots, err := createBrigade(db, kdAddrSyncSSHconf, delegationSyncSSHconf, env, opts)
	if err != nil {
		fatal(w, jout, "%s: Can't create brigade: %s\n", LogTag, err)
	}

	// wgconfx = chunked (wgconf + keydesk IP)
	wgconf, keydeskIPv6, err := requestBrigade(db, sshconf, &env.delegationCheckEnv, opts, &env.vpnCfgs)
	if err != nil {
		if _, err := setOrphan(db, brigadesSchema, opts.id); err != nil {
			fmt.Fprintf(os.Stderr, "%s: Can't set orphan: %s\n", LogTag, err)
		}

		fatal(w, jout, "%s: Can't request brigade: %s\n", LogTag, err)
	}

	switch jout {
	case true:
		answ := dcmgmt.Answer{
			Answer: keydesk.Answer{
				Code:    http.StatusCreated,
				Desc:    http.StatusText(http.StatusCreated),
				Status:  keydesk.AnswerStatusSuccess,
				Configs: *wgconf,
			},
			KeydeskIPv6: keydeskIPv6,
			FreeSlots:   int(freeSlots),
		}

		payload, err := json.Marshal(answ)
		if err != nil {
			fatal(w, jout, "%s: Can't marshal answer: %s\n", LogTag, err)
		}

		if _, err := w.Write(payload); err != nil {
			fatal(w, jout, "%s: Can't write answer: %s\n", LogTag, err)
		}
	default:
		if _, err = fmt.Fprintln(w, freeSlots); err != nil {
			log.Fatalf("%s: Can't print free slots: %s\n", LogTag, err)
		}

		if _, err = fmt.Fprintln(w, keydeskIPv6.String()); err != nil {
			log.Fatalf("%s: Can't print keydesk ipv6: %s\n", LogTag, err)
		}

		if _, err := fmt.Fprintln(w, *wgconf.WireguardConfig.FileName); err != nil {
			log.Fatalf("%s: Can't print wgconf filename: %s\n", LogTag, err)
		}

		if _, err := fmt.Fprintln(w, *wgconf.WireguardConfig.FileContent); err != nil {
			log.Fatalf("%s: Can't print wgconf content: %s\n", LogTag, err)
		}
	}
}

const fatalString = `{
	"code" : 500,
	"desc" : "Internal Server Error",
	"status" : "error",
	"message" : "%s"
}`

func fatal(w io.Writer, jout bool, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)

	switch jout {
	case true:
		fmt.Fprintf(w, fatalString, msg)
	default:
		fmt.Fprint(w, msg)
	}

	log.Fatal(msg)
}

func createBrigade(
	db *pgxpool.Pool,
	kdAddrSyncSSHconf *ssh.ClientConfig,
	delegationSyncSSHconf *ssh.ClientConfig,
	env *envOpts,
	opts *brigadeOpts,
) (int32, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	// pick up a less used pair

	var (
		pairID           string
		pairEndpointIPv4 netip.Addr
		pairControlIP    netip.Addr
		domainName       pgtype.Text
	)

	sqlPickPair := `
	SELECT
		pair_id,
		control_ip,
		endpoint_ipv4,
		domain_name
	FROM %s
	WHERE
	pair_id = (
			SELECT 
				pair_id 
			FROM %s 
			ORDER BY free_slots_count DESC 
			LIMIT 1
			)
	ORDER BY domain_name NULLS LAST
	LIMIT 1
`
	sqlPickPairForcedIP := `
	SELECT
		pair_id,
		control_ip,
		endpoint_ipv4,
		domain_name
	FROM %s
	WHERE
		control_ip=$1
	ORDER BY domain_name NULLS LAST
	LIMIT 1
`

	switch opts.forceIP {
	case netip.Addr{}:
		err = tx.QueryRow(
			ctx,
			fmt.Sprintf(
				sqlPickPair,
				pgx.Identifier{brigadesSchema, "slots"}.Sanitize(),
				pgx.Identifier{brigadesSchema, "active_pairs"}.Sanitize(),
			),
		).Scan(&pairID, &pairControlIP, &pairEndpointIPv4, &domainName)
	default:
		err = tx.QueryRow(
			ctx,
			fmt.Sprintf(
				sqlPickPairForcedIP,
				pgx.Identifier{brigadesSchema, "slots"}.Sanitize()),
			opts.forceIP.String(),
		).Scan(&pairID, &pairControlIP, &pairEndpointIPv4, &domainName)
	}

	if err != nil {
		return 0, fmt.Errorf("pair query: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s: ep: %s ctrl: %s\n", LogTag, pairEndpointIPv4, pairControlIP)

	if domainName.Valid {
		fmt.Fprintf(os.Stderr, "%s: domain: %s\n", LogTag, domainName.String)
	}

	// pick up cgnat
	cgnatNet, err := dcmgmtlib.RandomCGNAT24Net()
	if err != nil {
		return 0, fmt.Errorf("cgnat: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s:  cgnat_net: %s\n", LogTag, cgnatNet)

	// pick up ula
	ulaNet, err := dcmgmtlib.RandomULA64Net()
	if err != nil {
		return 0, fmt.Errorf("ula: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s: ula_net: %s\n", LogTag, ulaNet)

	// pick up keydesk
	keydesk, err := dcmgmtlib.RandomKeydesk()
	if err != nil {
		return 0, fmt.Errorf("keydesk: %w", err)
	}

	num := int32(0)
	if err := tx.QueryRow(ctx, kdlib.GetFreeSlotsNumberStatement(brigadesSchema, true)).Scan(&num); err != nil {
		return 0, fmt.Errorf("slots query: %w", err)
	}

	// create brigade
	sqlCreateBrigade := `
INSERT INTO %s
		(
			brigade_id,  
			pair_id,    
			brigadier,           
			endpoint_ipv4,
			domain_name,       
			dns_ipv4,            
			dns_ipv6,            
			keydesk_ipv6,        
			ipv4_cgnat,          
			ipv6_ula,            
			person,
			main              
		)
VALUES 
		(
			$1,
			$2,
			$3,
			$4,
			$5,
			$6,
			$7,
			$8,
			$9,
			$10,
			$11,
			true
		)
RETURNING instance_id;
`

	var instanceID uuid.UUID

	if err = tx.QueryRow(ctx,
		fmt.Sprintf(sqlCreateBrigade, pgx.Identifier{brigadesSchema, "brigades"}.Sanitize()),
		opts.id,
		pairID,
		opts.name,
		pairEndpointIPv4.String(),
		zeronull.Text(domainName.String),
		cgnatNet.Addr().String(),
		ulaNet.Addr().String(),
		keydesk.String(),
		cgnatNet.String(),
		ulaNet.String(),
		opts.person,
	).Scan(&instanceID); err != nil {
		return 0, fmt.Errorf("create brigade: %w", err)
	}

	sqlInsertStats := `INSERT INTO %s (brigade_id, instance_id) VALUES ($1,$2);`

	if _, err = tx.Exec(ctx,
		fmt.Sprintf(sqlInsertStats, (pgx.Identifier{brigadesStatsSchema, "brigades_stats"}.Sanitize())),
		opts.id, instanceID,
	); err != nil {
		return 0, fmt.Errorf("create stats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	if env.delegationMutex == "" {
		env.delegationMutex = DefaultDelegationMutexPath
	}

	dmu := lockedfile.MutexAt(env.delegationMutex)

	unlock, err := dmu.Lock()
	if err != nil {
		return 0, fmt.Errorf("lock delegation mutex: %w", err)
	}

	defer unlock()

	// Pick up subdomain.

	if !domainName.Valid {
		if err := dcmgmtlib.ApplySubdomain(ctx, db, env.subdomainAPIHost, env.subdomainAPIToken, opts.id, pairEndpointIPv4, env.Zone); err != nil {
			return 0, fmt.Errorf("apply subdomain: %w", err)
		}
	}

	// Sync delegation list.

	delegationList, err := dcmgmtlib.NewDelegationList(ctx, db, brigadesSchema)
	if err != nil {
		return 0, fmt.Errorf("delegation list: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s: %s@%s\n", LogTag, delegationSyncSSHconf.User, env.delegationServer)
	cleanup, err := dcmgmtlib.SyncDelegationList(delegationSyncSSHconf, env.delegationServer, env.ident, delegationList)
	cleanup(LogTag)

	if err != nil {
		return 0, fmt.Errorf("delegation sync: %w", err)
	}

	// Sync keydesk address list

	fmt.Fprintf(os.Stderr, "%s: keydesk: %s\n", LogTag, keydesk)

	kdAddrList, err := dcmgmtlib.NewKdAddrList(ctx, db, brigadesSchema)
	if err != nil {
		return 0, fmt.Errorf("keydesk addr list: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s: %s@%s\n", LogTag, kdAddrSyncSSHconf.User, env.kdAddrServer)
	cleanup, err = dcmgmtlib.SyncKdAddrList(kdAddrSyncSSHconf, env.kdAddrServer, env.ident, kdAddrList)
	cleanup(LogTag)

	if err != nil {
		return 0, fmt.Errorf("keydesk address sync: %w", err)
	}

	return num - 1, nil
}

func requestBrigade(
	db *pgxpool.Pool,
	sshconf *ssh.ClientConfig,
	dlgenv *delegationCheckEnv,
	opts *brigadeOpts,
	vpnCfgs *vpnCfgs,
) (*models.Newuser, netip.Addr, error) {
	var (
		brigadeID    []byte
		fullname     string
		endpointIPv4 netip.Addr
		domainName   pgtype.Text
		dnsIPv4      netip.Addr
		dnsIPv6      netip.Addr
		keydeskIPv6  netip.Addr
		ipv4CGNAT    netip.Prefix
		ipv6ULA      netip.Prefix
		pjson        []byte
		control_ip   netip.Addr
		nameservers  pgtype.Text
	)

	sqlFetchBrigade := `
SELECT
	meta_brigades.brigade_id,
	meta_brigades.brigadier,
	meta_brigades.endpoint_ipv4,
	meta_brigades.domain_name,
	meta_brigades.dns_ipv4,
	meta_brigades.dns_ipv6,
	meta_brigades.keydesk_ipv6,
	meta_brigades.ipv4_cgnat,
	meta_brigades.ipv6_ula,
	meta_brigades.person,
	meta_brigades.control_ip,
	meta_brigades.nameservers
FROM %s
WHERE
	meta_brigades.brigade_id=$1
`

	if err := func() error {
		ctx := context.Background()

		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}

		defer tx.Rollback(ctx)
		err = tx.QueryRow(ctx,
			fmt.Sprintf(sqlFetchBrigade,
				(pgx.Identifier{brigadesSchema, "meta_brigades"}.Sanitize()),
			),
			opts.id,
		).Scan(
			&brigadeID,
			&fullname,
			&endpointIPv4,
			&domainName,
			&dnsIPv4,
			&dnsIPv6,
			&keydeskIPv6,
			&ipv4CGNAT,
			&ipv6ULA,
			&pjson,
			&control_ip,
			&nameservers,
		)
		if err != nil {
			return err
		}

		return nil
	}(); err != nil {
		return nil, netip.Addr{}, fmt.Errorf("brigade query: %w", err)
	}

	person := &namesgenerator.Person{}
	if err := json.Unmarshal(pjson, &person); err != nil {
		return nil, netip.Addr{}, fmt.Errorf("person: %w", err)
	}

	// cmd := fmt.Sprintf("create -id %s -ep4 %s -int4 %s -int6 %s -dns4 %s -dns6 %s -kd6 %s -name %s -person %s -desc %s -url %s -dn %s -ch -j",
	cmd := fmt.Sprintf("create -id %s -ep4 %s -int4 %s -int6 %s -dns4 %s -dns6 %s -kd6 %s -name %s -person %s -desc %s -url %s -dn %s -j",
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(brigadeID),
		endpointIPv4,
		ipv4CGNAT,
		ipv6ULA,
		dnsIPv4,
		dnsIPv6,
		keydeskIPv6,
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(fullname)),
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.Name)),
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.Desc)),
		base64.StdEncoding.WithPadding(base64.StdPadding).EncodeToString([]byte(person.URL)),
		domainName.String,
	)

	if vpnCfgs != nil {
		if vpnCfgs.wg != "" {
			cmd += fmt.Sprintf(" -wg %s", vpnCfgs.wg)
		}

		if vpnCfgs.ovc != "" {
			cmd += fmt.Sprintf(" -ovc %s", vpnCfgs.ovc)
		}

		if vpnCfgs.ipsec != "" {
			cmd += fmt.Sprintf(" -ipsec %s", vpnCfgs.ipsec)
		}

		if vpnCfgs.outline != "" {
			cmd += fmt.Sprintf(" -outline %s", vpnCfgs.outline)
		}

		if vpnCfgs.proto0 != "" {
			cmd += fmt.Sprintf(" -proto0 %s", vpnCfgs.proto0)
		}
	}

	fmt.Fprintf(os.Stderr, "%s: %s#%s:22 -> %s\n", LogTag, sshkeyRemoteUsername, control_ip, cmd)

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", control_ip), sshconf)
	if err != nil {
		return nil, netip.Addr{}, fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, netip.Addr{}, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var b, e bytes.Buffer

	session.Stdout = &b
	session.Stderr = &e

	defer func() {
		switch errstr := e.String(); errstr {
		case "":
			fmt.Fprintf(os.Stderr, "%s: SSH Session StdErr: empty\n", LogTag)
		default:
			fmt.Fprintf(os.Stderr, "%s: SSH Session StdErr:\n", LogTag)
			for _, line := range strings.Split(errstr, "\n") {
				fmt.Fprintf(os.Stderr, "%s: | %s\n", LogTag, line)
			}
		}
	}()

	if err := session.Run(cmd); err != nil {
		return nil, netip.Addr{}, fmt.Errorf("ssh run: %w", err)
	}

	// payload, err := io.ReadAll(httputil.NewChunkedReader(&b))
	payload, err := io.ReadAll(&b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: Chunk read: %s\n", LogTag, payload)

		return nil, netip.Addr{}, fmt.Errorf("chunk read: %w", err)
	}

	nss := strings.Split(nameservers.String, ",")
	if len(nss) == 0 {
		nss = dlgenv.domainNS
	}

	fmt.Fprintf(os.Stderr, "%s: Waiting for delegation: %s, %s -> %s\n", LogTag, keydeskIPv6.String(), domainName.String, endpointIPv4)
	if !waitForAllDelegations(
		dlgenv.kdDomain,
		keydeskIPv6,
		dlgenv.kdNS,
		domainName.String,
		endpointIPv4,
		nss,
	) {
		return nil, netip.Addr{}, fmt.Errorf("delegation: %w", ErrNotDelegated)
	}

	wgconf := &keydesk.Answer{}
	if err := json.Unmarshal(payload, &wgconf); err != nil {
		return nil, netip.Addr{}, fmt.Errorf("json unmarshal: %w", err)
	}

	return &wgconf.Configs, keydeskIPv6, nil
}

func waitForAllDelegations(
	keydeskZone string, keydeskAddr netip.Addr, keydeskNS []string,
	domain string, endpointIPv4 netip.Addr, domainNS []string,
) bool {
	var kdOk, domainOk bool

	wg := &sync.WaitGroup{}

	wg.Add(1)
	go func() {
		defer wg.Done()

		ipstr := keydeskAddr.String()
		domain := strings.ReplaceAll(strings.Replace(ipstr, (ipstr)[:2], "w", 1), ":", "s") + "." + keydeskZone
		ok, err := dcmgmtlib.WaitForDelegation(domain, keydeskAddr, keydeskNS...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: Keydesk delegation: %s: %s\n", LogTag, keydeskAddr, err)
		}

		kdOk = ok
	}()

	if domain != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ok, err := dcmgmtlib.WaitForDelegation(domain, endpointIPv4, domainNS...)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: Domain delegation: %s: %s: %s\n", LogTag, domain, endpointIPv4, err)
			}

			domainOk = ok
		}()
	}

	wg.Wait()

	return kdOk && domainOk
}

func createDBPool(dburl string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dburl)
	if err != nil {
		return nil, fmt.Errorf("conn string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return pool, nil
}

func parseArgs() (bool, bool, *brigadeOpts, error) {
	brigadeID := flag.String("id", "", "brigadier_id")
	brigadierName := flag.String("name", "", "brigadierName :: base64")
	personName := flag.String("person", "", "personName :: base64")
	personDesc := flag.String("desc", "", "personDesc :: base64")
	personURL := flag.String("url", "", "personURL :: base64")
	chunked := flag.Bool("ch", false, "chunked output")
	nodeIP := flag.String("ip", "", "control IP for debug")
	jout := flag.Bool("j", false, "json output")

	flag.Parse()

	opts := &brigadeOpts{}

	// brigadeID must be base32 decodable.
	buf, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(*brigadeID)
	if err != nil {
		return false, false, nil, fmt.Errorf("id base32: %s: %w", *brigadeID, err)
	}

	id, err := uuid.FromBytes(buf)
	if err != nil {
		return false, false, nil, fmt.Errorf("id uuid: %s: %w", *brigadeID, err)
	}

	opts.id = id.String()

	// brigadierName must be not empty and must be a valid UTF8 string
	if *brigadierName == "" {
		return false, false, nil, ErrEmptyBrigadierName
	}

	buf, err = base64.StdEncoding.DecodeString(*brigadierName)
	if err != nil {
		return false, false, nil, fmt.Errorf("brigadier name: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, false, nil, ErrInvalidBrigadierName
	}

	opts.name = string(buf)

	// personName must be not empty and must be a valid UTF8 string
	if *personName == "" {
		return false, false, nil, ErrEmptyPersonName
	}

	buf, err = base64.StdEncoding.DecodeString(*personName)
	if err != nil {
		return false, false, nil, fmt.Errorf("person name: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, false, nil, ErrInvalidPersonName
	}

	opts.person.Name = string(buf)

	// personDesc must be not empty and must be a valid UTF8 string
	if *personDesc == "" {
		return false, false, nil, ErrEmptyPersonDesc
	}

	buf, err = base64.StdEncoding.DecodeString(*personDesc)
	if err != nil {
		return false, false, nil, fmt.Errorf("person desc: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, false, nil, ErrInvalidPersonDesc
	}

	opts.person.Desc = string(buf)

	// personURL must be not empty and must be a valid UTF8 string
	if *personURL == "" {
		return false, false, nil, ErrEmptyPersonURL
	}

	buf, err = base64.StdEncoding.DecodeString(*personURL)
	if err != nil {
		return false, false, nil, fmt.Errorf("person url: %w", err)
	}

	if !utf8.Valid(buf) {
		return false, false, nil, ErrInvalidPersonURL
	}

	u := string(buf)

	_, err = url.Parse(u)
	if err != nil {
		return false, false, nil, fmt.Errorf("parse person url: %w", err)
	}

	opts.person.URL = u

	if *nodeIP != "" {
		opts.forceIP, _ = netip.ParseAddr(*nodeIP)
	}

	return *chunked, *jout, opts, nil
}

func readConfigs() (string, *envOpts, error) {
	env := &envOpts{}

	env.dbURL = os.Getenv("DB_URL")
	if env.dbURL == "" {
		env.dbURL = defaultDatabaseURL
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshkeyDefaultPath)
	if err != nil {
		return "", nil, fmt.Errorf("lookup for ssh key: %w", err)
	}

	env.subdomainAPIHost = os.Getenv("SUBDOMAIN_API_SERVER")
	if env.subdomainAPIHost == "" {
		return "", nil, errors.New("empty subdomapi host")
	}

	if _, err := netip.ParseAddrPort(env.subdomainAPIHost); err != nil {
		return "", nil, fmt.Errorf("parse subdomapi host: %w", err)
	}

	env.subdomainAPIToken = os.Getenv("SUBDOMAIN_API_TOKEN")
	if env.subdomainAPIToken == "" {
		return "", nil, errors.New("empty subdomapi token")
	}

	_, env.ident, err = dcmgmtlib.ParseDCNameEnv()
	if err != nil {
		return "", nil, fmt.Errorf("dc name: %w", err)
	}

	env.delegationUser, env.delegationServer, err = dcmgmtlib.ParseConnEnv("DELEGATION_SYNC_CONNECT")
	if err != nil {
		return "", nil, fmt.Errorf("delegation sync connect: %w", err)
	}

	env.kdAddrUser, env.kdAddrServer, err = dcmgmtlib.ParseConnEnv("KEYDESK_ADDRESS_SYNC_CONNECT")
	if err != nil {
		return "", nil, fmt.Errorf("keydesk address sync connect: %w", err)
	}

	env.kdDomain = os.Getenv("KEYDESK_DOMAIN")
	if env.kdDomain == "" {
		return "", nil, errors.New("empty keydesk domain")
	}

	kdNameServers := os.Getenv("KEYDESK_NAMESERVERS")
	if kdNameServers == "" {
		return "", nil, errors.New("empty keydesk nameservers")
	}

	env.kdNS = strings.Split(kdNameServers, ",")

	domainNameServers := os.Getenv("DOMAIN_NAMESERVERS")
	if domainNameServers == "" {
		return "", nil, errors.New("empty domain nameservers")
	}

	env.delegationMutex = os.Getenv("DELEGATION_MUTEX")
	if env.delegationMutex == "" {
		env.delegationMutex = DefaultDelegationMutexPath
	}

	env.domainNS = strings.Split(domainNameServers, ",")

	env.vpnCfgs.wg = os.Getenv("WIREGUARD_CONFIGS")
	if env.vpnCfgs.wg == "" {
		env.vpnCfgs.wg = defaultWireguardConfigs
	}

	env.vpnCfgs.ovc = os.Getenv("OVC_CONFIGS")
	env.vpnCfgs.ipsec = os.Getenv("IPSEC_CONFIGS")
	env.vpnCfgs.outline = os.Getenv("OUTLINE_CONFIGS")
	env.vpnCfgs.proto0 = os.Getenv("PROTO0_CONFIGS")

	env.Zone = os.Getenv("SERVICE_ZONE")
	if env.Zone == "" {
		env.Zone = DefaultServiceZone
	}

	return sshKeyFilename, env, nil
}

func setOrphan(
	db *pgxpool.Pool,
	schema string,
	brigadeID string,
) (int32, error) {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	sqlSetOrphan := `INSERT INTO %s (endpoint_ipv4) SELECT endpoint_ipv4 FROM %s WHERE brigade_id=$1`
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		sqlSetOrphan,
		pgx.Identifier{schema, "orphaned_endpoints_ipv4"}.Sanitize(),
		pgx.Identifier{schema, "brigades"}.Sanitize(),
	), brigadeID); err != nil {
		return 0, fmt.Errorf("orphan insert: %w", err)
	}

	sqlDelBrigadesStats := `
	DELETE
		FROM %s
	WHERE brigade_id=$1
	`

	if _, err := tx.Exec(ctx, fmt.Sprintf(sqlDelBrigadesStats, pgx.Identifier{brigadesStatsSchema, "brigades_stats"}.Sanitize()), brigadeID); err != nil {
		return 0, fmt.Errorf("brigades stats delete: %w", err)
	}

	var domain_name pgtype.Text

	sqlDelBrigade := `
	DELETE 
		FROM %s
	WHERE brigade_id=$1
	RETURNING domain_name
	`

	if err := tx.QueryRow(
		ctx,
		fmt.Sprintf(sqlDelBrigade, pgx.Identifier{schema, "brigades"}.Sanitize()),
		brigadeID,
	).Scan(&domain_name); err != nil {
		return 0, fmt.Errorf("brigade delete: %w", err)
	}

	num := int32(0)

	if err := tx.QueryRow(ctx, kdlib.GetFreeSlotsNumberStatement(schema, true)).Scan(&num); err != nil {
		return 0, fmt.Errorf("free slots query: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return num, nil
}
