package config

// Init initializes the configuration from environment variables
func Init() (*Config, error) {
	return LoadConfig()
}
