package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/portal"
)

const version = "0.0.1"

func initializePortal(configPath string) (http.Handler, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	portal, err := portal.New(cfg)
	if err != nil {
		return nil, err
	}

	return portal.Routes(), nil
}

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	log.Printf("Starting MediSynth Portal v%s with config: %s", version, *configPath)

	handler, err := initializePortal(*configPath)
	if err != nil {
		log.Fatal(err)
	}

	// Get port from environment variable, fallback to config file
	port := os.Getenv("MEDISYNTH_API_PORT")
	if port == "" {
		cfg, err := config.LoadConfig(*configPath)
		if err != nil {
			log.Fatal(err)
		}
		port = strconv.Itoa(cfg.APIPort)
	}

	log.Printf("Starting portal server on 0.0.0.0:%s", port)
	if err := http.ListenAndServe(fmt.Sprintf("0.0.0.0:%s", port), handler); err != nil {
		log.Fatalf("could not start server: %v", err)
	}
}
