package api

import (
	"bytes" // Required for capturing Synthea output in the new structure
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// --- JOB MANAGEMENT ---

// JobStatus represents the status of a generation job
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
)

// GenerationJob holds information about an asynchronous Synthea generation task
type GenerationJob struct {
	ID            string
	Status        JobStatus
	RequestParams SyntheaParams // Store the original request params
	Result        map[string]interface{}
	Error         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// JobStore holds all the jobs, protected by a mutex
type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*GenerationJob
}

// Global job store (or inject it into your Api struct if preferred for testing/scoping)
var globalJobStore = &JobStore{
	jobs: make(map[string]*GenerationJob),
}

func newJobID() string {
	bytesArr := make([]byte, 16) // Renamed from 'bytes' to avoid conflict
	if _, err := rand.Read(bytesArr); err != nil {
		// Fallback for exceedingly rare error
		return time.Now().Format(time.RFC3339Nano) + hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(bytesArr)
}

// --- END JOB MANAGEMENT ---

// KeepCriterion defines a single criterion for the "Keep Modules".
type KeepCriterion struct {
	System  string `json:"system"`            // e.g., "SNOMED-CT", "RxNorm"
	Code    string `json:"code"`              // e.g., "10029008"
	Display string `json:"display,omitempty"` // e.g., "Suicide precautions"
}

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

	// New fields for Keep Modules
	KeepActiveConditions  []KeepCriterion `json:"keepActiveConditions,omitempty"`
	KeepActiveAllergies   []KeepCriterion `json:"keepActiveAllergies,omitempty"`
	KeepActiveProcedures  []KeepCriterion `json:"keepActiveProcedures,omitempty"`
	KeepActiveMedications []KeepCriterion `json:"keepActiveMedications,omitempty"`
	KeepLogic             *string         `json:"keepLogic,omitempty"`    // "AND" or "OR" for keep criteria
	OutputFormat          *string         `json:"outputFormat,omitempty"` // "fhir", "ccda", "csv"

	// New field for Custom Modules (from Module Builder, etc.)
	// These are paths or URLs to Synthea module JSON files.
	CustomModules []string `json:"customModules,omitempty"`
}

// --- Structs for building the Synthea Module JSON ---

// ModuleCode represents a single code within a condition.
type ModuleCode struct {
	System  string `json:"system"`
	Code    string `json:"code"`
	Display string `json:"display,omitempty"`
}

// ModuleCondition represents one condition within a ConditionBlock (e.g., "Active Allergy").
type ModuleCondition struct {
	ConditionType string       `json:"condition_type"`
	Codes         []ModuleCode `json:"codes"`
}

// ConditionBlock represents the "condition" block in a ConditionalTransition.
type ConditionBlock struct {
	ConditionType string            `json:"condition_type"` // e.g., "And", "Or"
	Conditions    []ModuleCondition `json:"conditions"`
}

// ConditionalTransition defines a transition rule from a state.
type ConditionalTransition struct {
	Transition string          `json:"transition"`
	Condition  *ConditionBlock `json:"condition,omitempty"`
}

// ModuleState represents a state in the Synthea module.
type ModuleState struct {
	Type                   string                  `json:"type"`
	Name                   string                  `json:"name,omitempty"` // As seen in your example
	DirectTransition       string                  `json:"direct_transition,omitempty"`
	ConditionalTransitions []ConditionalTransition `json:"conditional_transition,omitempty"`
	// Remove 'Allow' if using ConditionalTransitions primarily for logic like the example
}

// SyntheaModule is the root structure for a generic Synthea module JSON.
type SyntheaModule struct {
	Name       string                 `json:"name"`
	Remarks    []string               `json:"remarks,omitempty"`
	States     map[string]ModuleState `json:"states"`
	GMFVersion *int                   `json:"gmf_version,omitempty"` // Pointer to allow omission
}

