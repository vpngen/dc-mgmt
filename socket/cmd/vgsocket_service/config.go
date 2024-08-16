/*
## Configuration hierarchy

In this hierarchy, each level can override the levels below it. A typical hierarchy might be:

* **Default values**: These are the lowest priority and can be hardcoded into the application or included in a default configuration file that is bundled with the application.
* **Configuration files**: These have a higher priority than default values. A common practice is to have a base configuration file for default values, and then a separate, environment-specific configuration file that can override those values.
* **Environment variables**: These are higher priority than configuration files. Environment variables are typically used for settings that vary between deployment environments (such as dev, test, prod), and for sensitive information like passwords or API keys.
* **Command-line arguments**: These have the highest priority and can override all other settings. Command-line arguments are often used for one-off changes that shouldn't be persisted in a configuration file or environment variable.
*/
package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/ssh"
)

const (
	// ApplicationListenPortEnvName - the name of the environment variable
	// that contains the listen port for the HTTP server. It is assumed
	// that the application will be running in a container and the listen
	// address will be set to [::].
	ApplicationListenPortEnvName = "APP_PORT"
	// DatabaseURLEnvName - the name of the environment variable that
	// contains the database URL.
	DatabaseURLEnvName = "DATABASE_URL"
	// PermissiveCORSEnvName PermissiveCORS - the name of the environment variable that
	// contains the CORS mode.
	PermissiveCORSEnvName = "PERMISSIVE_CORS"
	// JWTPubKeyFileEnvName - the name of the environment variable that
	// contains the JWT key.
	JWTPubKeyFileEnvName = "JWT_PUBLIC_KEY_FILE"
	// JWTAlgorithmEnvName - the name of the environment variable that
	// contains the JWT algorithm.
	JWTAlgorithmEnvName = "JWT_ALGORITHM"
	// LogLevelEnvName - the name of the environment variable that
	// contains the log level. Valid values are: debug, info, warn, error.
	LogLevelEnvName = "LOG_LEVEL"
	// RandomResponsesEnvName - the name of the environment variable that
	// enables random responses for testing.
	RandomResponsesEnvName = "RANDOM_RESPONSES"
	// KeydeskRandomResponsesEnvName - the name of the environment variable
	// that enables random responses for testing in the datacenter.
	KeydeskRandomResponsesEnvName = "KEYDESK_RANDOM_RESPONSES"
	// KeydeskAccessKeyEnvName - the name of the environment variable that
	// contains the keydesk access key.
	KeydeskAccessKeyEnvName = "KEYDESK_ACCESS_KEY"
)

const (
	// DefaultListenPort - the default listen port for the HTTP server.
	DefaultListenPort = "9081"
	// DefaultDatabaseURL - the default database URL.
	DefaultDatabaseURL = "postgres://postgres:postgres@postgres:5432/postgres"
	// DefaultJWTAlgorithm - the default JWT algorithm.
	DefaultJWTAlgorithm = "EdDSA"
	// DefaultLogLevel - the default log level.
	DefaultLogLevel = slog.LevelInfo
)

type Config struct {
	Listen string // Listen address for the HTTP server

	DatabaseURL string // Database URL

	PermissiveCORS bool // Permissive CORS mode

	JWTAlgorithm     string            // JWT algorithm
	JWTVerifyKey     interface{}       // JWT verification key
	JWTSigningMethod jwt.SigningMethod // JWT signing method

	KdAccessKey string // Datacenter API key

	RandomResponses   bool // Random responses for testing
	KdRandomResponses bool // Random responses for testing in the datacenter

	LogLevel slog.Level // Log level
}

// availableJWTAlgs - the list of available JWT algorithms.
var availableJWTAlgs = jwt.GetAlgorithms()

var (
	// ErrUnknownJWTAlgorithm - the error returned when the JWT algorithm
	// is unknown.
	ErrUnknownJWTAlgorithm = errors.New("unknown JWT algorithm")

	// ErrInvalidLogLevel - the error returned when the log level is invalid.
	// Valid values are: debug, info, warn, error.
	ErrInvalidLogLevel = errors.New("invalid log level")

	// ErrInvalidJWTKey - the error returned when the JWT key is invalid.
	ErrInvalidJWTKey = errors.New("invalid JWT key")

	// ErrDatacenterAccessKeyNotSet - the error returned when the datacenter
	// access key is not set.
	ErrDatacenterAccessKeyNotSet = errors.New("datacenter access key is not set")
)

const maxKeyFileSize = 1 << 20 // 1 MB

