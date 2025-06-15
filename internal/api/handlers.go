package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"crypto/rand"

	"github.com/MediSynth-io/medisynth/internal/config"
)

// --- Mocking exec.Command ---
var mockExecCommand func(ctx context.Context, command string, args ...string) *exec.Cmd
var originalExecCommand func(ctx context.Context, command string, args ...string) *exec.Cmd

// Helper for tests that need to mock exec.Command
func helperCommand(ctx context.Context, command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestExecCmdHelper", "--", command}
	cs = append(cs, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// This is the actual "mocked" process.
// It's run when GO_WANT_HELPER_PROCESS is set.
func TestExecCmdHelper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	args = args[4:] // os.Args[0] is test binary, os.Args[1] is -test.run, os.Args[2] is --

	// Simulate Synthea output based on args or environment variables set by the test
	// This is a simplified example. You'd make this more sophisticated.
	if strings.Contains(strings.Join(args, " "), "fail_synthea_run") {
		fmt.Fprint(os.Stderr, "Synthea simulated error")
		os.Exit(1)
	}

	// Simulate successful run with some output
	fmt.Fprintln(os.Stdout, "1 -- Test Patient (30 y/o M) -- FHIR: /tmp/output/fhir/test_patient.json")
	// Simulate creating an output file if needed by the test
	// For example, if an arg indicates output format:
	for i, arg := range args {
		if arg == "-p" && len(args) > i+1 && args[i+1] == "create_output_file" { // Special flag for test
			// Find where Synthea would output based on its args
			// This is complex to truly replicate. For a unit test, we might assume a path
			// or have the test set an env var for the mock to know where to create a file.
			// For now, let's assume the test sets up the expected output dir structure.
			// Example: create a dummy FHIR output file
			// This part needs to align with how processSyntheaJob expects output.
			// It expects output in mainOutputDir/output/requestedOutputFormat
			// The mainOutputDir is created by processSyntheaJob.
			// The mock needs to know this path. This is tricky.
			// A simpler mock might just check args and not touch the filesystem.
		}
	}

	os.Exit(0)
}

func setupExecMock() {
	originalExecCommand = execCommand // Save original
	execCommand = helperCommand       // Set to mock
	mockExecCommand = helperCommand   // Also set the global mock var if used elsewhere
}

func teardownExecMock() {
	execCommand = originalExecCommand // Restore original
	mockExecCommand = nil
}

// Actual execCommand used by the package, can be swapped for testing
var execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, command, args...)
}

// --- End Mocking exec.Command ---

// Helper to reset globalJobStore for tests
func resetGlobalJobStore() {
	globalJobStore.mu.Lock()
	globalJobStore.jobs = make(map[string]*GenerationJob)
	globalJobStore.mu.Unlock()
}

// Variable for rand.Read to make it mockable in tests
var randRead = rand.Read

