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
	"github.com/MediSynth-io/medisynth/internal/portal"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Api struct {
	Config config.Config
	portal *portal.Portal
	Router *chi.Mux
}

func NewApi(cfg config.Config) (*Api, error) {
	api := &Api{
		Config: cfg,
		Router: chi.NewRouter(),
	}

	// Initialize other components like portal if necessary
	// portal, err := portal.New(&cfg)
	// if err != nil {
	// 	return nil, err
	// }
	// api.portal = portal

	api.setupRoutes()
	return api, nil
}

func (api *Api) setupRoutes() {
	r := api.Router
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/heartbeat"))

	// Public routes
	r.Post("/register", api.RegisterHandler)
	r.Post("/login", api.LoginHandler)

	// Protected routes for token management
	r.Group(func(r chi.Router) {
		r.Use(api.TokenAuthMiddleware)
		r.Post("/tokens", api.CreateTokenHandler)
		r.Get("/tokens", api.ListTokensHandler)
		r.Delete("/tokens/{tokenID}", api.DeleteTokenHandler)
	})
}

func (api *Api) Serve() {
	r := chi.NewRouter()

	// Add CORS middleware before other middleware
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://*.local:*", "http://localhost:*", "http://127.0.0.1:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	// Add a simple, standalone ping route for debugging
	r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	})

	// Custom NotFound handler for debugging
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("CHI ROUTER - NOT FOUND: Path='%s', RawQuery='%s'", r.URL.Path, r.URL.RawQuery)

		// Don't redirect if we're already on the portal domain
		if r.Host == api.Config.Domains.Portal {
			http.Error(w, fmt.Sprintf("Custom 404 - Path Not Found: %s", r.URL.Path), http.StatusNotFound)
			return
		}

		// Don't redirect API endpoints
		if strings.HasPrefix(r.URL.Path, "/auth/") || r.URL.Path == "/heartbeat" {
			http.Error(w, fmt.Sprintf("Custom 404 - Path Not Found: %s", r.URL.Path), http.StatusNotFound)
			return
		}

		// For non-API paths on api domain, redirect to portal
		if r.Host == api.Config.Domains.API {
			scheme := "http"
			if api.Config.Domains.Secure {
				scheme = "https"
			}
			portalURL := fmt.Sprintf("%s://%s:%d%s", scheme, api.Config.Domains.Portal, api.Config.APIPort, r.URL.Path)
			http.Redirect(w, r, portalURL, http.StatusSeeOther)
		} else {
			http.Error(w, fmt.Sprintf("Custom 404 - Path Not Found: %s", r.URL.Path), http.StatusNotFound)
		}
	})

	// Mount routes based on domain
	r.Group(func(r chi.Router) {
		r.Use(DomainMiddleware(api.portal.Routes(), api.Routes(), &api.Config))
	})

	// Serve the swagger-ui and the spec
	r.Handle("/swagger/*", http.StripPrefix("/swagger/", http.FileServer(http.Dir("./swagger-ui"))))

	// Auth routes
	r.Post("/register", api.RegisterHandler)
	r.Post("/login", api.LoginHandler)

	// Protected routes for token management
	r.Group(func(r chi.Router) {
		r.Use(api.TokenAuthMiddleware)
		r.Post("/tokens", api.CreateTokenHandler)
		r.Get("/tokens", api.ListTokensHandler)
		r.Delete("/tokens/{tokenID}", api.DeleteTokenHandler)
	})

	// Add a simple heartbeat endpoint
	r.Get("/heartbeat", api.Heartbeat)

	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			err := auth.CleanupExpiredSessions()
			if err != nil {
				log.Printf("Error cleaning up expired sessions: %v", err)
			}
			<-ticker.C
		}
	}()

	log.Printf("Starting API server on 0.0.0.0:%d", api.Config.APIPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("0.0.0.0:%d", api.Config.APIPort), r))
}

func (api *Api) Routes() http.Handler {
	apiRoutes := chi.NewRouter()
	apiRoutes.Get("/heartbeat", api.Heartbeat)
	apiRoutes.Post("/auth/register", auth.RegisterHandler)
	apiRoutes.Post("/auth/login", auth.LoginHandler)

	// Protected routes
	apiRoutes.Group(func(r chi.Router) {
		r.Use(auth.AuthMiddleware(auth.GetTokenManager()))
		r.Use(auth.RequireAuth)

		// Token management
		r.Post("/auth/tokens", auth.CreateTokenHandler)
		r.Get("/auth/tokens", auth.ListTokensHandler)
		r.Delete("/auth/tokens/{id}", auth.DeleteTokenHandler)

		// API routes
		r.Post("/generate-patients", api.RunSyntheaGeneration)
		r.Get("/generation-status/{jobID}", api.GetGenerationStatus)
	})
	return apiRoutes
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
