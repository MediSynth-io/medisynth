package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSyntheaClient is a mock implementation of the SyntheaClient interface
type MockSyntheaClient struct {
	mock.Mock
}

func (m *MockSyntheaClient) GeneratePatients(params map[string]interface{}) (string, error) {
	args := m.Called(params)
	return args.String(0), args.Error(1)
}

func (m *MockSyntheaClient) GetStatus(jobID string) (map[string]interface{}, error) {
	args := m.Called(jobID)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockSyntheaClient) GetResults(jobID string) ([]byte, error) {
	args := m.Called(jobID)
	return args.Get(0).([]byte), args.Error(1)
}

func setupTestAPI(t *testing.T) (*Api, *MockSyntheaClient) {
	// Create mock dependencies
	mockSynthea := new(MockSyntheaClient)

	// Create API instance with mocks
	cfg := config.Config{APIPort: 8081}
	api, err := NewApi(cfg)
	assert.NoError(t, err)

	// Set up test router
	router := chi.NewRouter()
	router.Post("/generate-patients", api.RunSyntheaGeneration)
	router.Get("/generation-status/{jobID}", api.GetGenerationStatus)

	return api, mockSynthea
}

func TestProcessSyntheaJob_ErrorAndSuccess(t *testing.T) {
	cfg := config.Config{APIPort: 8081}
	apiInstance, _ := NewApi(cfg)

	// Suppress log output during tests
	originalLogger := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalLogger)

	// Setup mock for exec.Command
	originalExec := execCommand
	defer func() { execCommand = originalExec }()

	t.Run("SyntheaFails", func(t *testing.T) {
		resetGlobalJobStore()
		execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
			// Create a command that will fail and output an error
			cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'Synthea failed' >&2; exit 1")
			return cmd
		}

		jobID := "failjob"
		params := SyntheaParams{Population: Pint(1), OutputFormat: Pstr("fhir")}
		job := &GenerationJob{
			ID:            jobID,
			Status:        StatusPending,
			RequestParams: params,
		}
		globalJobStore.AddJob(job)

		// Process the job synchronously for testing
		apiInstance.processSyntheaJob(job)

		// Give a small delay for status update
		time.Sleep(100 * time.Millisecond)

		updatedJob, exists := globalJobStore.GetJob(jobID)
		if !exists {
			t.Fatalf("Job %s not found in store", jobID)
		}
		if updatedJob.Status != StatusFailed {
			t.Errorf("Expected status %s, got %s", StatusFailed, updatedJob.Status)
		}
		if updatedJob.Error == "" {
			t.Error("Expected error message, got none")
		}
	})

	t.Run("SyntheaSucceeds", func(t *testing.T) {
		resetGlobalJobStore()
		execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
			// Create a command that will succeed
			cmd := exec.CommandContext(ctx, "sh", "-c", "echo 'Synthea completed successfully'")
			return cmd
		}

		jobID := "successjob"
		params := SyntheaParams{Population: Pint(1), OutputFormat: Pstr("fhir")}
		job := &GenerationJob{
			ID:            jobID,
			Status:        StatusPending,
			RequestParams: params,
		}
		globalJobStore.AddJob(job)

		// Process the job synchronously for testing
		apiInstance.processSyntheaJob(job)

		// Give a small delay for status update
		time.Sleep(100 * time.Millisecond)

		updatedJob, exists := globalJobStore.GetJob(jobID)
		if !exists {
			t.Fatalf("Job %s not found in store", jobID)
		}
		if updatedJob.Status != StatusCompleted {
			t.Errorf("Expected status %s, got %s", StatusCompleted, updatedJob.Status)
		}
		if updatedJob.Error != "" {
			t.Errorf("Expected no error, got %s", updatedJob.Error)
		}
	})
}

func TestNewJobID(t *testing.T) {
	// Test that job IDs are unique
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := newJobID()
		if ids[id] {
			t.Errorf("Duplicate job ID generated: %s", id)
		}
		ids[id] = true
	}

	// Test error path by mocking randRead to fail
	originalRandRead := randRead
	defer func() { randRead = originalRandRead }()

	randRead = func([]byte) (n int, err error) {
		return 0, assert.AnError
	}

	id := newJobID()
	if id != "fallback-jobid" {
		t.Errorf("Expected fallback job ID, got %s", id)
	}
}

func TestGetGenerationStatusHandler(t *testing.T) {
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
