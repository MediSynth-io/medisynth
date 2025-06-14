package main

import (
	"log"

	"github.com/MediSynth-io/medisynth/internal/api"
	"github.com/MediSynth-io/medisynth/internal/config"
)

const version = "0.0.1"

var configInit = config.Init

func initializeAPI() (*api.Api, error) {
	cfg, err := configInit()
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
	api, err := initializeAPI()
	if err != nil {
		log.Fatal(err)
	}

	api.Serve()
}
