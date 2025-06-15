package main

import (
	"log"
	"os"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/database"
)

func main() {
	log.Printf("Testing database initialization...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Config loaded - DB Path: %s", cfg.Database.Path)

	// Check if we can access the directory
	dbDir := "/data"
	if stat, err := os.Stat(dbDir); err != nil {
		log.Printf("Cannot access %s: %v", dbDir, err)
	} else {
		log.Printf("Directory %s exists, mode: %v", dbDir, stat.Mode())
	}

	// Try to initialize database
	if err := database.Init(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	log.Printf("Database initialization successful!")

	// Test database connection
	db := database.GetConnection()
	if db == nil {
		log.Fatalf("Database connection is nil")
	}

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	log.Printf("Database connection test successful!")
}
