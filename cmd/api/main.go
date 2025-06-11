package main

import (
	"log"

	"github.com/MediSynth-io/medisynth/internal/api"
	"github.com/MediSynth-io/medisynth/internal/config"
)

const version = "0.0.1"

func main() {
	cfg, err := config.Init()
	if err != nil {
		log.Fatal(err)
	}

	api, err := api.NewApi(*cfg)
	if err != nil {
		log.Fatal(err)
	}

	api.Serve()
}
