package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/portal"
)

const version = "0.0.1"

func initializePortal() (http.Handler, error) {
	cfg, err := config.LoadConfig()
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
	log.Printf("Starting MediSynth Portal v%s", version)

	handler, err := initializePortal()
	if err != nil {
		log.Fatal(err)
	}

	// Get port from environment variable, fallback to config file
	port := os.Getenv("API_PORT")
	if port == "" {
		cfg, err := config.LoadConfig()
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
