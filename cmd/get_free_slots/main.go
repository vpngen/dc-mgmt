package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/vpngen/realm-admin/internal/kdlib"

	"github.com/coreos/go-systemd/activation"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultBrigadesSchema = "brigades"
	defaultPairsSchema    = "pairs"
	defaultDCName         = "unknown"
	defaultDCID           = "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	KeySlotsAllTotal = iota
	KeySlotsAllActive
	KeySlotsFreeTotal
	KeySlotsFreeActive
)

var ErrNoListener = errors.New("no listener")

var LogTag = setLogTag()

const defaultLogTag = "get_free_slots"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	var w io.WriteCloser

	chunked, jsonFormat, active, listener, err := parseArgs()
	if err != nil {
		log.Fatalf("%s: Can't parse args: %s\n", LogTag, err)
	}

	dbURL, pairsSchema, brigadesSchema, dcName, dcID, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	db, err := createDBPool(dbURL)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	if listener == nil {

		var (
			num    int32
			output []byte
		)

		switch active {
		case KeySlotsFreeTotal:
			num, err = getFreeSlotsNumber(db, brigadesSchema, false)
			if err != nil {
				log.Fatalf("%s: Can't get free slots number: %s\n", LogTag, err)
			}

			output, err = getFormattedFreeSlotsNumber(num, false, jsonFormat)
			if err != nil {
				log.Fatalf("%s: Can't format nums: %s\n", LogTag, err)
			}
		case KeySlotsFreeActive:
			num, err = getFreeSlotsNumber(db, brigadesSchema, true)
			if err != nil {
				log.Fatalf("%s: Can't get free slots number: %s\n", LogTag, err)
			}

			output, err = getFormattedFreeSlotsNumber(num, true, jsonFormat)
			if err != nil {
				log.Fatalf("%s: Can't format nums: %s\n", LogTag, err)
			}
		case KeySlotsAllTotal:
			num, err = getAllSlotsNumber(db, pairsSchema, false)
			if err != nil {
				log.Fatalf("%s: Can't get all slots number: %s\n", LogTag, err)
			}

			output, err = getFormattedAllSlotsNumber(num, false, jsonFormat)
			if err != nil {
				log.Fatalf("%s: Can't format nums: %s\n", LogTag, err)
			}
		case KeySlotsAllActive:
			num, err = getAllSlotsNumber(db, pairsSchema, true)
			if err != nil {
				log.Fatalf("%s: Can't get all slots number: %s\n", LogTag, err)
			}

			output, err = getFormattedAllSlotsNumber(num, true, jsonFormat)
			if err != nil {
				log.Fatalf("%s: Can't format nums: %s\n", LogTag, err)
			}
		}

		switch chunked {
		case true:
			w = httputil.NewChunkedWriter(os.Stdout)
			defer w.Close()
		default:
			w = os.Stdout
		}

		if output == nil {
			output = []byte{}
		}

		_, err = w.Write(output)
		if err != nil {
			log.Fatalf("%s: Can't print output: %s\n", LogTag, err)
		}

		return
	}

	router := mux.NewRouter()
	router.HandleFunc("/metrics/datacenter/free_slots", func(w http.ResponseWriter, r *http.Request) {
		zabbixRequestFreeSlotsHandler(w, r, db, brigadesSchema, dcName, dcID)
	})
	router.HandleFunc("/metrics/datacenter/all_slots", func(w http.ResponseWriter, r *http.Request) {
		zabbixRequestAllSlotsHandler(w, r, db, brigadesSchema, dcName, dcID)
	})

	server := &http.Server{
		Handler:     router,
		IdleTimeout: 60 * time.Minute,
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Can't serve: %s\n", err)
		}
	}()

	// On signal, gracefully shut down the server and wait 5
	// seconds for current connections to stop.

	wg := &sync.WaitGroup{}

	done := make(chan struct{})
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit

		fmt.Fprintln(os.Stderr, "Quit signal received...")

		closeFunc := func(srv *http.Server) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			srv.SetKeepAlivesEnabled(false)
			if err := srv.Shutdown(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "Can't gracefully shut down the server: %s\n", err)
			}
		}

		fmt.Fprintln(os.Stderr, "Server is shutting down")
		wg.Add(1)

		go closeFunc(server)

		wg.Wait()

		close(done)
	}()

	// Wait for existing connections before exiting.
	<-done
}

func getFreeSlotsNumber(db *pgxpool.Pool, schema string, active bool) (int32, error) {
	var num int32

	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	sql := kdlib.GetFreeSlotsNumberStatement(schema, active)

	if err := tx.QueryRow(ctx,
		sql,
	).Scan(&num); err != nil {
		return 0, fmt.Errorf("slots query: %w", err)
	}

	return num, nil
}

func getAllSlotsNumber(db *pgxpool.Pool, schema string, active bool) (int32, error) {
	var num int32

	ctx := context.Background()

	tx, err := db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}

	defer tx.Rollback(ctx)

	sql := kdlib.GetAllSlotsNumberStatement(schema, active)

	if err := tx.QueryRow(ctx,
		sql,
	).Scan(&num); err != nil {
		return 0, fmt.Errorf("slots query: %w", err)
	}

	return num, nil
}

