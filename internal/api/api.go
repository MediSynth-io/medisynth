package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Api struct {
	Config config.Config
}

func NewApi(config config.Config) (*Api, error) {
	if config.APIPort == 0 {
		return nil, errors.New("Must have at least a port to start API")
	}

	api := Api{
		Config: config,
	}

	return &api, nil
}

func (api *Api) Serve() {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	// Routes
	r.Get("/heartbeat", api.Heartbeat)
	r.Post("/generate-patients", api.RunSyntheaGeneration)

	log.Printf("Starting server on port %d...", api.Config.APIPort)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", api.Config.APIPort), r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	http.ListenAndServe(fmt.Sprintf(":%d", api.Config.APIPort), r)
}
