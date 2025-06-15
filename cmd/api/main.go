package main

import (
	"flag"
	"log"

	"github.com/MediSynth-io/medisynth/internal/api"
	"github.com/MediSynth-io/medisynth/internal/config"
)

const version = "0.0.1"

func initializeAPI(configPath string) (*api.Api, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	api, err := api.NewApi(*cfg)
	if err != nil {
		return nil, err
	}

	return api, nil
}

func main() {
	configPath := flag.String("config", "app.yml", "Path to configuration file")
	flag.Parse()

	log.Printf("Starting MediSynth API v%s with config: %s", version, *configPath)

	api, err := initializeAPI(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	api.Serve()
}
