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
	"github.com/vpngen/realm-admin/internal/kdlib"
	"golang.org/x/crypto/ssh"
)

const (
	defaultPairsSchema         = "pairs"
	defaultBrigadesSchema      = "brigades"
	defaultBrigadesStatsSchema = "stats"
	defaultDCName              = "unknown"
)

const (
	sshKeyDefaultPath    = "/etc/vg-dc-stats"
	sshkeyRemoteUsername = "_marina_"
	fileTempSuffix       = ".tmp"
	defautStoreSubdir    = "vg-collectstats"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	ParallelCollectorsLimit = 16
	sshTimeOut              = time.Duration(15 * time.Second)
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

const DataCenterStatsVersion = 1

// DataCenterStats - structure for data center stats.
type DataCenterStats struct {
	Version              int `json:"version"`
	TotalFreeSlotsCount  int `json:"total_free_slots_count"`
	ActiveFreeSlotsCount int `json:"active_free_slots_count"`
	TotalPairsCount      int `json:"total_pairs_count"`
	ActivePairsCount     int `json:"active_pairs_count"`
}

// AggrStatsVersion - current version of aggregated stats.
const AggrStatsXVersion = 2

// AggrStatsX - structure for aggregated stats with additional fields.
type AggrStatsX struct {
	Version         int              `json:"version"`
	UpdateTime      time.Time        `json:"update_time"`
	Stats           []*storage.Stats `json:"stats"`
	DataCenterStats `json:"data_center_stats"`
}

var LogTag = setLogTag()

const defaultLogTag = "collectstats"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	sshKeyFilename, dbname, pairsSchema, brigadesSchema, statsSchema, dcName, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	storePath, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		if err := os.MkdirAll(storePath, 0o755); err != nil {
			log.Fatalf("%s: Can't create store path: %s\n", LogTag, err)
		}
	}

	sshconf, err := kdlib.CreateSSHConfig(sshKeyFilename, sshkeyRemoteUsername, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("%s: Can't create ssh configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	dateSuffix := time.Now().UTC().Format("20060102-150405")
	statsFileName := fmt.Sprintf("stats-%s-%s.json", dcName, dateSuffix)

	if err := pairsWalk(db, sshconf, pairsSchema, brigadesSchema, statsSchema, filepath.Join(storePath, statsFileName)); err != nil {
		log.Fatalf("%s: Can't collect stats: %s\n", LogTag, err)
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
		fmt.Fprintf(os.Stderr, "%s: fetch stats: %s\n", LogTag, err)

		return
	}

	// fmt.Fprintf(os.Stderr, "fetch stats: %s\n", groupStats)

	var parsedStats AggrStats
	if err := json.Unmarshal(groupStats, &parsedStats); err != nil {
		fmt.Fprintf(os.Stderr, "%s: unmarshal stats: %s\n", LogTag, err)

		return
	}

	stream <- &parsedStats
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
func handleStatsStream(db *pgxpool.Pool, statsSchema string, filename string, stream <-chan *AggrStats, wg *sync.WaitGroup, dataCenterStats DataCenterStats) {
	defer wg.Done()

	aggrStats := &AggrStatsX{
		Version:         AggrStatsXVersion,
		UpdateTime:      time.Now().UTC(),
		Stats:           make([]*storage.Stats, 0),
		DataCenterStats: dataCenterStats,
	}

	for stats := range stream {
		for _, s := range stats.Stats {
			aggrStats.Stats = append(aggrStats.Stats, s)

			if err := updateStats(db, statsSchema, s); err != nil {
				fmt.Fprintf(os.Stderr, "%s: update stats: %s\n", LogTag, err)
			}
		}
	}

	f, err := os.Create(filename + fileTempSuffix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: create stats file: %s\n", LogTag, err)

		return
	}

	defer f.Close()

	if err := json.NewEncoder(f).Encode(aggrStats); err != nil {
		fmt.Fprintf(os.Stderr, "%s: encode stats: %s\n", LogTag, err)

		return
	}

	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		if err := os.Remove(filename); err != nil {
			fmt.Fprintf(os.Stderr, "%s: remove stats file: %s\n", LogTag, err)

			return
		}
	}

	if err := os.Link(filename+fileTempSuffix, filename); err != nil {
		fmt.Fprintf(os.Stderr, "%s: link stats file: %s\n", LogTag, err)

		return
	}

	if _, err := os.Stat(filename + fileTempSuffix); !os.IsNotExist(err) {
		if err := os.Remove(filename + fileTempSuffix); err != nil {
			fmt.Fprintf(os.Stderr, "%s: remove temp stats file: %s\n", LogTag, err)

			return
		}
	}
}

func getDataCenterStats(db *pgxpool.Pool, pairsSchema, brigadesSchema string) DataCenterStats {
	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "begin transaction: %s\n", err)

		return DataCenterStats{}
	}

	defer tx.Rollback(ctx)

	var TotalPairsCount int

	sqlGetTotalPairsCont := `SELECT count(*) FROM %s`
	if err := tx.QueryRow(
		ctx,
		fmt.Sprintf(sqlGetTotalPairsCont, pgx.Identifier{pairsSchema, "pairs"}.Sanitize()),
	).Scan(&TotalPairsCount); err != nil && err != pgx.ErrNoRows {
		fmt.Fprintf(os.Stderr, "get total pairs count: %s\n", err)

		return DataCenterStats{}
	}

	var ActivePairsCount int
	sqlGetActivePairsCount := `SELECT count(*) FROM %s WHERE is_active = true`
	if err := tx.QueryRow(
		ctx,
		fmt.Sprintf(sqlGetActivePairsCount, pgx.Identifier{pairsSchema, "pairs"}.Sanitize()),
	).Scan(&ActivePairsCount); err != nil && err != pgx.ErrNoRows {
		fmt.Fprintf(os.Stderr, "get active pairs count: %s\n", err)

		return DataCenterStats{}
	}

	var TotalFreeSlotsCount int
	if err := tx.QueryRow(
		ctx,
		kdlib.GetFreeSlotsNumberStatement(brigadesSchema, false),
	).Scan(&TotalFreeSlotsCount); err != nil && err != pgx.ErrNoRows {
		fmt.Fprintf(os.Stderr, "get total free slots count: %s\n", err)

		return DataCenterStats{}
	}

	var ActiveFreeSlotsCount int
	if err := tx.QueryRow(
		ctx,
		kdlib.GetFreeSlotsNumberStatement(brigadesSchema, true),
	).Scan(&ActiveFreeSlotsCount); err != nil && err != pgx.ErrNoRows {
		fmt.Fprintf(os.Stderr, "get active free slots count: %s\n", err)

		return DataCenterStats{}
	}

	fmt.Fprintf(os.Stdout, "DataCenterStats: pairs: %d(%d), slots: %d (%d)\n",
		TotalPairsCount, ActivePairsCount, TotalFreeSlotsCount, ActiveFreeSlotsCount)

	return DataCenterStats{
		Version:              DataCenterStatsVersion,
		TotalPairsCount:      TotalPairsCount,
		ActivePairsCount:     ActivePairsCount,
		TotalFreeSlotsCount:  TotalFreeSlotsCount,
		ActiveFreeSlotsCount: ActiveFreeSlotsCount,
	}
}

