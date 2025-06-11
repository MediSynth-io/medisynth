package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// SyntheaParams defines the expected parameters for generating Synthea data.
type SyntheaParams struct {
	Population    *int    `json:"population,omitempty"`
	State         *string `json:"state,omitempty"`
	City          *string `json:"city,omitempty"`
	Gender        *string `json:"gender,omitempty"` // M or F
	AgeMin        *int    `json:"ageMin,omitempty"`
	AgeMax        *int    `json:"ageMax,omitempty"`
	Seed          *int64  `json:"seed,omitempty"`
	ClinicianSeed *int64  `json:"clinicianSeed,omitempty"`
	ReferenceDate *string `json:"referenceDate,omitempty"` // YYYYMMDD
}

func RunSyntheaGeneration(w http.ResponseWriter, r *http.Request) {
	var params SyntheaParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		log.Printf("Error decoding Synthea params payload: %v", err)
		return
	}

	syntheaJarPath := "/app/synthea-with-dependencies.jar"
	if _, err := os.Stat(syntheaJarPath); os.IsNotExist(err) {
		http.Error(w, "Synthea JAR not found at "+syntheaJarPath, http.StatusInternalServerError)
		log.Printf("Synthea JAR not found at %s", syntheaJarPath)
		return
	}

	mainOutputDir := filepath.Join(os.TempDir(), "synthea_exec_wd_"+time.Now().Format("20060102150405"))
	if err := os.MkdirAll(mainOutputDir, 0755); err != nil {
		http.Error(w, "Failed to create execution working directory: "+err.Error(), http.StatusInternalServerError)
		log.Printf("Failed to create execution working directory %s: %v", mainOutputDir, err)
		return
	}
	defer os.RemoveAll(mainOutputDir)

	// We will set mainOutputDir as the working directory for Synthea.
	// Synthea by default creates an 'output' folder in its CWD.
	// So, we don't need to specify --exporter.base_directory if we set the CWD.
	// However, to be explicit and ensure FHIR export is on:
	args := []string{"-jar", syntheaJarPath, "--exporter.fhir.export=true"}
	// If setting CWD works, --exporter.base_directory might become redundant or could even conflict.
	// Let's try without it first if CWD is set. If that fails, we can add it back pointing to "." (relative to the new CWD).
	// For now, let's keep it simple:
	// args = append(args, "--exporter.base_directory=.") // This would mean CWD/./output/fhir -> CWD/output/fhir

	if params.Population != nil {
		args = append(args, "-p", strconv.Itoa(*params.Population))
	}
	if params.State != nil {
		args = append(args, *params.State)
		if params.City != nil {
			args = append(args, *params.City)
		}
	}
	if params.Gender != nil {
		args = append(args, "-g", *params.Gender)
	}
	if params.AgeMin != nil && params.AgeMax != nil {
		args = append(args, "-a", fmt.Sprintf("%d-%d", *params.AgeMin, *params.AgeMax))
	} else if params.AgeMin != nil {
		args = append(args, "-a", fmt.Sprintf("%d-", *params.AgeMin))
	} else if params.AgeMax != nil {
		args = append(args, "-a", fmt.Sprintf("-%d", *params.AgeMax))
	}

	if params.Seed != nil {
		args = append(args, "-s", strconv.FormatInt(*params.Seed, 10))
	}
	if params.ClinicianSeed != nil {
		args = append(args, "-cs", strconv.FormatInt(*params.ClinicianSeed, 10))
	}
	if params.ReferenceDate != nil {
		args = append(args, "-r", *params.ReferenceDate)
	}

	log.Printf("Executing Synthea with command: java %s (in CWD: %s)", strings.Join(args, " "), mainOutputDir)

	cmd := exec.Command("java", args...)
	cmd.Dir = mainOutputDir // Set the working directory for the command
	var stdOut, stdErr strings.Builder
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		log.Printf("Synthea stdout: %s", stdOut.String())
		log.Printf("Synthea stderr: %s", stdErr.String())
		http.Error(w, "Failed to run Synthea: "+err.Error()+". Check service logs.", http.StatusInternalServerError)
		log.Printf("Error running Synthea (CWD: %s): %v.", mainOutputDir, err)
		return
	}
	log.Printf("Synthea run completed successfully. Stdout: %s", stdOut.String())
	if stdErr.Len() > 0 {
		log.Printf("Synthea stderr (run was successful but stderr had content): %s", stdErr.String())
	}

	// --- MODIFICATION START: Extract all patient summaries from stdout ---
	syntheaOutputLines := strings.Split(stdOut.String(), "\n")
	var patientSummaries []string // Changed from string to []string
	for _, line := range syntheaOutputLines {
		// A simple check: Synthea patient summary lines typically start with a number and "--"
		// Example: "1 -- Yvone889 Paucek755 (26 y/o F) Boston, Massachusetts  (38194)"
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) > 0 && strings.Contains(trimmedLine, "--") {
			parts := strings.SplitN(trimmedLine, "--", 2)
			if len(parts) == 2 {
				_, err := strconv.Atoi(strings.TrimSpace(parts[0]))
				if err == nil { // Check if the part before "--" is a number
					patientSummaries = append(patientSummaries, trimmedLine) // Append to slice
					log.Printf("Extracted patient summary line: %s", trimmedLine)
					// Removed break to collect all summaries
				}
			}
		}
	}

	// Prepare the JSON response
	response := make(map[string]interface{}) // Use interface{} for mixed types

	// Always include patientSummaries in the response.
	// If no summaries were extracted, this will be an empty list: [].
	response["patientSummaries"] = patientSummaries

	if len(patientSummaries) == 0 {
		// Add a message only if no patient summaries were found in the Synthea output.
		response["message"] = "Synthea ran, but no patient summary lines found in stdout. FHIR files might still be generated."
		log.Printf("Could not extract any patient summary lines from Synthea stdout.")
	}
	// The previous if/else that conditionally added patientSummaries or a message is now simplified.

	// Always check and include FHIR output status
	syntheaActualFhirOutputDir := filepath.Join(mainOutputDir, "output", "fhir")
	if _, err := os.Stat(syntheaActualFhirOutputDir); os.IsNotExist(err) {
		response["fhirOutputGenerated"] = "false"
		// Log details if no summaries AND no FHIR output found (potential issue)
		if len(patientSummaries) == 0 {
			log.Printf("Synthea's expected FHIR output directory does not exist: %s", syntheaActualFhirOutputDir)
			logFilesInDirectory(mainOutputDir)
		}
	} else {
		response["fhirOutputGenerated"] = "true"
		log.Printf("FHIR output directory found at: %s. Extracted %d patient summaries.", syntheaActualFhirOutputDir, len(patientSummaries))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding patient summaries response: %v", err)
	}
	// --- MODIFICATION END ---
}

