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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/go-chi/chi/v5"
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
	cmd, args := args[3], args[4:] // os.Args[0] is test binary, os.Args[1] is -test.run, os.Args[2] is --

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
	outputDir := "." // Current dir of the mocked command
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

func TestRunSyntheaGenerationHandler(t *testing.T) {
	cfg := config.Config{APIPort: 8080}
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
	cfg := config.Config{APIPort: 8080}
	apiInstance, _ := NewApi(cfg) // apiInstance not directly used by processSyntheaJob but good for consistency

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
			CreatedAt:     time.Now(),
		}
		globalJobStore.mu.Lock()
		globalJobStore.jobs[jobID] = job
		globalJobStore.mu.Unlock()

		// The mock TestExecCmdHelper needs to know where to "create" output.
		// This is the hard part. The mock runs as a separate process.
		// We can instruct the mock via environment variables or specific args.
		// For this test, the mock will just produce stdout.
		// The test needs to ensure the output directory structure is created by processSyntheaJob
		// and then simulate Synthea placing files there.

		// Create a temporary directory that processSyntheaJob will use
		// and that our mock might (if it were more complex) write to.
		expectedJobDir := filepath.Join(os.TempDir(), "synthea_job_"+jobID)
		// defer os.RemoveAll(expectedJobDir) // processSyntheaJob should clean this up

		// Simulate Synthea creating an output file
		// This needs to happen *after* processSyntheaJob creates expectedJobDir/output/fhir
		// This is tricky to coordinate with the mocked exec.Command.
		// A simpler test: mock exec.Command to write to stdout, and then this test
		// manually creates the expected file structure and content *before* asserting.

		go apiInstance.processSyntheaJob(job) // Run in goroutine like real scenario

		// Wait for job to complete (with timeout)
		success := waitForJobStatus(jobID, StatusCompleted, 5*time.Second)
		if !success {
			t.Fatalf("Job %s did not complete successfully in time. Status: %s, Error: %s", jobID, job.Status, job.Error)
		}

		globalJobStore.mu.RLock()
		completedJob := globalJobStore.jobs[jobID]
		globalJobStore.mu.RUnlock()

		if completedJob.Status != StatusCompleted {
			t.Errorf("Expected job status %s, got %s. Error: %s", StatusCompleted, completedJob.Status, completedJob.Error)
		}
		if completedJob.Error != "" {
			t.Errorf("Expected no error, got: %s", completedJob.Error)
		}
		if completedJob.Result == nil {
			t.Fatal("Expected non-nil result for completed job")
		}
		summaries, _ := completedJob.Result["patientSummaries"].([]string)
		if len(summaries) == 0 || !strings.Contains(summaries[0], "Test Patient") {
			t.Errorf("Expected patient summary from mock, got %v", summaries)
		}
		// To test outputFileContent, we'd need the mock to create files,
		// or this test would create them in the expected temp dir.
		// For now, we rely on the mock's stdout.
	})

	t.Run("SyntheaRunFailure", func(t *testing.T) {
		resetGlobalJobStore()
		setupExecMock()
		defer teardownExecMock()

		jobID := "testjob_fail_synthea"
		// Pass a special arg to tell the mock to fail
		// This requires the mock to parse its args.
		// A simpler way is to set an environment variable that the mock checks.
		// For this example, let's assume an arg like "fail_synthea_run"
		// would be part of the SyntheaParams that translates to such an arg.
		// Or, more simply, the mock is hardcoded for this test run.
		// Let's modify the mock to check an env var for failure.
		t.Setenv("MOCK_SYNTHEA_FAIL", "1")
		defer t.Setenv("MOCK_SYNTHEA_FAIL", "")

		// Modify the helperCommand to look for MOCK_SYNTHEA_FAIL
		// This means TestExecCmdHelper needs to be more dynamic.
		// For simplicity, let's assume the mock is set up to fail for this run.
		// The current mock uses "fail_synthea_run" in args.
		// We need to ensure our SyntheaParams would lead to this.
		// This is where mocking gets complex.
		// A simpler mock for this specific test:
		execCommand = func(ctx context.Context, command string, args ...string) *exec.Cmd {
			// Always fail for this test case
			cs := []string{"-test.run=TestExecCmdHelper", "--", command, "fail_synthea_run"} // Force fail arg
			cmd := exec.CommandContext(ctx, os.Args[0], cs...)
			cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
			return cmd
		}

		params := SyntheaParams{Population: Pint(1)} // No special params needed if mock is forced to fail
		job := &GenerationJob{ID: jobID, Status: StatusPending, RequestParams: params}
		globalJobStore.mu.Lock()
		globalJobStore.jobs[jobID] = job
		globalJobStore.mu.Unlock()

		go apiInstance.processSyntheaJob(job)
		success := waitForJobStatus(jobID, StatusFailed, 5*time.Second)
		if !success {
			t.Fatalf("Job %s did not fail in time. Status: %s", jobID, job.Status)
		}

		globalJobStore.mu.RLock()
		failedJob := globalJobStore.jobs[jobID]
		globalJobStore.mu.RUnlock()

		if failedJob.Status != StatusFailed {
			t.Errorf("Expected job status %s, got %s", StatusFailed, failedJob.Status)
		}
		if failedJob.Error == "" {
			t.Error("Expected an error message for failed job")
		}
		if !strings.Contains(failedJob.Error, "Synthea simulated error") {
			t.Errorf("Expected error to contain 'Synthea simulated error', got '%s'", failedJob.Error)
		}
	})

	// TODO: Add tests for Keep Module generation logic
	// TODO: Add tests for CSV and CCDA output processing
	// TODO: Add test for CustomModules
}

