package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
)

func TestNewApi(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		cfg := config.Config{APIPort: 8080}
		apiInstance, err := NewApi(cfg)
		if err != nil {
			t.Fatalf("NewApi failed with valid config: %v", err)
		}
		if apiInstance == nil {
			t.Fatal("NewApi returned nil with valid config")
		}
		if apiInstance.Config.APIPort != 8080 {
			t.Errorf("Expected APIPort 8080, got %d", apiInstance.Config.APIPort)
		}
	})

	t.Run("InvalidConfigZeroPort", func(t *testing.T) {
		cfg := config.Config{APIPort: 0}
		_, err := NewApi(cfg)
		if err == nil {
			t.Fatal("NewApi should have failed with zero APIPort, but it didn't")
		}
		expectedErrorMsg := "Must have at least a port to start API"
		if !strings.Contains(err.Error(), expectedErrorMsg) {
			t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
		}
	})
}

func TestServe(t *testing.T) {
	// Create API instance with test port
	cfg := config.Config{APIPort: 8081}
	api, err := NewApi(cfg)
	assert.NoError(t, err)

	// Start server in a goroutine
	go func() {
		api.Serve()
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Test root endpoint
	resp, err := http.Get("http://localhost:8081/")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "hello", string(body))

	// Test heartbeat endpoint
	resp, err = http.Get("http://localhost:8081/heartbeat")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var heartbeatResp map[string]string
	err = json.NewDecoder(resp.Body).Decode(&heartbeatResp)
	resp.Body.Close()
	assert.NoError(t, err)
	assert.Equal(t, "ok", heartbeatResp["status"])

	// Test non-existent endpoint
	resp, err = http.Get("http://localhost:8081/nonexistent")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// Helper to set up a test server for routing tests
func setupTestServer(t *testing.T) (*httptest.Server, *Api) {
	cfg := config.Config{APIPort: 8080} // Port doesn't matter for httptest
	apiInstance, err := NewApi(cfg)
	if err != nil {
		t.Fatalf("Failed to create API instance for test server: %v", err)
	}

	// We need to get the router from the Serve method.
	// The Serve method itself blocks, so we can't call it directly.
	// Instead, we replicate its router setup.
	r := chi.NewRouter()
	r.Use(middleware.Logger) // Using middleware to be closer to actual setup
	r.Use(middleware.Recoverer)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// Simplified NotFound for tests, or use the actual one if it doesn't log.Fatalf
		http.Error(w, "Test Not Found", http.StatusNotFound)
	})
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello"))
	})
	r.Get("/heartbeat", apiInstance.Heartbeat)
	r.Post("/generate-patients", apiInstance.RunSyntheaGeneration)
	r.Get("/generation-status/{jobID}", apiInstance.GetGenerationStatus)

	return httptest.NewServer(r), apiInstance
}

func TestAPIRoutes(t *testing.T) {
	server, _ := setupTestServer(t)
	defer server.Close()

	// Reset globalJobStore before each route test group if handlers modify it
	// This is important if tests run in parallel or share state.
	// For simplicity here, assuming serial execution or handlers that don't conflict initially.

	t.Run("RootPath", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/")
		if err != nil {
			t.Fatalf("GET / failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 OK for /, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "hello" {
			t.Errorf("Expected body 'hello', got '%s'", string(body))
		}
	})

	t.Run("Heartbeat", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/heartbeat")
		if err != nil {
			t.Fatalf("GET /heartbeat failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200 OK for /heartbeat, got %d", resp.StatusCode)
		}
		var statusResp map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
			t.Fatalf("Failed to decode /heartbeat response: %v", err)
		}
		if statusResp["status"] != "ok" {
			t.Errorf("Expected status 'ok' in heartbeat response, got '%s'", statusResp["status"])
		}
	})

	t.Run("GeneratePatients_ValidRequest", func(t *testing.T) {
		// Reset job store for this specific test
		globalJobStore.mu.Lock()
		globalJobStore.jobs = make(map[string]*GenerationJob)
		globalJobStore.mu.Unlock()

		payload := `{"population": 1}`
		resp, err := http.Post(server.URL+"/generate-patients", "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatalf("POST /generate-patients failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("Expected status 202 Accepted for /generate-patients, got %d", resp.StatusCode)
		}
		var jobResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&jobResp); err != nil {
			t.Fatalf("Failed to decode /generate-patients response: %v", err)
		}
		if _, ok := jobResp["jobID"]; !ok {
			t.Error("Expected jobID in /generate-patients response")
		}
		// Further checks: job added to store, goroutine (hard to test directly)
		// We can check the job store size or if the specific job ID exists after a short delay
		// For now, just checking the response.
	})

	t.Run("GeneratePatients_InvalidRequest", func(t *testing.T) {
		payload := `{"population": "not-an-int"}` // Malformed JSON
		resp, err := http.Post(server.URL+"/generate-patients", "application/json", strings.NewReader(payload))
		if err != nil {
			t.Fatalf("POST /generate-patients with invalid payload failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400 Bad Request for invalid payload, got %d", resp.StatusCode)
		}
	})

	t.Run("GetGenerationStatus_NotFound", func(t *testing.T) {
		// Reset job store
		globalJobStore.mu.Lock()
		globalJobStore.jobs = make(map[string]*GenerationJob)
		globalJobStore.mu.Unlock()

		resp, err := http.Get(server.URL + "/generation-status/nonexistentjobid")
		if err != nil {
			t.Fatalf("GET /generation-status for non-existent ID failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 Not Found for non-existent job ID, got %d", resp.StatusCode)
		}
	})

	t.Run("NotFoundHandler", func(t *testing.T) {
		resp, err := http.Get(server.URL + "/nonexistentpath")
		if err != nil {
			t.Fatalf("GET /nonexistentpath failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404 Not Found for non-existent path, got %d", resp.StatusCode)
		}
	})

	// More tests for GetGenerationStatus with actual jobs (pending, completed, failed)
	// would require manipulating globalJobStore and potentially waiting for processSyntheaJob.
	// This becomes more of an integration test for the handlers.
}

func TestGetGenerationStatus(t *testing.T) {
	cfg := config.Config{}
	api, _ := NewApi(cfg)

	tests := []struct {
		name           string
		jobID          string
		expectedStatus JobStatus
		expectedCode   int
		addJob         bool
	}{
		{
			name:           "Existing job",
			jobID:          "test-job-id",
			expectedStatus: StatusPending,
			expectedCode:   http.StatusOK,
			addJob:         true,
		},
		{
			name:           "Non-existent job",
			jobID:          "non-existent",
			expectedStatus: "",
			expectedCode:   http.StatusNotFound,
			addJob:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset store
			globalJobStore.mu.Lock()
			globalJobStore.jobs = make(map[string]*GenerationJob)
			globalJobStore.mu.Unlock()

			if tt.addJob {
				job := &GenerationJob{
					ID:     tt.jobID,
					Status: StatusPending,
				}
				globalJobStore.AddJob(job)
			}

			// Use chi router to set URL param
			r := chi.NewRouter()
			r.Get("/generation-status/{jobID}", api.GetGenerationStatus)
			req := httptest.NewRequest("GET", fmt.Sprintf("/generation-status/%s", tt.jobID), nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Check response
			assert.Equal(t, tt.expectedCode, w.Code)
			if tt.expectedCode == http.StatusOK {
				var response map[string]string
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)
				assert.Equal(t, string(tt.expectedStatus), response["status"])
			}
		})
	}
}
