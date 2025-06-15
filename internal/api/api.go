package api

import (
	"fmt"
	"log"
	"net/http"

	"encoding/json"
	"strings"
	"time"

	"context"
	"sync"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/MediSynth-io/medisynth/internal/s3"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

type Api struct {
	Config   config.Config
	Router   *chi.Mux
	S3Client *s3.Client
}

func NewApi(cfg config.Config) (*Api, error) {
	s3Client, err := s3.NewClient(&cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	api := &Api{
		Config:   cfg,
		Router:   chi.NewRouter(),
		S3Client: s3Client,
	}
	api.setupRoutes()
	return api, nil
}

func (api *Api) setupRoutes() {
	r := api.Router

	// Enhanced middleware with real IP logging
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				clientIP := middleware.GetReqID(r.Context())
				if clientIP == "" {
					clientIP = r.RemoteAddr
				}
				// Get real client IP from headers
				if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
					clientIP = realIP
				} else if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
					clientIP = strings.Split(forwarded, ",")[0]
				}

				log.Printf("[API] %s %s %s %d %d bytes in %v - Real IP: %s",
					r.Method, r.URL.Path, r.Proto, ww.Status(), ww.BytesWritten(),
					time.Since(start), clientIP)
			}()

			next.ServeHTTP(ww, r)
		})
	})
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://*.local:*", "http://localhost:*", "http://127.0.0.1:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Root API info endpoint
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service": "MediSynth API",
			"version": "v1.0.0",
			"status":  "running",
			"endpoints": map[string]string{
				"health":   "/heartbeat",
				"docs":     "/swagger/",
				"generate": "/generate-patients",
				"status":   "/generation-status/{jobID}",
			},
		})
	})

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

		// Job-related routes
		r.Post("/generate-patients", api.RunSyntheaGeneration)
		r.Get("/generation-status/{jobID}", api.GetGenerationStatus)
		r.Get("/jobs", api.ListJobsHandler)
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
			if strings.HasPrefix(host, config.DomainPortal) {
				portalHandler.ServeHTTP(w, r)
				return
			}

			if strings.HasPrefix(host, config.DomainAPI) {
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

// --- Job Handlers ---

var runningJobs = make(map[string]context.CancelFunc)
var runningJobsMutex sync.Mutex

func (api *Api) RunSyntheaGeneration(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized: User ID not found in token", http.StatusUnauthorized)
		return
	}

	var params models.SyntheaParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	job := &models.Job{
		ID:           "job-" + database.GenerateID(),
		UserID:       userID,
		JobID:        "synthea-" + database.GenerateID(),
		Status:       models.JobStatusPending,
		Parameters:   params.ToMap(),
		OutputFormat: params.GetOutputFormat(),
	}

	if err := job.MarshalParameters(); err != nil {
		http.Error(w, "Failed to process job parameters", http.StatusInternalServerError)
		return
	}

	if err := database.CreateJob(job); err != nil {
		log.Printf("ERROR: Failed to create job in database: %v", err)
		http.Error(w, "Failed to create job", http.StatusInternalServerError)
		return
	}

	go api.executeSyntheaJob(job)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobID":     job.ID,
		"status":    job.Status,
		"message":   "Job accepted and is pending execution.",
		"statusUrl": fmt.Sprintf("/generation-status/%s", job.ID),
	})
}

func (api *Api) executeSyntheaJob(job *models.Job) {
	_, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	runningJobsMutex.Lock()
	runningJobs[job.ID] = cancel
	runningJobsMutex.Unlock()

	defer func() {
		runningJobsMutex.Lock()
		delete(runningJobs, job.ID)
		runningJobsMutex.Unlock()
		cancel()
	}()

	log.Printf("Starting Synthea generation for job %s", job.ID)
	database.UpdateJobStatus(job.ID, models.JobStatusRunning, nil, nil, nil, nil)

	time.Sleep(10 * time.Second) // Simulate work

	s3KeyPrefix := fmt.Sprintf("synthea_output/%s/", job.JobID)
	log.Printf("Simulating S3 upload for job %s to path %s", job.ID, s3KeyPrefix)

	population, _ := job.Parameters["population"].(float64)
	patientCount := int(population)

	err := database.UpdateJobStatus(job.ID, models.JobStatusCompleted, nil, &s3KeyPrefix, nil, &patientCount)
	if err != nil {
		log.Printf("ERROR: Failed to update job %s to completed: %v", job.ID, err)
		return
	}

	log.Printf("Job %s completed successfully", job.ID)
}

func (api *Api) GetGenerationStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	job, err := database.GetJobByID(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	userID, _ := r.Context().Value("userID").(string)
	if job.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

func (api *Api) ListJobsHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized: User ID not found in token", http.StatusUnauthorized)
		return
	}

	jobs, err := database.GetJobsByUserID(userID)
	if err != nil {
		log.Printf("ERROR: Failed to get jobs for user %s: %v", userID, err)
		http.Error(w, "Failed to retrieve job history", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(jobs)
}
