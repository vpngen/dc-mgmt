package main

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http/httputil"
	"net/netip"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype/zeronull"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vpngen/keydesk/keydesk/storage"
	"golang.org/x/crypto/ssh"
)

const (
	defaultPairsSchema         = "pairs"
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
)

const (
	sshkeyFilename       = "id_ecdsa"
	sshkeyRemoteUsername = "_marina_"
	etcDefaultPath       = "/etc/vg-dc-mgmt"
	fileTempSuffix       = ".tmp"
	defautStoreSubdir    = "vg-collectstats"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	ParallelCollectorsLimit = 16
	sshTimeOut              = time.Duration(5 * time.Second)
)

const (
	sqlGetBrigadesGroups = `
SELECT
	p.control_ip,
	ARRAY_AGG(b.brigade_id) AS brigade_group
FROM
	%s AS p
LEFT JOIN
	%s AS b ON p.pair_id = b.pair_id
GROUP BY
	p.pair_id
HAVING
	COUNT(b.brigade_id) > 0;
`

	sqlInsertStats = `
INSERT INTO %s (
	brigade_id,
	first_visit,
	total_users_count,
	throttled_users_count,
	active_users_count,
	active_wg_users_count,
	active_ipsec_users_count,
	total_traffic_rx,
	total_traffic_tx,
	total_wg_traffic_rx,
	total_wg_traffic_tx,
	total_ipsec_traffic_rx,
	total_ipsec_traffic_tx,
	counters_update_time,
	update_time
) VALUES (
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
	$12,
	$13,
	$14,
	$15
) ON CONFLICT (brigade_id,align_time) DO UPDATE SET
	first_visit = $2,
	total_users_count = $3,
	throttled_users_count = $4,
	active_users_count = $5,
	active_wg_users_count = $6,
	active_ipsec_users_count = $7,
	total_traffic_rx = $8,
	total_traffic_tx = $9,
	total_wg_traffic_rx = $10,
	total_wg_traffic_tx = $11,
	total_ipsec_traffic_rx = $12,
	total_ipsec_traffic_tx = $13,
	counters_update_time = $14,
	update_time = $15
;
`
)

// BrigadeGroup - brigades in the same pair.
type BrigadeGroup struct {
	ConnectAddr netip.Addr
	Brigades    [][]byte
}

// GroupsList - list of brigades groups.
type GroupsList []BrigadeGroup

// AggrStats - structure for aggregated stats.
type AggrStats struct {
	Ver   int              `json:"version"`
	Stats []*storage.Stats `json:"stats"`
}

// AggrStatsX - structure for aggregated stats with additional fields.
type AggrStatsX struct {
	Ver        int              `json:"version"`
	UpdateTime time.Time        `json:"update_time"`
	Stats      []*storage.Stats `json:"stats"`
}

func main() {
	executable, _ := os.Executable()
	exe := filepath.Base(executable)

	sshKeyDir, dbname, pairsSchema, brigadesSchema, statsSchema, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", exe, err)
	}

	storePath, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", exe, err)
	}

	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		if err := os.MkdirAll(storePath, 0o755); err != nil {
			log.Fatalf("%s: Can't create store path: %s\n", exe, err)
		}
	}

	sshconf, err := createSSHConfig(sshKeyDir)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", exe, err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", exe, err)
	}

	statsFileName := time.Now().UTC().Format("stats-20060102-150405.json")

	if err := pairsWalk(db, sshconf, pairsSchema, brigadesSchema, statsSchema, filepath.Join(storePath, statsFileName)); err != nil {
		log.Fatalf("%s: Can't collect stats: %s\n", exe, err)
	}
}

// collectStats - collect stats from the pair.
func collectStats(sshconf *ssh.ClientConfig, addr netip.Addr, brigades [][]byte, stream chan<- *AggrStats, sem <-chan struct{}, wg *sync.WaitGroup) {
	defer func() {
		<-sem // Release the semaphore
	}()

	defer wg.Done()

	ids := make([]string, 0, len(brigades))
	for _, id := range brigades {
		ids = append(ids, base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(id[:]))
	}

	groupStats, err := fetchStatsBySSH(sshconf, addr, ids)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fetch stats: %s\n", err)

		return
	}

	// fmt.Fprintf(os.Stderr, "fetch stats: %s\n", groupStats)

	var parsedStats AggrStats
	if err := json.Unmarshal(groupStats, &parsedStats); err != nil {
		fmt.Fprintf(os.Stderr, "unmarshal stats: %s\n", err)

		return
	}

	stream <- &parsedStats

	/*for _, stats := range parsedStats.Stats {
		if err := insertStats(db, statsSchema, stats); err != nil {
			fmt.Fprintf(os.Stderr, "insert stats: %s\n", err)

			return
		}
	}*/
}

