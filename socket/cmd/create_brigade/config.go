package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/vpngen/wordsgens/namesgenerator"

	"github.com/vpngen/dc-mgmt/internal/kdlib"
	dcmgmtlib "github.com/vpngen/dc-mgmt/internal/kdlib/dc-mgmt"
)

const (
	// DatabaseURLEnvName - the name of the environment variable that
	// contains the database URL.
	DatabaseURLEnvName = "DATABASE_URL"
	// JWTKeyFileEnvName - the name of the environment variable that
	// contains the JWT key.
	LogLevelEnvName = "LOG_LEVEL"
	// ManagementRandomResponsesEnvName - the name of the environment variable
	// that enables random responses for testing in the datacenter.
	ManagementRandomResponsesEnvName = "MANAGEMENT_RANDOM_RESPONSES"

	// PairsAppEnvName - the name of the environment variable that contains
	// the name of the script which creates pairs.
	PairsAppEnvName = "PAIRS_APP"
)

const (
	// DefaultDatabaseURL - the default database URL.
	DefaultDatabaseURL = "postgresql:///vgrealm"
	// DefaultLogLevel - the default log level.
	DefaultLogLevel = slog.LevelInfo

	// DefaultPairsApp - the default script which creates pairs.
	DefaultPairsApp = "/opt/socket-control-endpoints/hetzner_pairs.sh"
)

const (
	sshkeyDefaultPath       = "/etc/vg-dc-vpnapi"
	defaultMaxUsers         = 150
	defaultWireguardConfigs = "native"
)

type Config struct {
	DatabaseURL string // Database URL

	MgmtRandomResponses bool // Random responses for testing in the datacenter

	LogLevel slog.Level // Log level

	BrigadeID   uuid.UUID // Brigade ID
	BrigadeName string    // Brigade name

	Zone string // Zone

	PairsApp string // Script which creates pairs

	DCIdent string // Datacenter identifier

	// Subdomain API.
	SubdomAPIHost  string // Subdomain API host
	SubdomAPIToken string // Subdomain API token

	// Delegation sync.
	DelegationSyncUser string // Delegation sync user
	DelegationSyncHost string // Delegation sync host

	// Name servers.
	NameServers []string // Domain nameservers

	// Config types.
	WG      string // Wireguard configs
	OVC     string // OVC configs
	IPsec   string // IPsec configs
	Outline string // Outline configs

	SSHKeyFile string // SSH key file

	MaxUsers int // Max users
}

var (
	ErrInvalidLogLevel = fmt.Errorf("invalid log level")
	ErrBrigadeName     = fmt.Errorf("brigade name is empty")
)

func (c *Config) configDatabaseURL() error {
	dsn := os.Getenv(DatabaseURLEnvName)
	if dsn == "" {
		c.DatabaseURL = DefaultDatabaseURL
		return nil
	}

	if _, err := url.Parse(dsn); err != nil {
		return fmt.Errorf("invalid database URL: %w: %s", err, dsn)
	}

	c.DatabaseURL = dsn

	return nil
}

func (c *Config) configLogLevel() error {
	level := strings.TrimSpace(strings.ToLower(os.Getenv(LogLevelEnvName)))

	switch level {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	case "":
		c.LogLevel = DefaultLogLevel
	default:
		return fmt.Errorf("%w: %s", ErrInvalidLogLevel, level)
	}

	return nil
}

func (c *Config) configPairsApp() {
	c.PairsApp = os.Getenv(PairsAppEnvName)
	if c.PairsApp == "" {
		c.PairsApp = DefaultPairsApp
	}
}

func (c *Config) configRandomResponses() {
	c.MgmtRandomResponses = os.Getenv(ManagementRandomResponsesEnvName) != ""
}

func (c *Config) realmEnv() error {
	_, dcident, err := dcmgmtlib.ParseDCNameEnv()
	if err != nil {
		return fmt.Errorf("dc name: %w", err)
	}

	c.DCIdent = dcident

	return nil
}

func (c *Config) subdomCreds() error {
	host := os.Getenv("SUBDOMAIN_API_SERVER")
	if host == "" {
		return errors.New("empty subdomapi host")
	}

	if _, err := netip.ParseAddrPort(host); err != nil {
		return fmt.Errorf("parse subdomapi host: %w", err)
	}

	token := os.Getenv("SUBDOMAIN_API_TOKEN")
	if token == "" {
		return errors.New("empty subdomapi token")
	}

	c.SubdomAPIHost = host
	c.SubdomAPIToken = token

	return nil
}