func getFormattedFreeSlotsNumber(num int32, active, jsonFormat bool) ([]byte, error) {
	if jsonFormat {
		return kdlib.GetFreeSlotsNumberJSONBytes(num, active), nil
	}

	return fmt.Appendf([]byte{}, "%d", num), nil
}

func getFormattedAllSlotsNumber(num int32, active, jsonFormat bool) ([]byte, error) {
	if jsonFormat {
		return kdlib.GetAllSlotsNumberJSONBytes(num, active), nil
	}

	return fmt.Appendf([]byte{}, "%d", num), nil
}

func zabbixRequestAllSlotsHandler(w http.ResponseWriter, r *http.Request, db *pgxpool.Pool, schema, dcName, dcID string) {
	if r.URL.Query().Get("format") != "zabbix" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
		return
	}

	switch r.URL.Query().Get("action") {
	case "list":
		zabbixResponse := fmt.Sprintf(
			"[{\"{#VPNGEN_DATACENTER_NAME}\": \"%s\", \"{#VPNGEN_DATACENTER_ID}\": \"%s\"}]",
			dcName,
			dcID,
		)

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(zabbixResponse))

		return
	case "get_total_number":
		id := r.URL.Query().Get("id")
		if id != dcID {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))

			return
		}

		num, err := getAllSlotsNumber(db, schema, false)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))

			return
		}

		zabbixResponse := fmt.Sprintf("%d\n", num)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(zabbixResponse))

		return
	case "get_active_number":
		id := r.URL.Query().Get("id")
		if id != dcID {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))

			return
		}

		num, err := getAllSlotsNumber(db, schema, true)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))

			return
		}

		zabbixResponse := fmt.Sprintf("%d\n", num)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(zabbixResponse))

		return
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
	}
}

func zabbixRequestFreeSlotsHandler(w http.ResponseWriter, r *http.Request, db *pgxpool.Pool, schema, dcName, dcID string) {
	if r.URL.Query().Get("format") != "zabbix" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
		return
	}

	switch r.URL.Query().Get("action") {
	case "list":
		zabbixResponse := fmt.Sprintf(
			"[{\"{#VPNGEN_DATACENTER_NAME}\": \"%s\", \"{#VPNGEN_DATACENTER_ID}\": \"%s\"}]",
			dcName,
			dcID,
		)

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(zabbixResponse))

		return
	case "get_total_number":
		id := r.URL.Query().Get("id")
		if id != dcID {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))

			return
		}

		num, err := getFreeSlotsNumber(db, schema, false)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))

			return
		}

		zabbixResponse := fmt.Sprintf("%d\n", num)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(zabbixResponse))

		return
	case "get_active_number":
		id := r.URL.Query().Get("id")
		if id != dcID {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request"))

			return
		}

		num, err := getFreeSlotsNumber(db, schema, true)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal server error"))

			return
		}

		zabbixResponse := fmt.Sprintf("%d\n", num)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(zabbixResponse))

		return
	default:
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid request"))
	}
}

func createDBPool(dbURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("conn string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	return pool, nil
}

func parseArgs() (bool, bool, int, net.Listener, error) {
	chunked := flag.Bool("ch", false, "chunked output")
	slotsAllTotal := flag.Bool("at", false, "all slots")
	slotsAllActive := flag.Bool("aa", false, "all active slots")
	slotsFreeTotal := flag.Bool("ft", false, "free slots")
	slotsFreeActive := flag.Bool("fa", false, "free active slots")
	jsonFormat := flag.Bool("j", false, "json output")
	listenAddr := flag.String("l", "", "Listen addr:port (http and https separate with commas)")

	flag.Parse()

	if *listenAddr != "" {
		if l, err := net.Listen("tcp", *listenAddr); err == nil {
			return *chunked, *jsonFormat, 0, l, nil
		}

		listeners, err := activation.Listeners()
		if err != nil {
			return false, false, 0, nil, ErrNoListener
		}

		if len(listeners) == 0 {
			return false, false, 0, nil, ErrNoListener
		}

		return *chunked, *jsonFormat, 0, listeners[0], nil
	}

	if *slotsAllTotal {
		return *chunked, *jsonFormat, KeySlotsAllTotal, nil, nil
	}

	if *slotsAllActive {
		return *chunked, *jsonFormat, KeySlotsAllActive, nil, nil
	}

	if *slotsFreeTotal {
		return *chunked, *jsonFormat, KeySlotsFreeTotal, nil, nil
	}

	if *slotsFreeActive {
		return *chunked, *jsonFormat, KeySlotsFreeActive, nil, nil
	}

	return *chunked, *jsonFormat, KeySlotsAllTotal, nil, nil
}

func readConfigs() (string, string, string, string, string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	brigadesSchema := os.Getenv("BRIGADES_SCHEMA")
	if brigadesSchema == "" {
		brigadesSchema = defaultBrigadesSchema
	}

	pairsSchema := os.Getenv("PAIRS_SCHEMA")
	if brigadesSchema == "" {
		pairsSchema = defaultPairsSchema
	}

	dcName := os.Getenv("DC_NAME")
	if dcName == "" {
		dcName = defaultDCName
	}

	dcID := os.Getenv("DC_ID")
	if dcID == "" {
		dcID = defaultDCID
	}

	return dbURL, pairsSchema, brigadesSchema, dcName, dcID, nil
}
