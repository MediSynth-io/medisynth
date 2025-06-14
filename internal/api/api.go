package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"encoding/json"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/portal"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type Api struct {
	Config config.Config
	portal *portal.Portal
}

func NewApi(config config.Config) (*Api, error) {
	if config.APIPort == 0 {
		return nil, errors.New("Must have at least a port to start API")
	}

	// Initialize database
	if err := database.Init(&config); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %v", err)
	}

	// Initialize portal
	p, err := portal.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize portal: %v", err)
	}

	api := Api{
		Config: config,
		portal: p,
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

	// Custom NotFound handler for debugging
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("CHI ROUTER - NOT FOUND: Path='%s', RawQuery='%s'", r.URL.Path, r.URL.RawQuery)
		http.Error(w, fmt.Sprintf("Custom 404 - Path Not Found: %s", r.URL.Path), http.StatusNotFound)
	})

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	r.Get("/heartbeat", api.Heartbeat)

	// Auth routes
	r.Post("/auth/register", auth.RegisterHandler)
	r.Post("/auth/login", auth.LoginHandler)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth)

		// Token management
		r.Post("/auth/tokens", auth.CreateTokenHandler)
		r.Get("/auth/tokens", auth.ListTokensHandler)
		r.Delete("/auth/tokens", auth.DeleteTokenHandler)

		// API routes
		r.Post("/generate-patients", api.RunSyntheaGeneration)
		r.Get("/generation-status/{jobID}", api.GetGenerationStatus)
	})

	// Portal routes
	r.Mount("/portal", api.portal.Routes())

	log.Printf("Starting server on port %d...", api.Config.APIPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", api.Config.APIPort), r))
}

func (api *Api) Heartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (api *Api) RunSyntheaGeneration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var req struct {
		Count      int `json:"count"`
		Population int `json:"population"`
		Age        int `json:"age,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "invalid JSON"})
		return
	}
	patientCount := req.Count
	if patientCount == 0 {
		patientCount = req.Population
	}
	if patientCount == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "count is required"})
		return
	}
	// Create a job (simulate)
	jobID := "job-123" // In real code, generate unique ID
	job := &GenerationJob{
		ID:        jobID,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	globalJobStore.AddJob(job)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"job_id": jobID, "jobID": jobID})
}

func (api *Api) GetGenerationStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	jobID := chi.URLParam(r, "jobID")
	job, exists := globalJobStore.GetJob(jobID)
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "job not found"})
		return
	}
	resp := map[string]interface{}{"status": string(job.Status)}
	if job.Status == StatusCompleted {
		resp["progress"] = 100
	}
	json.NewEncoder(w).Encode(resp)
}