func (c *Config) configJWT() error {
	var err error

	filename := os.Getenv(JWTPubKeyFileEnvName)
	f, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open JWT pubkey file: %w", err)
	}

	defer f.Close()

	buf, err := io.ReadAll(io.LimitReader(f, maxKeyFileSize))
	if err != nil {
		return fmt.Errorf("read JWT pubkey file: %w", err)
	}

	alg := os.Getenv(JWTAlgorithmEnvName)
	for _, a := range availableJWTAlgs {
		if a == alg {
			c.JWTAlgorithm = alg
			break
		}
	}

	if c.JWTAlgorithm == "" {
		c.JWTAlgorithm = DefaultJWTAlgorithm
	}

	switch c.JWTAlgorithm {
	case "EdDSA":
		pk, _, _, _, err := ssh.ParseAuthorizedKey(buf)
		if err != nil {
			return fmt.Errorf("parse EdDSA key: %w", err)
		}

		ck, ok := pk.(ssh.CryptoPublicKey)
		if !ok {
			return ErrInvalidJWTKey
		}

		c.JWTVerifyKey, ok = ck.CryptoPublicKey().(ed25519.PublicKey)
		if !ok {
			return ErrInvalidJWTKey
		}

		c.JWTSigningMethod = jwt.SigningMethodEdDSA
	case "ES256", "ES384", "ES512":
		pk, _, _, _, err := ssh.ParseAuthorizedKey(buf)
		if err != nil {
			return fmt.Errorf("parse EdDSA key: %w", err)
		}

		ck, ok := pk.(ssh.CryptoPublicKey)
		if !ok {
			return ErrInvalidJWTKey
		}

		c.JWTVerifyKey, ok = ck.CryptoPublicKey().(*ecdsa.PublicKey)
		if !ok {
			return ErrInvalidJWTKey
		}

		switch c.JWTAlgorithm {
		case "ES256":
			c.JWTSigningMethod = jwt.SigningMethodES256
		case "ES384":
			c.JWTSigningMethod = jwt.SigningMethodES384
		case "ES512":
			c.JWTSigningMethod = jwt.SigningMethodES512
		}
	case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512":
		pk, _, _, _, err := ssh.ParseAuthorizedKey(buf)
		if err != nil {
			return fmt.Errorf("parse EdDSA key: %w", err)
		}

		ck, ok := pk.(ssh.CryptoPublicKey)
		if !ok {
			return ErrInvalidJWTKey
		}

		c.JWTVerifyKey, ok = ck.CryptoPublicKey().(*rsa.PublicKey)
		if !ok {
			return ErrInvalidJWTKey
		}

		switch c.JWTAlgorithm {
		case "RS256":
			c.JWTSigningMethod = jwt.SigningMethodRS256
		case "RS384":
			c.JWTSigningMethod = jwt.SigningMethodRS384
		case "RS512":
			c.JWTSigningMethod = jwt.SigningMethodRS512
		case "PS256":
			c.JWTSigningMethod = jwt.SigningMethodPS256
		case "PS384":
			c.JWTSigningMethod = jwt.SigningMethodPS384
		case "PS512":
			c.JWTSigningMethod = jwt.SigningMethodPS512
		}
	case "HS256", "HS384", "HS512":
		c.JWTVerifyKey = buf

		switch c.JWTAlgorithm {
		case "HS256":
			c.JWTSigningMethod = jwt.SigningMethodHS256
		case "HS384":
			c.JWTSigningMethod = jwt.SigningMethodHS384
		case "HS512":
			c.JWTSigningMethod = jwt.SigningMethodHS512
		}
	default:
		return fmt.Errorf("%w: %s", ErrUnknownJWTAlgorithm, c.JWTAlgorithm)
	}

	return nil
}

func (c *Config) configListenPort() error {
	port := os.Getenv(ApplicationListenPortEnvName)

	if port == "" {
		c.Listen = ":" + DefaultListenPort

		return nil
	}

	if _, err := strconv.Atoi(port); err != nil {
		return fmt.Errorf("invalid listen port: %w: %s", err, port)
	}

	c.Listen = ":" + port

	return nil
}

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

func (c *Config) configRandomResponses() {
	c.RandomResponses = os.Getenv(RandomResponsesEnvName) != ""
	c.KdRandomResponses = os.Getenv(KeydeskRandomResponsesEnvName) != ""
}

func (c *Config) configDatacenterAccessKey() {
	c.KdAccessKey = os.Getenv(KeydeskAccessKeyEnvName)
}

// readEnv - reads configuration from environment variables.
func (c *Config) readEnv() error {
	// Configure log level.
	if err := c.configLogLevel(); err != nil {
		return fmt.Errorf("log level: %w", err)
	}

	// Configure listen port.
	if err := c.configListenPort(); err != nil {
		return fmt.Errorf("listen port: %w", err)
	}

	// Configure database URL.
	if err := c.configDatabaseURL(); err != nil {
		return fmt.Errorf("database URL: %w", err)
	}

	// Permissive CORS mode.
	c.PermissiveCORS = os.Getenv(PermissiveCORSEnvName) == "yes"

	// Configure JWT.
	if err := c.configJWT(); err != nil {
		return fmt.Errorf("JWT: %w", err)
	}

	// Random responses.
	c.configRandomResponses()

	// Datacenter access key.
	c.configDatacenterAccessKey()

	if c.KdAccessKey == "" && !c.KdRandomResponses {
		return ErrDatacenterAccessKeyNotSet
	}

	return nil // just for future cases
}

func (c *Config) readArgs() error {
	// Some code to read args from os.Args

	return nil // just for future cases
}

// PrintInfo - prints configuration information to the logger.
func (c *Config) PrintInfo(logger *slog.Logger) {
	logger.Info("log level", "level", c.LogLevel)

	if c.PermissiveCORS {
		logger.Warn("CORS is permissive")
	}

	dburl, _ := url.Parse(c.DatabaseURL)

	logger.Info("listen config", "listen", c.Listen)
	logger.Info("database config", "dburl", dburl.Redacted())
	logger.Info("JWT config", "algorithm", c.JWTAlgorithm)

	if c.RandomResponses {
		logger.Warn("random responses are enabled")
	}

	if c.KdRandomResponses {
		logger.Warn("datacenter random responses are enabled")
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