func (c *Config) delegationSync() error {
	user, server, err := dcmgmtlib.ParseConnEnv("DELEGATION_SYNC_CONNECT")
	if err != nil {
		return fmt.Errorf("delegation sync connect: %w", err)
	}

	ns := os.Getenv("DOMAIN_NAMESERVERS")
	if ns == "" {
		return errors.New("empty domain nameservers")
	}

	c.NameServers = strings.Split(ns, ",")

	c.DelegationSyncUser = user
	c.DelegationSyncHost = server

	return nil
}

func (c *Config) vpnConfigTypes() error {
	// Some code to configure types
	wg := os.Getenv("WIREGUARD_CONFIGS")
	if wg == "" {
		wg = defaultWireguardConfigs
	}

	ovc := os.Getenv("OVC_CONFIGS")
	ipsec := os.Getenv("IPSEC_CONFIGS")
	outline := os.Getenv("OUTLINE_CONFIGS")

	c.WG = wg
	c.OVC = ovc
	c.IPsec = ipsec
	c.Outline = outline

	return nil
}

func (c *Config) sshConfig() error {
	sshKey, err := kdlib.LookupForSSHKeyfile(os.Getenv("SSH_KEY"), sshkeyDefaultPath)
	if err != nil {
		return fmt.Errorf("lookup for ssh key: %w", err)
	}

	c.SSHKeyFile = sshKey

	return nil
}

// readEnv - reads configuration from environment variables.
func (c *Config) readEnv() error {
	// Configure log level.
	if err := c.configLogLevel(); err != nil {
		return fmt.Errorf("log level: %w", err)
	}

	// Configure database URL.
	if err := c.configDatabaseURL(); err != nil {
		return fmt.Errorf("database URL: %w", err)
	}

	// Random responses.
	c.configRandomResponses()

	// Pairs app.
	c.configPairsApp()

	// Datacenter identifier.
	if err := c.realmEnv(); err != nil {
		return fmt.Errorf("dc name: %w", err)
	}

	// Subdomain API.
	if err := c.subdomCreds(); err != nil {
		return fmt.Errorf("subdomain creds: %w", err)
	}

	// Delegation sync.
	if err := c.delegationSync(); err != nil {
		return fmt.Errorf("delegation sync: %w", err)
	}

	// VPN config types.
	if err := c.vpnConfigTypes(); err != nil {
		return fmt.Errorf("vpn config types: %w", err)
	}

	// SSH config.
	if err := c.sshConfig(); err != nil {
		return fmt.Errorf("ssh config: %w", err)
	}

	return nil // just for future cases
}

func (c *Config) readArgs() error {
	// Some code to read args from os.Args
	id := flag.String("id", "", "brigade ID (UUID form)")
	name := flag.String("name", "", "brigade name")
	zone := flag.String("zone", "", "zone")
	auto := flag.Bool("auto", false, "auto-generate brigade ID and name")
	maxusers := flag.Int("maxusers", 0, "max users")

	flag.Parse()

	switch *auto {
	case true:
		c.BrigadeID = uuid.New()

		name, _, err := namesgenerator.IndianNameShort()
		if err != nil {
			return fmt.Errorf("auto-generate brigade name: %w", err)
		}

		c.BrigadeName = name
	default:
		if *name == "" {
			return ErrBrigadeName
		}

		brigadeID, err := uuid.Parse(*id)
		if err != nil {
			return fmt.Errorf("brigade ID: %w", err)
		}

		c.BrigadeName = *name
		c.BrigadeID = brigadeID
	}

	c.MaxUsers = *maxusers

	if *maxusers == 0 {
		c.MaxUsers = defaultMaxUsers
	}

	c.Zone = *zone

	return nil // just for future cases
}

// PrintInfo - prints configuration information to the logger.
func (c *Config) PrintInfo(logger *slog.Logger) {
	logger.Info("log level", "level", c.LogLevel)

	dburl, _ := url.Parse(c.DatabaseURL)

	logger.Info("database config", "dburl", dburl.Redacted())

	if c.MgmtRandomResponses {
		logger.Warn("random responses from management are enabled")
	}
}

// NewConfig - creates a new configuration structure.
func NewConfig() (*Config, error) {
	c := &Config{}

	if err := c.readEnv(); err != nil {
		return nil, fmt.Errorf("envs: %w", err)
	}

	if err := c.readArgs(); err != nil {
		return nil, fmt.Errorf("args: %w", err)
	}

	return c, nil
}
