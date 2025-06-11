package main

import (
	"fmt"
	"log"

	"github.com/MediSynth-io/medisynth/internal/api"
	"github.com/MediSynth-io/medisynth/internal/config"
)

const version = "0.0.1"

func main() {
	config, err := config.Init()
	if err != nil {
		log.Fatal(err)
	}

	api, err := api.NewApi(config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(api.Config.ApiPort)

	api.Serve()
}
