package main

import (
	"flag"
	"log"

	"github.com/MediSynth-io/medisynth/internal/api"
	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/store"
)

const version = "0.0.1"

func initializeAPI(configPath string) (*api.Api, error) {
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Initialize database
	if err := database.Init(cfg); err != nil {
		return nil, err
	}

	// Initialize store
	dataStore := store.New()

	// Initialize auth with store
	auth.SetStore(dataStore)

	// Initialize API
	api, err := api.NewApi(*cfg)
	if err != nil {
		return nil, err
	}

	return api, nil
}

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	log.Printf("Starting MediSynth API v%s with config: %s", version, *configPath)

	api, err := initializeAPI(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	api.Serve()
}