func (api *Api) RunSyntheaGeneration(w http.ResponseWriter, r *http.Request) {
	var params SyntheaParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, "Invalid request payload: "+err.Error(), http.StatusBadRequest)
		log.Printf("Error decoding Synthea params payload: %v", err)
		return
	}
	defer r.Body.Close() // Ensure body is closed

	jobID := newJobID()
	job := &GenerationJob{
		ID:            jobID,
		Status:        StatusPending,
		RequestParams: params,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	globalJobStore.mu.Lock()
	globalJobStore.jobs[jobID] = job
	globalJobStore.mu.Unlock()

	log.Printf("Job %s created for Synthea generation.", jobID)
	go api.processSyntheaJob(job) // Process in background

	response := map[string]interface{}{
		"jobID":     jobID,
		"status":    string(StatusPending),
		"message":   "Synthea generation request accepted. Check status using the jobID.",
		"statusUrl": fmt.Sprintf("%s/generation-status/%s", r.Host, jobID), // Construct a helpful URL
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted) // 202 Accepted
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Job %s: Error encoding acceptance response: %v", jobID, err)
	}
}

func (api *Api) processSyntheaJob(job *GenerationJob) {
	globalJobStore.mu.Lock()
	job.Status = StatusRunning
	job.UpdatedAt = time.Now()
	globalJobStore.mu.Unlock()
	log.Printf("Job %s: Status changed to %s", job.ID, StatusRunning)

	// --- Start of moved logic from original RunSyntheaGeneration ---
	params := job.RequestParams // Use parameters from the job

	syntheaJarPath := "/app/synthea-with-dependencies.jar" // Consider making this configurable
	if _, err := os.Stat(syntheaJarPath); os.IsNotExist(err) {
		log.Printf("Job %s: Synthea JAR not found at %s", job.ID, syntheaJarPath)
		globalJobStore.mu.Lock()
		job.Status = StatusFailed
		job.Error = "Synthea JAR not found at " + syntheaJarPath
		job.UpdatedAt = time.Now()
		globalJobStore.mu.Unlock()
		return
	}

	mainOutputDir := filepath.Join(os.TempDir(), "synthea_job_"+job.ID)
	if err := os.MkdirAll(mainOutputDir, 0755); err != nil {
		log.Printf("Job %s: Failed to create execution working directory %s: %v", job.ID, mainOutputDir, err)
		globalJobStore.mu.Lock()
		job.Status = StatusFailed
		job.Error = "Failed to create execution working directory: " + err.Error()
		job.UpdatedAt = time.Now()
		globalJobStore.mu.Unlock()
		return
	}
	defer os.RemoveAll(mainOutputDir)
	log.Printf("Job %s: Created working directory %s", job.ID, mainOutputDir)

	requestedOutputFormat := "fhir"
	if params.OutputFormat != nil && *params.OutputFormat != "" {
		validFormats := map[string]bool{"fhir": true, "ccda": true, "csv": true}
		if _, isValid := validFormats[strings.ToLower(*params.OutputFormat)]; isValid {
			requestedOutputFormat = strings.ToLower(*params.OutputFormat)
		} else {
			log.Printf("Job %s: Warning: Invalid outputFormat '%s' requested. Defaulting to 'fhir'.", job.ID, *params.OutputFormat)
		}
	}
	log.Printf("Job %s: Using output format: %s", job.ID, requestedOutputFormat)

	args := []string{"-Dgenerate.max_attempts_to_keep_patient=5000", "-jar", syntheaJarPath}
	args = append(args, fmt.Sprintf("--exporter.%s.export=true", requestedOutputFormat))

	var moduleConditions []ModuleCondition
	addCriteria := func(criteria []KeepCriterion, syntheaConditionType string) {
		for _, c := range criteria {
			if c.Code == "" || c.System == "" {
				log.Printf("Job %s: Skipping invalid KeepCriterion for %s: %+v", job.ID, syntheaConditionType, c)
				continue
			}
			moduleConditions = append(moduleConditions, ModuleCondition{
				ConditionType: syntheaConditionType,
				Codes: []ModuleCode{{
					System:  c.System,
					Code:    c.Code,
					Display: c.Display,
				}},
			})
		}
	}

	addCriteria(params.KeepActiveConditions, "Active Condition")
	addCriteria(params.KeepActiveAllergies, "Active Allergy")
	addCriteria(params.KeepActiveProcedures, "Procedure")
	addCriteria(params.KeepActiveMedications, "Active Medication")

	if len(moduleConditions) > 0 {
		gmfVersion := 2
		keepLogicConditionType := "And"
		if params.KeepLogic != nil {
			if strings.ToUpper(*params.KeepLogic) == "OR" {
				keepLogicConditionType = "Or"
			} else if strings.ToUpper(*params.KeepLogic) == "AND" {
				// Default is already "And", so this is fine
			} else {
				log.Printf("Job %s: Warning: Invalid value for keepLogic '%s'. Defaulting to 'And'.", job.ID, *params.KeepLogic)
			}
		}
		log.Printf("Job %s: Using '%s' logic for combining keep criteria.", job.ID, keepLogicConditionType)

		keepModule := SyntheaModule{
			Name:       "Medisynth Generated Keep Module for Job " + job.ID,
			GMFVersion: &gmfVersion,
			States: map[string]ModuleState{
				"Initial": {
					Type: "Initial", Name: "Initial",
					ConditionalTransitions: []ConditionalTransition{
						{
							Transition: "Keep",
							Condition: &ConditionBlock{
								ConditionType: keepLogicConditionType,
								Conditions:    moduleConditions,
							},
						},
						{Transition: "Terminal"},
					},
				},
				"Keep":     {Type: "Terminal", Name: "Keep"},
				"Terminal": {Type: "Terminal", Name: "Terminal"},
			},
		}
		keepModuleFilePath := filepath.Join(mainOutputDir, "medisynth_generated_keep_module.json")
		keepModuleFile, err := os.Create(keepModuleFilePath)
		if err != nil {
			log.Printf("Job %s: Error creating keep module file %s: %v", job.ID, keepModuleFilePath, err)
			globalJobStore.mu.Lock()
			job.Status = StatusFailed
			job.Error = "Failed to create keep module file: " + err.Error()
			job.UpdatedAt = time.Now()
			globalJobStore.mu.Unlock()
			return
		}
		encoder := json.NewEncoder(keepModuleFile)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(keepModule); err != nil {
			keepModuleFile.Close()
			log.Printf("Job %s: Error encoding keep module JSON to %s: %v", job.ID, keepModuleFilePath, err)
			globalJobStore.mu.Lock()
			job.Status = StatusFailed
			job.Error = "Failed to encode keep module JSON: " + err.Error()
			job.UpdatedAt = time.Now()
			globalJobStore.mu.Unlock()
			return
		}
		if err := keepModuleFile.Close(); err != nil {
			log.Printf("Job %s: Error closing keep module file %s: %v", job.ID, keepModuleFilePath, err)
			globalJobStore.mu.Lock()
			job.Status = StatusFailed
			job.Error = "Failed to close keep module file: " + err.Error()
			job.UpdatedAt = time.Now()
			globalJobStore.mu.Unlock()
			return
		}
		args = append(args, "-k", keepModuleFilePath)
		log.Printf("Job %s: Generated Keep Module for -k flag at: %s", job.ID, keepModuleFilePath)
	}

	if len(params.CustomModules) > 0 {
		for _, modulePathOrURL := range params.CustomModules {
			if strings.TrimSpace(modulePathOrURL) != "" {
				args = append(args, "-m", strings.TrimSpace(modulePathOrURL))
				log.Printf("Job %s: Adding custom module: %s", job.ID, strings.TrimSpace(modulePathOrURL))
			}
		}
	}

	if params.Population != nil {
		args = append(args, "-p", strconv.Itoa(*params.Population))
	} else { // Default population if not provided
		args = append(args, "-p", "1")
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

	log.Printf("Job %s: Executing Synthea with command: java %s (in CWD: %s)", job.ID, strings.Join(args, " "), mainOutputDir)

	cmd := exec.Command("java", args...)
	cmd.Dir = mainOutputDir
	var stdOut, stdErr bytes.Buffer // Use bytes.Buffer for more control
	cmd.Stdout = &stdOut
	cmd.Stderr = &stdErr

	if err := cmd.Run(); err != nil {
		log.Printf("Job %s: Synthea stdout: %s", job.ID, stdOut.String())
		log.Printf("Job %s: Synthea stderr: %s", job.ID, stdErr.String())
		log.Printf("Job %s: Error running Synthea (CWD: %s): %v.", job.ID, mainOutputDir, err)
		globalJobStore.mu.Lock()
		job.Status = StatusFailed
		job.Error = "Failed to run Synthea: " + err.Error() + ". Stderr: " + stdErr.String()
		job.UpdatedAt = time.Now()
		globalJobStore.mu.Unlock()
		return
	}
	log.Printf("Job %s: Synthea run completed successfully. Stdout: %s", job.ID, stdOut.String())
	if stdErr.Len() > 0 {
		log.Printf("Job %s: Synthea stderr (run was successful but stderr had content): %s", job.ID, stdErr.String())
	}

	syntheaOutputLines := strings.Split(stdOut.String(), "\n")
	var patientSummaries []string
	for _, line := range syntheaOutputLines {
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) > 0 && strings.Contains(trimmedLine, "--") {
			parts := strings.SplitN(trimmedLine, "--", 2)
			if len(parts) == 2 {
				_, err := strconv.Atoi(strings.TrimSpace(parts[0]))
				if err == nil {
					patientSummaries = append(patientSummaries, trimmedLine)
					log.Printf("Job %s: Extracted patient summary line: %s", job.ID, trimmedLine)
				}
			}
		}
	}

	finalResponse := make(map[string]interface{})
	finalResponse["patientSummaries"] = patientSummaries
	finalResponse["outputFormatUsed"] = requestedOutputFormat

	if len(patientSummaries) == 0 {
		finalResponse["message"] = "Synthea ran, but no patient summary lines found in stdout. Output files might still be generated."
		log.Printf("Job %s: Could not extract any patient summary lines from Synthea stdout.", job.ID)
	}

	syntheaActualOutputSubDir := filepath.Join(mainOutputDir, "output", requestedOutputFormat)
	outputGenerated := false
	if _, err := os.Stat(syntheaActualOutputSubDir); os.IsNotExist(err) {
		finalResponse["outputGenerated"] = "false"
		if len(patientSummaries) == 0 {
			log.Printf("Job %s: Synthea's expected output directory does not exist: %s", job.ID, syntheaActualOutputSubDir)
			// api.logFilesInDirectory(mainOutputDir) // logFilesInDirectory is a method on Api, can't call directly here without 'api' instance
		}
	} else {
		finalResponse["outputGenerated"] = "true"
		outputGenerated = true
		log.Printf("Job %s: Output directory found at: %s. Extracted %d patient summaries.", job.ID, syntheaActualOutputSubDir, len(patientSummaries))
	}

	var outputFileContentList []interface{}
	if outputGenerated && len(patientSummaries) > 0 {
		files, err := os.ReadDir(syntheaActualOutputSubDir)
		if err != nil {
			log.Printf("Job %s: Error reading output directory %s for content extraction: %v", job.ID, syntheaActualOutputSubDir, err)
		} else {
			if len(patientSummaries) == 1 {
				foundPatientFile := false
				for _, file := range files {
					if file.IsDir() {
						continue
					}
					fileName := file.Name()
					filePath := filepath.Join(syntheaActualOutputSubDir, fileName)
					fileBytes, readErr := os.ReadFile(filePath)
					if readErr != nil {
						log.Printf("Job %s: Error reading potential output file %s: %v", job.ID, filePath, readErr)
						continue
					}
					if requestedOutputFormat == "fhir" {
						if strings.HasSuffix(fileName, ".json") && fileName != "practitionerInformation.json" && fileName != "hospitalInformation.json" && fileName != "payerInformation.json" {
							var bundleData interface{}
							if unmarshalErr := json.Unmarshal(fileBytes, &bundleData); unmarshalErr == nil {
								if bundleMap, ok := bundleData.(map[string]interface{}); ok {
									if resourceType, ok := bundleMap["resourceType"].(string); ok && resourceType == "Bundle" {
										outputFileContentList = append(outputFileContentList, bundleData)
										log.Printf("Job %s: Added FHIR bundle from file: %s to response", job.ID, fileName)
										foundPatientFile = true
										break
									}
								}
							}
						}
					} else if requestedOutputFormat == "ccda" {
						if strings.HasSuffix(fileName, ".xml") {
							outputFileContentList = append(outputFileContentList, string(fileBytes))
							log.Printf("Job %s: Added CCDA content from file: %s to response", job.ID, fileName)
							foundPatientFile = true
							break
						}
					} else if requestedOutputFormat == "csv" {
						if fileName == "patients.csv" {
							outputFileContentList = append(outputFileContentList, string(fileBytes))
							log.Printf("Job %s: Added CSV content from file: %s to response", job.ID, fileName)
							foundPatientFile = true
							break
						} else if strings.HasSuffix(fileName, ".csv") && !foundPatientFile {
							outputFileContentList = append(outputFileContentList, string(fileBytes))
							log.Printf("Job %s: Added CSV content (fallback) from file: %s to response", job.ID, fileName)
							foundPatientFile = true
						}
					}
				}
				if !foundPatientFile && requestedOutputFormat == "csv" && len(outputFileContentList) > 0 {
				} else if !foundPatientFile {
					log.Printf("Job %s: Could not find a suitable primary output file for the single patient summary in %s for format %s", job.ID, syntheaActualOutputSubDir, requestedOutputFormat)
				}
			} else if len(patientSummaries) > 1 {
				log.Printf("Job %s: Multiple patient summaries found (%d). Attempting to process for supported formats.", job.ID, len(patientSummaries))
				if requestedOutputFormat == "fhir" {
					log.Printf("Job %s: FHIR output for multiple patients: collecting all patient bundles.", job.ID)
					for _, file := range files {
						if file.IsDir() {
							continue
						}
						fileName := file.Name()
						if fileName == "practitionerInformation.json" || fileName == "hospitalInformation.json" || fileName == "payerInformation.json" {
							continue
						}
						if strings.HasSuffix(fileName, ".json") {
							filePath := filepath.Join(syntheaActualOutputSubDir, fileName)
							fileBytes, readErr := os.ReadFile(filePath)
							if readErr != nil {
								log.Printf("Job %s: Error reading potential FHIR bundle %s: %v", job.ID, filePath, readErr)
								continue
							}
							var bundleData interface{}
							if unmarshalErr := json.Unmarshal(fileBytes, &bundleData); unmarshalErr == nil {
								if bundleMap, ok := bundleData.(map[string]interface{}); ok {
									if resourceType, ok := bundleMap["resourceType"].(string); ok && resourceType == "Bundle" {
										outputFileContentList = append(outputFileContentList, bundleData)
										log.Printf("Job %s: Added FHIR bundle from file: %s to response list (multiple patients)", job.ID, fileName)
									}
								}
							} else {
								log.Printf("Job %s: Error unmarshalling potential FHIR bundle %s (multiple patients): %v", job.ID, filePath, unmarshalErr)
							}
						}
					}
				} else if requestedOutputFormat == "csv" {
					log.Printf("Job %s: CSV output for multiple patients: collecting all CSV files.", job.ID)
					for _, file := range files {
						if file.IsDir() {
							continue
						}
						fileName := file.Name()
						if strings.HasSuffix(fileName, ".csv") {
							filePath := filepath.Join(syntheaActualOutputSubDir, fileName)
							fileBytes, readErr := os.ReadFile(filePath)
							if readErr != nil {
								log.Printf("Job %s: Error reading CSV file %s: %v", job.ID, filePath, readErr)
								continue
							}
							outputFileContentList = append(outputFileContentList, map[string]string{"fileName": fileName, "content": string(fileBytes)})
							log.Printf("Job %s: Added CSV file to response object: %s (multiple patients)", job.ID, fileName)
						}
					}
				} else {
					log.Printf("Job %s: Automatic output file inclusion in response for format '%s' with multiple patients is not fully implemented. No file content will be added.", job.ID, requestedOutputFormat)
				}
			}
		}
	}

	if requestedOutputFormat == "csv" {
		if len(outputFileContentList) > 0 {
			finalResponse["outputFileContent"] = outputFileContentList
		} else {
			finalResponse["outputFileContent"] = nil
		}
	} else {
		if len(outputFileContentList) == 1 {
			finalResponse["outputFileContent"] = outputFileContentList[0]
		} else if len(outputFileContentList) > 0 {
			finalResponse["outputFileContent"] = outputFileContentList
		} else {
			finalResponse["outputFileContent"] = nil
		}
	}
	finalResponse["nameFormatExplanation"] = "Patient names (e.g., Alicia629) are generated by Synthea. The appended numbers are part of its synthetic data generation process to help ensure uniqueness or due to its naming algorithms."
	// --- End of moved logic ---

	globalJobStore.mu.Lock()
	job.Status = StatusCompleted
	job.Result = finalResponse
	job.UpdatedAt = time.Now()
	globalJobStore.mu.Unlock()
	log.Printf("Job %s: Status changed to %s", job.ID, StatusCompleted)
}

