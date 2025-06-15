package api

import (
	"fmt"
	"log"
	"net/http"

	"encoding/json"
	"strings"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Api struct {
	Config config.Config
	Router *chi.Mux
}

func NewApi(cfg config.Config) (*Api, error) {
	api := &Api{
		Config: cfg,
		Router: chi.NewRouter(),
	}
	api.setupRoutes()
	return api, nil
}

func (api *Api) setupRoutes() {
	r := api.Router

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://*.local:*", "http://localhost:*", "http://127.0.0.1:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public routes
	r.Get("/heartbeat", api.Heartbeat)
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})
	r.Post("/register", api.RegisterHandler)
	r.Post("/login", api.LoginHandler)

	// Protected API routes
	r.Group(func(r chi.Router) {
		r.Use(api.TokenAuthMiddleware)
		r.Post("/tokens", api.CreateTokenHandler)
		r.Get("/tokens", api.ListTokensHandler)
		r.Delete("/tokens/{tokenID}", api.DeleteTokenHandler)
		r.Post("/generate-patients", api.RunSyntheaGeneration)
		r.Get("/generation-status/{jobID}", api.GetGenerationStatus)
	})

	// Swagger UI
	r.Handle("/swagger/*", http.StripPrefix("/swagger/", http.FileServer(http.Dir("./swagger-ui"))))
}

func (api *Api) Serve() {
	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				err := auth.CleanupExpiredSessions()
				if err != nil {
					log.Printf("Error cleaning up expired sessions: %v", err)
				}
			}
		}
	}()

	log.Printf("Starting API server on 0.0.0.0:%d", api.Config.APIPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", api.Config.APIPort), api.Router))
}

func DomainMiddleware(portalHandler, apiHandler http.Handler, config *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract host and port
			hostParts := strings.Split(r.Host, ":")
			host := hostParts[0]
			port := "80"
			if len(hostParts) > 1 {
				port = hostParts[1]
			}

			// Try exact domain matches first
			if strings.HasPrefix(host, config.Domains.Portal) {
				portalHandler.ServeHTTP(w, r)
				return
			}

			if strings.HasPrefix(host, config.Domains.API) {
				apiHandler.ServeHTTP(w, r)
				return
			}

			// If no exact match, try localhost or IP
			if host == "localhost" || host == "127.0.0.1" {
				// Use the port to determine which service to route to
				if port == fmt.Sprintf("%d", config.APIPort) {
					apiHandler.ServeHTTP(w, r)
					return
				}
				if port == "8082" { // Portal port
					portalHandler.ServeHTTP(w, r)
					return
				}
			}

			// Log unmatched requests for debugging
			log.Printf("Unmatched request - Host: %s, Port: %s, Path: %s", host, port, r.URL.Path)

			// If no domain matches, proceed to the next handler
			next.ServeHTTP(w, r)
		})
	}
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
