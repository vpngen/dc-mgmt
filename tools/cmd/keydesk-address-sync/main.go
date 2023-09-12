package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/vpngen/realm-admin/internal/kdlib"
	dcmgmt "github.com/vpngen/realm-admin/internal/kdlib/dc-mgmt"
)

const (
	maxPostgresqlNameLen = 63
	defaultDatabaseURL   = "postgresql:///vgrealm"
)

const (
	defaultBrigadesSchema = "brigades"
)

const (
	sshKeyDefaultPath = "/etc/vg-dc-vpnapi"
)

var LogTag = setLogTag()

const defaultLogTag = "keydes-address-sync"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	sshKeyFile, dbname, schema, ident, kdAddrSyncServerUser, kdAddrSyncServer, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	kdAddrSyncSSHconf, err := kdlib.CreateSSHConfig(sshKeyFile, kdAddrSyncServerUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("Can't create keydesk address ssh config: %s\n", err)
	}

	db, err := kdlib.CreateDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	kdAddrList, err := dcmgmt.NewKdAddrList(context.Background(), db, schema)
	if err != nil {
		log.Fatalf("%s: Can't get keydesk address list: %s\n", LogTag, err)
	}

	if kdAddrList == "" {
		log.Fatalf("%s: Empty keydesk address list\n", LogTag)
	}

	fmt.Fprintf(os.Stderr, "%s: %s@%s\n", LogTag, kdAddrSyncSSHconf.User, kdAddrSyncServer)
	cleanup, err := dcmgmt.SyncKdAddrList(kdAddrSyncSSHconf, kdAddrSyncServer, ident, kdAddrList)
	defer cleanup(LogTag)

	if err != nil {
		log.Fatalf("%s: Can't sync keydesk address list: %s\n", LogTag, err)
	}
}

func readConfigs() (string, string, string, string, string, string, error) {
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		dbURL = defaultDatabaseURL
	}

	brigadeSchema := os.Getenv("BRIGADES_SCHEMA")
	if brigadeSchema == "" {
		brigadeSchema = defaultBrigadesSchema
	}

	sshKeyFilename, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshKeyDefaultPath)
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("ssh key: %w", err)
	}

	_, ident, err := dcmgmt.ParseDCNameEnv()
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("dc name: %w", err)
	}

	user, server, err := dcmgmt.ParseConnEnv("KEYDESK_ADDRESS_SYNC_CONNECT")
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("keydesk address sync connect: %w", err)
	}

	return sshKeyFilename, dbURL, brigadeSchema, ident, user, server, nil
}
