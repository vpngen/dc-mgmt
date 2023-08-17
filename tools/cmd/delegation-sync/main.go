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

const defaultLogTag = "delegation-sync"

func setLogTag() string {
	executable, err := os.Executable()
	if err != nil {
		return defaultLogTag
	}

	return filepath.Base(executable)
}

func main() {
	sshKeyFile, dbname, schema, ident, delegationSyncServerUser, delegationSyncServer, err := readConfigs()
	if err != nil {
		log.Fatalf("%s: Can't read configs: %s\n", LogTag, err)
	}

	delegationSyncSSHconf, err := kdlib.CreateSSHConfig(sshKeyFile, delegationSyncServerUser, kdlib.SSHDefaultTimeOut)
	if err != nil {
		log.Fatalf("Can't create delegation sync ssh config: %s\n", err)
	}

	db, err := kdlib.CreateDBPool(dbname)
	if err != nil {
		log.Fatalf("%s: Can't create db pool: %s\n", LogTag, err)
	}

	delegationList, err := dcmgmt.NewDelegationList(context.Background(), db, schema)
	if err != nil {
		log.Fatalf("%s: Can't get delegation list: %s\n", LogTag, err)
	}

	if delegationList == "" {
		log.Fatalf("%s: Empty delegation list\n", LogTag)
	}

	fmt.Fprintf(os.Stderr, "%s: %s@%s\n", LogTag, delegationSyncSSHconf.User, delegationSyncServer)
	cleanup, err := dcmgmt.SyncDelegationList(delegationSyncSSHconf, delegationSyncServer, ident, delegationList)
	defer cleanup(LogTag)

	if err != nil {
		log.Fatalf("%s: Can't sync delegation list: %s\n", LogTag, err)
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

	user, server, err := dcmgmt.ParseConnEnv("DELEGATION_SYNC_CONNECT")
	if err != nil {
		return "", "", "", "", "", "", fmt.Errorf("delegation sync connect: %w", err)
	}

	return sshKeyFilename, dbURL, brigadeSchema, ident, user, server, nil
}
