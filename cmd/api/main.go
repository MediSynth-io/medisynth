package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/handlers"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

const version = "0.0.1"

func main() {
	configPath := "." // Default for running binary from project root or Docker
	if _, err := os.Stat(filepath.Join(configPath, "app.yml")); os.IsNotExist(err) {
		altPath := filepath.Join("..")
		if _, errStatAlt := os.Stat(filepath.Join(altPath, "app.yml")); errStatAlt == nil {
			configPath = altPath
			log.Printf("Info: app.yml not found in CWD (.), attempting to use path: %s", configPath)
		} else {
			log.Printf("Warning: app.yml not found in CWD (.) or at %s. Using current directory '.' for config. config.Init() might fail.", filepath.Join(altPath, "app.yml"))
		}
	}

	cfg, err := config.Init()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v. Ensure app.yml is accessible. Current effective config search path (if used by Init): %s", err, configPath)
	}

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// Routes
	r.Get("/heartbeat", handlers.Heartbeat)
	r.Post("/generate-patients", handlers.RunSyntheaGeneration)

	log.Printf("Starting server on port %d...", cfg.APIPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.APIPort), r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
