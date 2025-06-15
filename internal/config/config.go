package config

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	APIPort        int    `mapstructure:"API_PORT"`
	APIURL         string `mapstructure:"API_URL"`
	APIInternalURL string `mapstructure:"API_INTERNAL_URL"`

	// Database configuration (flattened)
	DatabaseType       string `mapstructure:"DB_TYPE"`        // "sqlite" or "postgres"
	DatabasePath       string `mapstructure:"DB_PATH"`        // SQLite file path
	DatabaseSocketPath string `mapstructure:"DB_SOCKET_PATH"` // SQLite socket path
	DatabaseWALMode    bool   `mapstructure:"DB_WAL_MODE"`    // SQLite WAL mode
	DatabaseMaxRetries int    `mapstructure:"DB_MAX_RETRIES"` // SQLite retries
	DatabaseRetryDelay int    `mapstructure:"DB_RETRY_DELAY"` // SQLite retry delay

	// PostgreSQL configuration
	DatabaseHost            string `mapstructure:"DB_HOST"`                    // PostgreSQL host
	DatabasePort            string `mapstructure:"DB_PORT"`                    // PostgreSQL port
	DatabaseName            string `mapstructure:"DB_NAME"`                    // PostgreSQL database name
	DatabaseUser            string `mapstructure:"DB_USER"`                    // PostgreSQL username
	DatabasePassword        string `mapstructure:"DB_PASSWORD"`                // PostgreSQL password
	DatabaseSSLMode         string `mapstructure:"DB_SSL_MODE"`                // PostgreSQL SSL mode
	DatabaseMaxConns        int    `mapstructure:"DB_MAX_CONNECTIONS"`         // PostgreSQL max connections
	DatabaseMaxIdle         int    `mapstructure:"DB_MAX_IDLE_CONNECTIONS"`    // PostgreSQL max idle connections
	DatabaseConnMaxLifetime string `mapstructure:"DB_CONNECTION_MAX_LIFETIME"` // PostgreSQL connection max lifetime

	// Domain configuration (flattened)
	DomainPortal string `mapstructure:"DOMAIN_PORTAL"`
	DomainAPI    string `mapstructure:"DOMAIN_API"`
	DomainSecure bool   `mapstructure:"DOMAIN_SECURE"`

	// DigitalOcean Spaces configuration
	S3Endpoint        string `mapstructure:"S3_ENDPOINT"`          // e.g. https://nyc3.digitaloceanspaces.com
	S3Region          string `mapstructure:"S3_REGION"`            // e.g. nyc3, ams3, sgp1
	S3Bucket          string `mapstructure:"S3_BUCKET"`            // Your DigitalOcean Space name
	S3AccessKeyID     string `mapstructure:"S3_ACCESS_KEY_ID"`     // DigitalOcean Spaces Key
	S3SecretAccessKey string `mapstructure:"S3_SECRET_ACCESS_KEY"` // DigitalOcean Spaces Secret
	S3UseSSL          bool   `mapstructure:"S3_USE_SSL"`

	// Admin configuration
	AdminEmails []string `mapstructure:"ADMIN_EMAILS"` // Comma-separated list of admin emails

	// Bitcoin configuration
	BitcoinAddress string `mapstructure:"BITCOIN_ADDRESS"` // Bitcoin wallet address for receiving payments
}

// Database returns a database config struct for backward compatibility
func (c *Config) Database() DatabaseConfig {
	return DatabaseConfig{
		Path:       c.DatabasePath,
		SocketPath: c.DatabaseSocketPath,
		WALMode:    c.DatabaseWALMode,
		MaxRetries: c.DatabaseMaxRetries,
		RetryDelay: c.DatabaseRetryDelay,
	}
}

// Domains returns a domains config struct for backward compatibility
func (c *Config) Domains() DomainsConfig {
	return DomainsConfig{
		Portal: c.DomainPortal,
		API:    c.DomainAPI,
		Secure: c.DomainSecure,
	}
}

type DatabaseConfig struct {
	Path       string
	SocketPath string
	WALMode    bool
	MaxRetries int
	RetryDelay int
}

type DomainsConfig struct {
	Portal string
	API    string
	Secure bool
}

// LoadConfig loads the configuration from environment variables.
func LoadConfig() (*Config, error) {
	v := viper.New()

	// Set defaults first
	v.SetDefault("API_PORT", 8081)
	v.SetDefault("API_URL", "https://api.medisynth.io")
	v.SetDefault("API_INTERNAL_URL", "http://medisynth-api-svc:8081")
	v.SetDefault("DB_TYPE", "sqlite")
	v.SetDefault("DB_PATH", "/data/medisynth.db")
	v.SetDefault("DB_SOCKET_PATH", "/data/sqlite.sock")
	v.SetDefault("DB_WAL_MODE", true)
	v.SetDefault("DB_MAX_RETRIES", 5)
	v.SetDefault("DB_RETRY_DELAY", 5)
	v.SetDefault("DB_HOST", "")
	v.SetDefault("DB_PORT", "")
	v.SetDefault("DB_NAME", "")
	v.SetDefault("DB_USER", "")
	v.SetDefault("DB_PASSWORD", "")
	v.SetDefault("DB_SSL_MODE", "disable")
	v.SetDefault("DB_MAX_CONNECTIONS", 10)
	v.SetDefault("DB_MAX_IDLE_CONNECTIONS", 5)
	v.SetDefault("DB_CONNECTION_MAX_LIFETIME", "0")
	v.SetDefault("DOMAIN_PORTAL", "portal.medisynth.io")
	v.SetDefault("DOMAIN_API", "api.medisynth.io")
	v.SetDefault("DOMAIN_SECURE", true)
	v.SetDefault("S3_ENDPOINT", "https://nyc3.digitaloceanspaces.com")
	v.SetDefault("S3_REGION", "nyc3")
	v.SetDefault("S3_BUCKET", "medisynth-data")
	v.SetDefault("S3_ACCESS_KEY_ID", "")
	v.SetDefault("S3_SECRET_ACCESS_KEY", "")
	v.SetDefault("S3_USE_SSL", true)
	v.SetDefault("ADMIN_EMAILS", "")
	v.SetDefault("BITCOIN_ADDRESS", "")

	// Then, tell Viper to read environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Manually handle comma-separated strings
	adminEmailsStr := v.GetString("ADMIN_EMAILS")
	if adminEmailsStr != "" {
		cfg.AdminEmails = strings.Split(adminEmailsStr, ",")
		for i, email := range cfg.AdminEmails {
			cfg.AdminEmails[i] = strings.TrimSpace(email)
		}
	}

	// Final verification log
	log.Printf("Configuration loaded successfully.")
	if cfg.BitcoinAddress != "" {
		log.Printf("Bitcoin address loaded from env: %s", cfg.BitcoinAddress)
	} else {
		log.Printf("CRITICAL WARNING: BITCOIN_ADDRESS is not set. Payment functionality will fail.")
	}
	if cfg.DatabaseType == "postgres" {
		log.Printf("Postgres host loaded from env: %s", cfg.DatabaseHost)
	}

	return &cfg, nil
}

// IsAdmin checks if the given email is in the admin list
func (c *Config) IsAdmin(email string) bool {
	for _, adminEmail := range c.AdminEmails {
		if strings.EqualFold(adminEmail, email) {
			return true
		}
	}
	return false
}
