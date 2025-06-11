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
	v.AddConfigPath(path)  // Path to look for the config file in
	v.SetConfigName("app") // Name of config file (without extension)
	v.SetConfigType("yml") // REQUIRED if the config file does not have the extension in the name
	v.AutomaticEnv()       // Read in environment variables that match
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// If a config file is found, read it in.
	if err := v.ReadInConfig(); err != nil {
		log.Printf("Warning: Could not read config file: %s. Using defaults or environment variables.", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	// Set default port if not specified
	if cfg.APIPort == 0 {
		cfg.APIPort = 8080 // Default port
		log.Println("APIPort not specified, using default 8080")
	}

	log.Printf("Configuration loaded: %+v", cfg)
	return &cfg, nil
}
