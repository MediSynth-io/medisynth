package api

import (
	"fmt"
	"log"
	"net/http"

	"encoding/json"
	"strings"
	"time"

	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/MediSynth-io/medisynth/internal/s3"
	awsSDKs3 "github.com/aws/aws-sdk-go-v2/service/s3"
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

	// Public root endpoint (limited info)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"service":       "MediSynth API",
			"version":       "v1.0.0",
			"status":        "running",
			"message":       "Authentication required for API endpoints",
			"documentation": "Visit /swagger/ for API documentation",
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
		r.Use(api.UnifiedAuthMiddleware)

		// API Documentation (private)
		r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"service": "MediSynth API",
				"version": "v1.0.0",
				"status":  "running",
				"endpoints": map[string]string{
					"health":   "/heartbeat",
					"docs":     "/docs",
					"swagger":  "/swagger/",
					"generate": "/generate-patients",
					"status":   "/generation-status/{jobID}",
					"jobs":     "/jobs",
					"tokens":   "/tokens",
				},
				"documentation": "Access /swagger/ for interactive API documentation",
			})
		})

		// Swagger UI (private)
		r.Handle("/swagger/*", http.StripPrefix("/swagger/", http.FileServer(http.Dir("./swagger-ui"))))

		// Token management
		r.Post("/tokens", api.CreateTokenHandler)
		r.Get("/tokens", api.ListTokensHandler)
		r.Delete("/tokens/{tokenID}", api.DeleteTokenHandler)

		// Job-related routes
		r.Post("/generate-patients", api.RunSyntheaGeneration)
		r.Get("/generation-status/{jobID}", api.GetGenerationStatus)
		r.Get("/jobs", api.ListJobsHandler)
		r.Get("/jobs/{jobID}/files", api.ListJobFilesHandler)
	})
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
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

	// --- Synthea Execution ---
	outputDir, err := os.MkdirTemp("", "synthea-output-"+job.ID)
	if err != nil {
		log.Printf("ERROR: Failed to create temp dir for job %s: %v", job.ID, err)
		errMsg := "failed to create temp dir"
		database.UpdateJobStatus(job.ID, models.JobStatusFailed, &errMsg, nil, nil, nil)
		return
	}
	defer os.RemoveAll(outputDir)

	log.Printf("Created temp directory for Synthea output: %s", outputDir)

	syntheaArgs, err := job.GetSyntheaArgs()
	if err != nil {
		log.Printf("ERROR: Failed to build Synthea args for job %s: %v", job.ID, err)
		errMsg := "failed to build synthea args"
		database.UpdateJobStatus(job.ID, models.JobStatusFailed, &errMsg, nil, nil, nil)
		return
	}

	// Base synthea command
	cmdArgs := []string{"-p", syntheaArgs.Population}

	// Add other parameters from SyntheaParams as needed
	if syntheaArgs.Gender != "" {
		cmdArgs = append(cmdArgs, "-g", syntheaArgs.Gender)
	}
	if syntheaArgs.AgeRange != "" {
		cmdArgs = append(cmdArgs, "-a", syntheaArgs.AgeRange)
	}
	if syntheaArgs.City != "" {
		cmdArgs = append(cmdArgs, "--city", syntheaArgs.City)
	}

	cmdArgs = append(cmdArgs, "--exporter.base_directory", outputDir)

	log.Printf("Running Synthea for job %s with args: %v", job.ID, cmdArgs)

	cmd := exec.CommandContext(ctx, "synthea", cmdArgs...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	err = cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("Synthea execution failed: %s", errOut.String())
		log.Printf("ERROR: Job %s failed: %s", job.ID, errMsg)
		log.Printf("Synthea stdout: %s", out.String())
		database.UpdateJobStatus(job.ID, models.JobStatusFailed, &errMsg, nil, nil, nil)
		return
	}

	log.Printf("Synthea execution successful for job %s.", job.ID)

	// --- S3 Upload ---
	s3KeyPrefix := fmt.Sprintf("synthea_output/%s/", job.JobID)
	log.Printf("Uploading Synthea output for job %s to S3 path %s", job.ID, s3KeyPrefix)

	err = api.uploadDirectoryToS3(ctx, outputDir, s3KeyPrefix)
	if err != nil {
		errMsg := fmt.Sprintf("S3 upload failed: %v", err)
		log.Printf("ERROR: Job %s failed: %v", job.ID, errMsg)
		database.UpdateJobStatus(job.ID, models.JobStatusFailed, &errMsg, nil, nil, nil)
		return
	}

	population, _ := job.Parameters["population"].(float64)
	patientCount := int(population)

	err = database.UpdateJobStatus(job.ID, models.JobStatusCompleted, nil, &s3KeyPrefix, nil, &patientCount)
	if err != nil {
		log.Printf("ERROR: Failed to update job %s to completed: %v", job.ID, err)
		return
	}

	log.Printf("Job %s completed successfully", job.ID)
}