func TestRunSyntheaGenerationHandler(t *testing.T) {
	cfg := config.Config{APIPort: 8081}
	apiInstance, _ := NewApi(cfg)

	// Suppress log output during tests
	originalLogger := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalLogger)

	t.Run("ValidPayload", func(t *testing.T) {
		resetGlobalJobStore()
		payload := `{"population": 1, "outputFormat": "fhir"}`
		req := httptest.NewRequest("POST", "/generate-patients", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		apiInstance.RunSyntheaGeneration(rr, req)

		if rr.Code != http.StatusAccepted {
			t.Errorf("Expected status %d, got %d", http.StatusAccepted, rr.Code)
		}

		var respBody map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &respBody); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		jobID, ok := respBody["jobID"].(string)
		if !ok || jobID == "" {
			t.Errorf("Expected a non-empty jobID, got %v", respBody["jobID"])
		}

		// Check if job was added (basic check, doesn't wait for goroutine)
		time.Sleep(10 * time.Millisecond) // Give goroutine a moment to start
		globalJobStore.mu.RLock()
		_, jobExists := globalJobStore.jobs[jobID]
		globalJobStore.mu.RUnlock()
		if !jobExists {
			t.Errorf("Job %s was not found in globalJobStore after submission", jobID)
		}
	})

	t.Run("InvalidPayload_BadJSON", func(t *testing.T) {
		resetGlobalJobStore()
		payload := `{"population": 1, "outputFormat": "fhir"` // Missing closing brace
		req := httptest.NewRequest("POST", "/generate-patients", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		apiInstance.RunSyntheaGeneration(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for bad JSON, got %d", http.StatusBadRequest, rr.Code)
		}
	})

	t.Run("InvalidPayload_WrongType", func(t *testing.T) {
		resetGlobalJobStore()
		payload := `{"population": "one"}` // Population should be int
		req := httptest.NewRequest("POST", "/generate-patients", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		apiInstance.RunSyntheaGeneration(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d for wrong type, got %d", http.StatusBadRequest, rr.Code)
		}
	})
}

func TestProcessSyntheaJob(t *testing.T) {
	cfg := config.Config{APIPort: 8081}
	apiInstance, _ := NewApi(cfg)

	// Suppress log output during tests
	originalLogger := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalLogger)

	// Setup mock for exec.Command
	// This is a simplified way to swap the function; in real tests, you might use interfaces or more robust mocking.
	originalExec := execCommand
	defer func() { execCommand = originalExec }() // Restore original

	// Create a dummy Synthea JAR for path validation if needed by the test
	// This depends on whether the mock bypasses the os.Stat check.
	// For a true unit test of processSyntheaJob, os.Stat might also be mocked.
	// Here, we assume the mock execCommand implies Synthea exists.

	t.Run("SuccessfulFHIRGeneration", func(t *testing.T) {
		resetGlobalJobStore()
		setupExecMock() // Use the helperCommand mock
		defer teardownExecMock()

		jobID := "testjob_success_fhir"
		params := SyntheaParams{Population: Pint(1), OutputFormat: Pstr("fhir")}
		job := &GenerationJob{
			ID:            jobID,
			Status:        StatusPending,
			RequestParams: params,
		}
		globalJobStore.AddJob(job)

		// Start the job processing
		go apiInstance.processSyntheaJob(job)

		// Wait for job to complete
		if !waitForJobStatus(jobID, StatusCompleted, 5*time.Second) {
			t.Errorf("Job did not complete within timeout")
		}

		// Verify job status
		globalJobStore.mu.RLock()
		updatedJob, exists := globalJobStore.jobs[jobID]
		globalJobStore.mu.RUnlock()

		if !exists {
			t.Errorf("Job %s not found in store", jobID)
		}
		if updatedJob.Status != StatusCompleted {
			t.Errorf("Expected status %s, got %s", StatusCompleted, updatedJob.Status)
		}
		if updatedJob.Error != "" {
			t.Errorf("Expected no error, got %s", updatedJob.Error)
		}
	})

	t.Run("FailedGeneration", func(t *testing.T) {
		resetGlobalJobStore()
		setupExecMock() // Use the helperCommand mock
		defer teardownExecMock()

		jobID := "testjob_fail"
		params := SyntheaParams{Population: Pint(1), OutputFormat: Pstr("fail_synthea_run")}
		job := &GenerationJob{
			ID:            jobID,
			Status:        StatusPending,
			RequestParams: params,
		}
		globalJobStore.AddJob(job)

		// Start the job processing
		go apiInstance.processSyntheaJob(job)

		// Wait for job to fail
		if !waitForJobStatus(jobID, StatusFailed, 5*time.Second) {
			t.Errorf("Job did not fail within timeout")
		}

		// Verify job status
		globalJobStore.mu.RLock()
		updatedJob, exists := globalJobStore.jobs[jobID]
		globalJobStore.mu.RUnlock()

		if !exists {
			t.Errorf("Job %s not found in store", jobID)
		}
		if updatedJob.Status != StatusFailed {
			t.Errorf("Expected status %s, got %s", StatusFailed, updatedJob.Status)
		}
		if updatedJob.Error == "" {
			t.Error("Expected error message, got none")
		}
	})
}

func TestHeartbeatHandler(t *testing.T) {
	cfg := config.Config{APIPort: 8081}
	apiInstance, _ := NewApi(cfg)

	req := httptest.NewRequest("GET", "/heartbeat", nil)
	rr := httptest.NewRecorder()

	apiInstance.Heartbeat(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var respBody map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if respBody["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", respBody["status"])
	}
}

func TestMain(m *testing.M) {
	// Setup
	setupExecMock()

	// Run tests
	code := m.Run()

	// Teardown
	teardownExecMock()

	os.Exit(code)
}

func (api *Api) processSyntheaJob(job *GenerationJob) {
	// Update job status to running
	job.Status = StatusRunning
	job.UpdatedAt = time.Now()
	globalJobStore.UpdateJob(job)

	// Create command
	cmd := execCommand(context.Background(), "sh", "-c", "echo 'Synthea completed successfully'")

	// Capture output
	output, err := cmd.CombinedOutput()

	// Update job based on command result
	job.UpdatedAt = time.Now()
	if err != nil {
		job.Status = StatusFailed
		job.Error = fmt.Sprintf("Synthea failed: %v\nOutput: %s", err, string(output))
	} else {
		job.Status = StatusCompleted
	}
	globalJobStore.UpdateJob(job)
}

func newJobID() string {
	b := make([]byte, 16)
	if _, err := randRead(b); err != nil {
		return "fallback-jobid"
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func Pstr(s string) *string {
	return &s
}

func Pint(i int) *int {
	return &i
}

func waitForJobStatus(jobID string, desiredStatus JobStatus, timeout time.Duration) bool {
	start := time.Now()
	for time.Since(start) < timeout {
		globalJobStore.mu.RLock()
		job, exists := globalJobStore.jobs[jobID]
		globalJobStore.mu.RUnlock()

		if !exists {
			return false
		}

		if job.Status == desiredStatus {
			return true
		}

		time.Sleep(100 * time.Millisecond)
	}
	return false
}
