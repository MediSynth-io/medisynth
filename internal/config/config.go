package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	APIPort int `yaml:"apiPort"`
}

// LoadConfig loads the configuration from file and environment variables.
func LoadConfig(path string) (*Config, error) {
	v := viper.New()

	// Set up config file handling
	v.SetConfigFile(path)   // Use the full path to the config file
	v.SetConfigType("toml") // Set the config type to toml
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

	log.Printf("Configuration loaded: %+v", cfg)
	return &cfg, nil
}