func (api *Api) uploadDirectoryToS3(ctx context.Context, dir, s3KeyPrefix string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		s3Key := filepath.ToSlash(filepath.Join(s3KeyPrefix, relPath))

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		log.Printf("Uploading %s to s3://%s/%s", path, api.S3Client.BucketName, s3Key)

		_, err = api.S3Client.PutObject(ctx, &awsSDKs3.PutObjectInput{
			Bucket: &api.S3Client.BucketName,
			Key:    &s3Key,
			Body:   file,
		})
		return err
	})
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

func (api *Api) ListJobFilesHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized: User ID not found in token", http.StatusUnauthorized)
		return
	}

	jobID := chi.URLParam(r, "jobID")
	job, err := database.GetJobByID(jobID)
	if err != nil {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	if job.UserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if job.OutputPath == nil || *job.OutputPath == "" {
		http.Error(w, "Job has no output path", http.StatusNotFound)
		return
	}

	files, err := api.S3Client.ListFiles(r.Context(), *job.OutputPath)
	if err != nil {
		log.Printf("ERROR: Failed to list files for job %s: %v", jobID, err)
		http.Error(w, "Failed to list job files", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// --- Auth Handlers ---

func (api *Api) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	user, err := auth.RegisterUser(req.Email, req.Password)
	if err != nil {
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	// We don't return the user object, just a success status
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      user.ID,
		"email":   user.Email,
		"message": "User registered successfully",
	})
}

func (api *Api) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	user, err := auth.ValidateUser(req.Email, req.Password)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Note: This login handler is for the API. It does not set a session cookie.
	// It's intended for clients that need to verify credentials before requesting
	// an API token. A successful response here doesn't mean the user is "logged in"
	// in a session-based sense.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      user.ID,
		"message": "Login successful",
	})
}

// --- Token Handlers ---

func (api *Api) CreateTokenHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized: User ID not found in token", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	token, err := auth.CreateToken(userID, req.Name)
	if err != nil {
		http.Error(w, "Failed to create token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(token)
}

func (api *Api) ListTokensHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized: User ID not found in token", http.StatusUnauthorized)
		return
	}

	tokens, err := auth.ListTokens(userID)
	if err != nil {
		http.Error(w, "Failed to list tokens", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tokens)
}

func (api *Api) DeleteTokenHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized: User ID not found in token", http.StatusUnauthorized)
		return
	}
	tokenID := chi.URLParam(r, "tokenID")

	// Validate that the user owns the token before deleting
	token, err := auth.ValidateToken(tokenID)
	if err != nil {
		http.Error(w, "Token not found", http.StatusNotFound)
		return
	}

	if token.UserID != userID {
		http.Error(w, "Forbidden: You can only delete your own tokens", http.StatusForbidden)
		return
	}

	if err := auth.DeleteToken(userID, tokenID); err != nil {
		http.Error(w, "Failed to delete token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- Middleware ---

func (api *Api) UnifiedAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "Authorization header format must be Bearer {token}", http.StatusUnauthorized)
			return
		}

		tokenStr := parts[1]
		token, err := auth.ValidateToken(tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Add user ID to context
		ctx := context.WithValue(r.Context(), "userID", token.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