func (api *Api) GetGenerationStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID") // Use Chi's URLParam helper
	if jobID == "" {
		http.Error(w, "Missing jobID in path", http.StatusBadRequest)
		return
	}

	globalJobStore.mu.RLock()
	job, exists := globalJobStore.jobs[jobID]
	globalJobStore.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if !exists {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	response := make(map[string]interface{})
	response["jobID"] = job.ID
	response["status"] = string(job.Status)
	response["createdAt"] = job.CreatedAt.Format(time.RFC3339)
	response["updatedAt"] = job.UpdatedAt.Format(time.RFC3339)

	switch job.Status {
	case StatusCompleted:
		// Merge the job result into the status response
		for k, v := range job.Result {
			response[k] = v
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Job %s: Error encoding completed status response: %v", jobID, err)
		}
	case StatusFailed:
		response["error"] = job.Error
		// Consider returning a 500 or other appropriate status if it's a server failure
		w.WriteHeader(http.StatusOK) // Or http.StatusInternalServerError if the error implies server fault
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Job %s: Error encoding failed status response: %v", jobID, err)
		}
	case StatusPending, StatusRunning:
		w.Header().Set("Retry-After", "30") // Suggest client retries after 30 seconds
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Job %s: Error encoding pending/running status response: %v", jobID, err)
		}
	default:
		log.Printf("Job %s: Unknown job status encountered: %s", jobID, job.Status)
		http.Error(w, "Unknown job status", http.StatusInternalServerError)
	}
}

// Helper function to log files in a directory for debugging
func (api *Api) logFilesInDirectory(dirPath string) {
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
			// logFilesInDirectory(subDirPath) // This would need 'api' instance or be a static func
		}
	}
}

// Heartbeat responds with a 200 OK status.
func (api *Api) Heartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// RunContainerizedCode is a placeholder for running containerized code.
// Actual implementation of Docker-in-Docker or similar container execution
// is complex and has security implications. This is a basic placeholder.
func (api *Api) RunContainerizedCode(w http.ResponseWriter, r *http.Request) {
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
