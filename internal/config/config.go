package config

import (
	"log"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	APIPort  int `yaml:"apiPort"`
	Database struct {
		Path       string `yaml:"path"`
		SocketPath string `yaml:"socketPath"`
		WALMode    bool   `yaml:"walMode"`
		MaxRetries int    `yaml:"maxRetries"`
		RetryDelay int    `yaml:"retryDelay"`
	} `yaml:"database"`
	Domains struct {
		Portal string `yaml:"portal"`
		API    string `yaml:"api"`
		Secure bool   `yaml:"secure"`
	} `yaml:"domains"`
}

// LoadConfig loads the configuration from file and environment variables.
func LoadConfig(path string) (*Config, error) {
	v := viper.New()

	// Set up config file handling
	v.SetConfigFile(path)   // Use the full path to the config file
	v.SetConfigType("yaml") // Set the config type to yaml
	v.AutomaticEnv()        // Read in environment variables that match
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Try to read the config file
	if err := v.ReadInConfig(); err != nil {
		// If the file doesn't exist or is invalid, return an error
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		log.Printf("Warning: Could not read config file: %s. Using defaults or environment variables.", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Set default port if not specified
	if cfg.APIPort == 0 {
		cfg.APIPort = 8081 // Default port
		log.Println("APIPort not specified, using default 8081")
	}

	// Set default database path if not specified
	if cfg.Database.Path == "" {
		cfg.Database.Path = "/data/medisynth.db"
		log.Println("Database path not specified, using default /data/medisynth.db")
	}

	// Set default socket path if not specified
	if cfg.Database.SocketPath == "" {
		cfg.Database.SocketPath = "/data/sqlite.sock"
		log.Println("Database socket path not specified, using default /data/sqlite.sock")
	}

	// Set default WAL mode
	if !cfg.Database.WALMode {
		cfg.Database.WALMode = true
		log.Println("WAL mode not specified, enabling by default")
	}

	// Set default retry settings
	if cfg.Database.MaxRetries == 0 {
		cfg.Database.MaxRetries = 5
	}
	if cfg.Database.RetryDelay == 0 {
		cfg.Database.RetryDelay = 5
	}

	// Set default domains if not specified
	if cfg.Domains.Portal == "" {
		cfg.Domains.Portal = "portal.medisynth.io" // Default to production domain
		log.Println("Portal domain not specified, using default portal.medisynth.io")
	}
	if cfg.Domains.API == "" {
		cfg.Domains.API = "api.medisynth.io" // Default to production domain
		log.Println("API domain not specified, using default api.medisynth.io")
	}

	// Only set secure default if it wasn't specified in the config file
	if !v.IsSet("domains.secure") {
		// Default to secure only in production
		env := os.Getenv("MEDISYNTH_ENV")
		cfg.Domains.Secure = env == "prod"
		log.Printf("Domain security not specified, defaulting to %v based on environment", cfg.Domains.Secure)
	}

	log.Printf("Configuration loaded: %+v", cfg)
	return &cfg, nil
}