func TestGetGenerationStatusHandler(t *testing.T) {
	cfg := config.Config{APIPort: 8080}
	apiInstance, _ := NewApi(cfg)
	router := chi.NewRouter() // Need a router for chi.URLParam
	router.Get("/generation-status/{jobID}", apiInstance.GetGenerationStatus)

	// Suppress log output during tests
	originalLogger := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalLogger)

	t.Run("JobNotFound", func(t *testing.T) {
		resetGlobalJobStore()
		req := httptest.NewRequest("GET", "/generation-status/nonexistentjob", nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req) // Use the router to serve

		if rr.Code != http.StatusNotFound {
			t.Errorf("Expected status %d for non-existent job, got %d", http.StatusNotFound, rr.Code)
		}
	})

	t.Run("JobCompleted", func(t *testing.T) {
		resetGlobalJobStore()
		jobID := "completedjob"
		completedJob := &GenerationJob{
			ID:        jobID,
			Status:    StatusCompleted,
			Result:    map[string]interface{}{"data": "patient data"},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		globalJobStore.mu.Lock()
		globalJobStore.jobs[jobID] = completedJob
		globalJobStore.mu.Unlock()

		req := httptest.NewRequest("GET", "/generation-status/"+jobID, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status %d for completed job, got %d", http.StatusOK, rr.Code)
		}
		var respBody map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &respBody); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}
		if respBody["status"] != string(StatusCompleted) {
			t.Errorf("Expected status '%s', got '%s'", StatusCompleted, respBody["status"])
		}
		if respBody["data"] != "patient data" {
			t.Errorf("Expected data 'patient data', got '%v'", respBody["data"])
		}
	})

	// TODO: Add tests for Pending, Running, Failed statuses
}

func TestHeartbeatHandler(t *testing.T) {
	cfg := config.Config{APIPort: 8080}
	apiInstance, _ := NewApi(cfg)

	req := httptest.NewRequest("GET", "/heartbeat", nil)
	rr := httptest.NewRecorder()
	apiInstance.Heartbeat(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}
	var respBody map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("Failed to unmarshal heartbeat response: %v", err)
	}
	if respBody["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", respBody["status"])
	}
}

func TestNewJobID(t *testing.T) {
	id1 := newJobID()
	id2 := newJobID()

	if id1 == "" {
		t.Error("newJobID returned an empty string")
	}
	if len(id1) != 32 { // 16 bytes * 2 hex chars
		t.Errorf("Expected job ID length 32, got %d", len(id1))
	}
	if id1 == id2 {
		t.Error("newJobID returned identical IDs on sequential calls")
	}
}

// Helper for pointer to string
func Pstr(s string) *string { return &s }

// Helper for pointer to int
func Pint(i int) *int { return &i }

// waitForJobStatus polls for a job to reach a desired status or timeout
func waitForJobStatus(jobID string, desiredStatus JobStatus, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond) // Poll frequently
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done(): // Timeout
			globalJobStore.mu.RLock()
			job, exists := globalJobStore.jobs[jobID]
			status := StatusPending
			if exists {
				status = job.Status
			}
			globalJobStore.mu.RUnlock()
			log.Printf("Timeout waiting for job %s to reach status %s. Current status: %s", jobID, desiredStatus, status)
			return false
		case <-ticker.C:
			globalJobStore.mu.RLock()
			job, exists := globalJobStore.jobs[jobID]
			globalJobStore.mu.RUnlock()

			if exists && job.Status == desiredStatus {
				return true
			}
			if exists && (job.Status == StatusFailed && desiredStatus != StatusFailed) {
				log.Printf("Job %s reached status Failed while waiting for %s. Error: %s", jobID, desiredStatus, job.Error)
				return false // Job failed while we were waiting for something else
			}
		}
	}
}

// This ensures the TestExecCmdHelper is compiled into the test binary.
// It's a common pattern for testing exec.Command.
func TestMain(m *testing.M) {
	// Check if we are in the helper process
	for _, arg := range os.Args {
		if arg == "-test.run=TestExecCmdHelper" && os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
			// This call will be handled by TestExecCmdHelper itself.
			// It will call os.Exit, so m.Run() won't be reached in the helper.
			break
		}
	}
	os.Exit(m.Run())
}
