package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/go-chi/chi/v5"
)

type Api struct {
	Config config.Config
}

func NewApi(config config.Config) (*Api, error) {
	if config.ApiPort == 0 {
		return nil, errors.New("Must have at least a port to start API")
	}

	api := Api{
		Config: config,
	}

	return &api, nil
}

func (api *Api) Serve() {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})

	http.ListenAndServe(fmt.Sprintf(":%d", api.Config.ApiPort), r)
}