// Helper function to log files in a directory for debugging
func logFilesInDirectory(dirPath string) {
	log.Printf("Listing contents of directory: %s", dirPath)
	files, err := os.ReadDir(dirPath)
	if err != nil {
		log.Printf("Error reading directory %s: %v", dirPath, err)
		return
	}
	if len(files) == 0 {
		log.Printf("Directory %s is empty.", dirPath)
		return
	}
	for _, file := range files {
		log.Printf("Found in %s: %s (Is Directory: %t)", dirPath, file.Name(), file.IsDir())
		if file.IsDir() {
			// Optionally, recurse or list one level deeper if needed for debugging
			// subDirPath := filepath.Join(dirPath, file.Name())
			// logFilesInDirectory(subDirPath)
		}
	}
}

// Heartbeat responds with a 200 OK status.
func Heartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RunContainerizedCode is a placeholder for running containerized code.
// Actual implementation of Docker-in-Docker or similar container execution
// is complex and has security implications. This is a basic placeholder.
func RunContainerizedCode(w http.ResponseWriter, r *http.Request) {
	// For now, this endpoint will just acknowledge the request.
	// In a real scenario, you would parse the request, potentially authenticate,
	// and then interact with a container runtime or orchestration API.
	// This is a highly simplified example.

	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		log.Printf("Error decoding payload for /run-code: %v", err)
		return
	}

	log.Printf("Received request for /run-code with payload: %+v", payload)

	// Placeholder response
	response := map[string]interface{}{
		"message":          "Request to run containerized code received.",
		"status":           "pending",
		"details":          "Actual container execution is not yet implemented in this placeholder.",
		"received_payload": payload,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted is appropriate for async tasks
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response for /run-code: %v", err)
	}
}