// updateStats - update stats in the database.
func updateStats(db *pgxpool.Pool, statsSchema string, stats *storage.Stats) error {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	brigadeID, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(stats.BrigadeID)
	if err != nil {
		return fmt.Errorf("decode brigade id: %w", err)
	}

	sqlUpdateStats := `
	INSERT INTO %s (
		brigade_id, 
		created_at,
		first_visit,
		total_users_count,
		throttled_users_count,
		active_users_count,
		active_wg_users_count,
		active_ipsec_users_count,
		total_traffic_rx,
		total_traffic_tx,
		total_wg_traffic_rx,
		total_wg_traffic_tx,
		total_ipsec_traffic_rx,
		total_ipsec_traffic_tx,
		update_time
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	ON CONFLICT (brigade_id) DO UPDATE
	SET 
		first_visit=$3,
		total_users_count=$4,
		throttled_users_count=$5,
		active_users_count=$6,
		active_wg_users_count=$7,
		active_ipsec_users_count=$8,
		total_traffic_rx=$9,
		total_traffic_tx=$10,
		total_wg_traffic_rx=$11,
		total_wg_traffic_tx=$12,
		total_ipsec_traffic_rx=$13,
		total_ipsec_traffic_tx=$14,
		update_time=$15
	`

	_, err = tx.Exec(
		ctx,
		fmt.Sprintf(sqlUpdateStats, pgx.Identifier{statsSchema, "brigades_stats"}.Sanitize()),
		brigadeID,
		stats.BrigadeCreatedAt,
		zeronull.Timestamp(stats.KeydeskFirstVisit),
		stats.TotalUsersCount,
		stats.ThrottledUsersCount,
		stats.ActiveUsersCount,
		stats.ActiveWgUsersCount,
		stats.ActiveIPSecUsersCount,
		stats.TotalTraffic.Rx,
		stats.TotalTraffic.Tx,
		stats.TotalWgTraffic.Rx,
		stats.TotalWgTraffic.Tx,
		stats.TotalIPSecTraffic.Rx,
		stats.TotalIPSecTraffic.Tx,
		stats.UpdateTime,
	)
	if err != nil {
		return fmt.Errorf("update stats: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// handleStatsStream - handle stats stream and update stats in the database and write to the file.
func handleStatsStream(db *pgxpool.Pool, statsSchema string, filename string, stream <-chan *AggrStats, wg *sync.WaitGroup) {
	defer wg.Done()

	aggrStats := &AggrStatsX{
		UpdateTime: time.Now().UTC(),
		Stats:      make([]*storage.Stats, 0),
	}

	for stats := range stream {
		for _, s := range stats.Stats {
			aggrStats.Stats = append(aggrStats.Stats, s)

			if err := updateStats(db, statsSchema, s); err != nil {
				fmt.Fprintf(os.Stderr, "update stats: %s\n", err)
			}
		}
	}

	f, err := os.Create(filename + fileTempSuffix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "create stats file: %s\n", err)

		return
	}

	defer f.Close()

	if err := json.NewEncoder(f).Encode(aggrStats); err != nil {
		fmt.Fprintf(os.Stderr, "encode stats: %s\n", err)

		return
	}

	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		if err := os.Remove(filename); err != nil {
			fmt.Fprintf(os.Stderr, "remove stats file: %s\n", err)

			return
		}
	}

	if err := os.Link(filename+fileTempSuffix, filename); err != nil {
		fmt.Fprintf(os.Stderr, "link stats file: %s\n", err)

		return
	}
}

// pairsWalk - walk through pairs and collect stats.
func pairsWalk(db *pgxpool.Pool, sshconf *ssh.ClientConfig, pairsSchema, brigadesSchema, statsSchema, statsfile string) error {
	groups, err := getBrigadesGroups(db, pairsSchema, brigadesSchema)
	if err != nil {
		return fmt.Errorf("get brigades groups: %w", err)
	}

	sem := make(chan struct{}, ParallelCollectorsLimit) // Semaphore for limiting parallel collectors.
	var wgg sync.WaitGroup

	stream := make(chan *AggrStats, ParallelCollectorsLimit)
	var wgh sync.WaitGroup

	wgh.Add(1)
	go handleStatsStream(db, statsSchema, statsfile, stream, &wgh)

	for _, group := range groups {
		sem <- struct{}{} // Acquire the semaphore
		wgg.Add(1)

		collectStats(sshconf, group.ConnectAddr, group.Brigades, stream, sem, &wgg)
	}

	wgg.Wait() // Wait for all goroutines to finish

	close(stream)

	wgh.Wait() // Wait for all goroutines to finish

	return nil
}

// insertStats - inserts stats into database.
func insertStats(db *pgxpool.Pool, schema string, stats *storage.Stats) error {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	brigadeID, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(stats.BrigadeID)
	if err != nil {
		return fmt.Errorf("decode brigade id: %w", err)
	}

	_, err = tx.Exec(ctx,
		fmt.Sprintf(sqlInsertStats, (pgx.Identifier{schema, "brigades_statistics"}.Sanitize())),
		brigadeID,
		stats.KeydeskFirstVisit,
		stats.TotalUsersCount,
		stats.ThrottledUsersCount,
		stats.ActiveUsersCount,
		stats.ActiveWgUsersCount,
		stats.ActiveIPSecUsersCount,
		stats.TotalTraffic.Rx,
		stats.TotalTraffic.Tx,
		stats.TotalWgTraffic.Rx,
		stats.TotalWgTraffic.Tx,
		stats.TotalIPSecTraffic.Rx,
		stats.TotalIPSecTraffic.Tx,
		stats.CountersUpdateTime,
		stats.UpdateTime,
	)
	if err != nil {
		tx.Rollback(ctx)

		return fmt.Errorf("create stats: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

// fetchStatsBySSH - fetch brigades stats from remote host by ssh.
func fetchStatsBySSH(sshconf *ssh.ClientConfig, addr netip.Addr, ids []string) ([]byte, error) {
	cmd := fmt.Sprintf("fetchstats -b %s -ch", strings.Join(ids, ","))

	fmt.Fprintf(os.Stderr, "%s#%s:22 -> %s\n", sshkeyRemoteUsername, addr, cmd)

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", addr), sshconf)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	var b, e bytes.Buffer

	session.Stdout = &b
	session.Stderr = &e

	if err := session.Run(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "session errors:\n%s\n", e.String())

		return nil, fmt.Errorf("ssh run: %w", err)
	}

	groupStats, err := io.ReadAll(httputil.NewChunkedReader(&b))
	if err != nil {
		fmt.Fprintf(os.Stderr, "readed data:\n%s\n", groupStats)

		return nil, fmt.Errorf("chunk read: %w", err)
	}

	return groupStats, nil
}

// getBrigadesGroups - returns brigades lists on per pair basis.
func getBrigadesGroups(db *pgxpool.Pool, schema_pairs, schema_stats string) (GroupsList, error) {
	var list GroupsList

	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}

	rows, err := tx.Query(ctx,
		fmt.Sprintf(sqlGetBrigadesGroups,
			(pgx.Identifier{schema_pairs, "pairs"}.Sanitize()),
			(pgx.Identifier{schema_stats, "brigades"}.Sanitize()),
		),
	)
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigades groups: %w", err)
	}

	var group BrigadeGroup

	_, err = pgx.ForEachRow(rows, []any{&group.ConnectAddr, &group.Brigades}, func() error {
		list = append(list, group)

		return nil
	})
	if err != nil {
		tx.Rollback(ctx)

		return nil, fmt.Errorf("brigade group row: %w", err)
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return list, nil
}

// createDBPool - creates database connection pool.
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

func parseArgs() (string, error) {
	store := flag.String("p", "", "directory to store the data")
	flag.Parse()

	if *store == "" {
		sysUser, err := user.Current()
		if err != nil {
			return "", fmt.Errorf("user: %w", err)
		}

		return filepath.Join(sysUser.HomeDir, defautStoreSubdir), nil
	}

	return *store, nil
}

// readConfigs - reads configs from environment variables.
func readConfigs() (string, string, string, string, string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	pairsSchema := os.Getenv("PAIRS_SCHEMA")
	if pairsSchema == "" {
		pairsSchema = defaultPairsSchema
	}

	brigadesSchema := os.Getenv("BRIGADES_SCHEMA")
	if brigadesSchema == "" {
		brigadesSchema = defaultBrigadesSchema
	}

	brigadesStatsSchema := os.Getenv("BRIGADES_STATS_SCHEMA")
	if brigadesStatsSchema == "" {
		brigadesStatsSchema = defaultBrigadesStatsSchema
	}

	sshKeyDir := os.Getenv("CONFDIR")
	if sshKeyDir == "" {
		sysUser, err := user.Current()
		if err != nil {
			return "", "", "", "", "", fmt.Errorf("user: %w", err)
		}

		sshKeyDir = filepath.Join(sysUser.HomeDir, ".ssh")
	}

	if fstat, err := os.Stat(sshKeyDir); err != nil || !fstat.IsDir() {
		sshKeyDir = etcDefaultPath
	}

	return sshKeyDir, dbURL, pairsSchema, brigadesSchema, brigadesStatsSchema, nil
}

// createSSHConfig - creates ssh client config.
func createSSHConfig(path string) (*ssh.ClientConfig, error) {
	// var hostKey ssh.PublicKey

	key, err := os.ReadFile(filepath.Join(path, sshkeyFilename))
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: sshkeyRemoteUsername,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		// HostKeyCallback: ssh.FixedHostKey(hostKey),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshTimeOut,
	}

	return config, nil
}
