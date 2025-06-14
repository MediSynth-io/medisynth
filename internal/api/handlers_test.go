package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestGeneratePatientsHandler(t *testing.T) {
	api, mockSynthea := setupTestAPI(t)

	// Test cases
	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		mockResponse   string
		mockError      error
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name: "Valid request",
			requestBody: map[string]interface{}{
				"count": 10,
				"age":   30,
			},
			mockResponse:   "job-123",
			mockError:      nil,
			expectedStatus: http.StatusAccepted,
			expectedBody: map[string]interface{}{
				"job_id": "job-123",
			},
		},
		{
			name: "Invalid request - missing count",
			requestBody: map[string]interface{}{
				"age": 30,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody: map[string]interface{}{
				"error": "count is required",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mock expectations
			if tt.mockResponse != "" {
				mockSynthea.On("GeneratePatients", mock.Anything).Return(tt.mockResponse, tt.mockError)
			}

			// Create request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/generate-patients", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Make request
			api.RunSyntheaGeneration(w, req)

			// Assert response
			if tt.name == "Valid request" {
				assert.Equal(t, tt.expectedStatus, w.Code)
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBody["job_id"], response["job_id"])
				if jobID, ok := response["jobID"]; ok {
					assert.Equal(t, response["job_id"], jobID)
				}
				return
			}

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedBody, response)
		})
	}
}

func TestGetGenerationStatusHandler(t *testing.T) {
	api, mockSynthea := setupTestAPI(t)

	// Set up test router
	router := chi.NewRouter()
	router.Get("/generation-status/{jobID}", api.GetGenerationStatus)

	// Test cases
	tests := []struct {
		name           string
		jobID          string
		mockStatus     map[string]interface{}
		mockError      error
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:  "Valid job ID",
			jobID: "job-123",
			mockStatus: map[string]interface{}{
				"status":   "completed",
				"progress": 100,
			},
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"status":   "completed",
				"progress": float64(100),
			},
		},
		{
			name:           "Job not found",
			jobID:          "nonexistent",
			mockError:      assert.AnError,
			expectedStatus: http.StatusNotFound,
			expectedBody: map[string]interface{}{
				"error": "job not found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the job store before each test
			resetGlobalJobStore()

			// Set up mock expectations
			mockSynthea.On("GetStatus", tt.jobID).Return(tt.mockStatus, tt.mockError)

			if tt.name == "Valid job ID" {
				// Insert a completed job into the globalJobStore before making the request
				globalJobStore.AddJob(&GenerationJob{
					ID:        tt.jobID,
					Status:    StatusCompleted,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				})
			}

			// Create request
			req := httptest.NewRequest("GET", "/generation-status/"+tt.jobID, nil)
			w := httptest.NewRecorder()

			// Make request using the router
			router.ServeHTTP(w, req)

			// Assert response
			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedBody, response)
		})
	}
}