// pairsWalk - walk through pairs and collect stats.
func pairsWalk(db *pgxpool.Pool, sshconf *ssh.ClientConfig, pairsSchema, brigadesSchema, statsSchema, statsfile string) error {
	dataCenterStats := getDataCenterStats(db, pairsSchema, brigadesSchema)

	groups, err := getBrigadesGroups(db, pairsSchema, brigadesSchema)
	if err != nil {
		return fmt.Errorf("get brigades groups: %w", err)
	}

	sem := make(chan struct{}, ParallelCollectorsLimit) // Semaphore for limiting parallel collectors.
	var wgg sync.WaitGroup

	stream := make(chan *AggrStats, ParallelCollectorsLimit)
	var wgh sync.WaitGroup

	wgh.Add(1)
	go handleStatsStream(db, statsSchema, statsfile, stream, &wgh, dataCenterStats)

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
		return nil, fmt.Errorf("ssh run: %w", err)
	}

	groupStats, err := io.ReadAll(httputil.NewChunkedReader(&b))
	if err != nil {
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
func readConfigs() (string, string, string, string, string, string, error) {
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

	dcName := os.Getenv("DC_NAME")
	if dcName == "" {
		dcName = defaultDCName
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshKeyDefaultPath)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("ssh key: %w", err)
	}

	return sshKeyFilename, dbURL, pairsSchema, brigadesSchema, brigadesStatsSchema, dcName, nil
}
