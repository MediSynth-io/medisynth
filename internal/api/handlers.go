package api

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

	// Determine output format
	requestedOutputFormat := "fhir" // Default
	if params.OutputFormat != nil && *params.OutputFormat != "" {
		validFormats := map[string]bool{"fhir": true, "ccda": true, "csv": true}
		if _, isValid := validFormats[strings.ToLower(*params.OutputFormat)]; isValid {
			requestedOutputFormat = strings.ToLower(*params.OutputFormat)
		} else {
			log.Printf("Warning: Invalid outputFormat '%s' requested. Defaulting to 'fhir'.", *params.OutputFormat)
		}
	}
	log.Printf("Using output format: %s", requestedOutputFormat)

	args := []string{"-Dgenerate.max_attempts_to_keep_patient=5000", "-jar", syntheaJarPath}
	args = append(args, fmt.Sprintf("--exporter.%s.export=true", requestedOutputFormat))

	var moduleConditions []ModuleCondition // Changed from guardConditions
	addCriteria := func(criteria []KeepCriterion, syntheaConditionType string) {
		for _, c := range criteria {
			if c.Code == "" || c.System == "" { // Basic validation
				log.Printf("Skipping invalid KeepCriterion for %s: %+v", syntheaConditionType, c)
				continue
			}
			// Each criterion becomes a ModuleCondition
			moduleConditions = append(moduleConditions, ModuleCondition{
				ConditionType: syntheaConditionType, // "Active Allergy", "Active Condition", etc.
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

		// Determine the logic for combining keep criteria
		keepLogicConditionType := "And" // Default to "And"
		if params.KeepLogic != nil {
			if strings.ToUpper(*params.KeepLogic) == "OR" {
				keepLogicConditionType = "Or"
			} else if strings.ToUpper(*params.KeepLogic) == "AND" {
				keepLogicConditionType = "And"
			} else {
				log.Printf("Warning: Invalid value for keepLogic '%s'. Defaulting to 'And'.", *params.KeepLogic)
			}
		}
		log.Printf("Using '%s' logic for combining keep criteria.", keepLogicConditionType)

		keepModule := SyntheaModule{
			Name:       "Medisynth Generated Keep Module",
			GMFVersion: &gmfVersion, // Use gmf_version from your example
			States: map[string]ModuleState{
				"Initial": {
					Type: "Initial",
					Name: "Initial", // Match example
					ConditionalTransitions: []ConditionalTransition{
						{
							Transition: "Keep", // Target state if conditions met
							Condition: &ConditionBlock{
								ConditionType: keepLogicConditionType, // Use determined logic
								Conditions:    moduleConditions,
							},
						},
						{
							Transition: "Terminal", // Default transition if conditions not met
						},
					},
				},
				"Keep": { // State for patients meeting criteria
					Type: "Terminal",
					Name: "Keep", // Synthea -k flag looks for a state named "Keep"
				},
				"Terminal": { // State for patients not meeting criteria
					Type: "Terminal",
					Name: "Terminal",
				},
			},
		}

		keepModuleFilePath := filepath.Join(mainOutputDir, "medisynth_generated_keep_module.json")
		keepModuleFile, err := os.Create(keepModuleFilePath)
		if err != nil {
			http.Error(w, "Failed to create keep module file: "+err.Error(), http.StatusInternalServerError)
			log.Printf("Error creating keep module file %s: %v", keepModuleFilePath, err)
			return
		}
		// defer keepModuleFile.Close() // Will be closed explicitly before use

		encoder := json.NewEncoder(keepModuleFile)
		encoder.SetIndent("", "  ") // Pretty print JSON
		if err := encoder.Encode(keepModule); err != nil {
			keepModuleFile.Close()
			http.Error(w, "Failed to encode keep module JSON: "+err.Error(), http.StatusInternalServerError)
			log.Printf("Error encoding keep module JSON to %s: %v", keepModuleFilePath, err)
			return
		}
		if err := keepModuleFile.Close(); err != nil { // Ensure file is written and closed
			http.Error(w, "Failed to close keep module file: "+err.Error(), http.StatusInternalServerError)
			log.Printf("Error closing keep module file %s: %v", keepModuleFilePath, err)
			return
		}

		args = append(args, "-k", keepModuleFilePath) // Use -k flag
		log.Printf("Generated Keep Module for -k flag at: %s", keepModuleFilePath)
	}

	// Add custom modules
	if len(params.CustomModules) > 0 {
		for _, modulePathOrURL := range params.CustomModules {
			if strings.TrimSpace(modulePathOrURL) != "" {
				args = append(args, "-m", strings.TrimSpace(modulePathOrURL))
				log.Printf("Adding custom module: %s", strings.TrimSpace(modulePathOrURL))
			}
		}
	}
	// --- MODIFICATION END ---

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
	response["outputFormatUsed"] = requestedOutputFormat // Inform client of the format used

	if len(patientSummaries) == 0 {
		// Add a message only if no patient summaries were found in the Synthea output.
		response["message"] = "Synthea ran, but no patient summary lines found in stdout. Output files might still be generated."
		log.Printf("Could not extract any patient summary lines from Synthea stdout.")
	}

	// Always check and include FHIR output status
	syntheaActualOutputSubDir := filepath.Join(mainOutputDir, "output", requestedOutputFormat)
	outputGenerated := false
	if _, err := os.Stat(syntheaActualOutputSubDir); os.IsNotExist(err) {
		response["outputGenerated"] = "false"
		// Log details if no summaries AND no FHIR output found (potential issue)
		if len(patientSummaries) == 0 {
			log.Printf("Synthea's expected output directory does not exist: %s", syntheaActualOutputSubDir)
			api.logFilesInDirectory(mainOutputDir) // Corrected: Call method on api instance
		}
	} else {
		response["outputGenerated"] = "true"
		outputGenerated = true // Keep track for later use
		log.Printf("Output directory found at: %s. Extracted %d patient summaries.", syntheaActualOutputSubDir, len(patientSummaries))
	}

	// --- NEW: Attempt to include FHIR bundle in response ---
	var outputFileContentList []interface{} // Changed from fhirBundles to be more generic
	if outputGenerated && len(patientSummaries) > 0 {
		files, err := os.ReadDir(syntheaActualOutputSubDir)
		if err != nil {
			log.Printf("Error reading output directory %s for content extraction: %v", syntheaActualOutputSubDir, err)
		} else {
			// If one patient summary, try to find its bundle.
			// This heuristic assumes the first found FHIR Bundle file is the relevant one for a single patient.
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
						log.Printf("Error reading potential output file %s: %v", filePath, readErr)
						continue
					}

					if requestedOutputFormat == "fhir" {
						if strings.HasSuffix(fileName, ".json") && fileName != "practitionerInformation.json" && fileName != "hospitalInformation.json" && fileName != "payerInformation.json" {
							var bundleData interface{}
							if unmarshalErr := json.Unmarshal(fileBytes, &bundleData); unmarshalErr == nil {
								if bundleMap, ok := bundleData.(map[string]interface{}); ok {
									if resourceType, ok := bundleMap["resourceType"].(string); ok && resourceType == "Bundle" {
										outputFileContentList = append(outputFileContentList, bundleData)
										log.Printf("Added FHIR bundle from file: %s to response", fileName)
										foundPatientFile = true
										break
									}
								}
							}
						}
					} else if requestedOutputFormat == "ccda" {
						if strings.HasSuffix(fileName, ".xml") { // Basic check for CCDA
							outputFileContentList = append(outputFileContentList, string(fileBytes)) // Return XML as string
							log.Printf("Added CCDA content from file: %s to response", fileName)
							foundPatientFile = true
							break
						}
					} else if requestedOutputFormat == "csv" {
						// For CSV, let's prioritize patients.csv, or just grab the first .csv if not found.
						// A more robust solution might list all CSVs or zip them.
						if fileName == "patients.csv" {
							outputFileContentList = append(outputFileContentList, string(fileBytes)) // Return CSV content as string
							log.Printf("Added CSV content from file: %s to response", fileName)
							foundPatientFile = true
							break
						} else if strings.HasSuffix(fileName, ".csv") && !foundPatientFile {
							// Fallback: if patients.csv not yet found, take the first csv encountered
							// This part can be improved if specific CSV files are always desired.
							outputFileContentList = append(outputFileContentList, string(fileBytes))
							log.Printf("Added CSV content (fallback) from file: %s to response", fileName)
							foundPatientFile = true
							// Do not break here if patients.csv might still appear
						}
					}
				}
				if !foundPatientFile && requestedOutputFormat == "csv" && len(outputFileContentList) > 0 {
					// If we picked up a fallback CSV and patients.csv wasn't found, that's fine.
				} else if !foundPatientFile {
					log.Printf("Could not find a suitable primary output file for the single patient summary in %s for format %s", syntheaActualOutputSubDir, requestedOutputFormat)
				}
			} else if len(patientSummaries) > 1 { // Handling multiple patient summaries
				log.Printf("Multiple patient summaries found (%d). Attempting to process for supported formats.", len(patientSummaries))
				if requestedOutputFormat == "fhir" {
					log.Printf("FHIR output for multiple patients: collecting all patient bundles.")
					for _, file := range files {
						if file.IsDir() {
							continue
						}
						fileName := file.Name()
						// Skip known metadata files for FHIR
						if fileName == "practitionerInformation.json" || fileName == "hospitalInformation.json" || fileName == "payerInformation.json" {
							continue
						}
						if strings.HasSuffix(fileName, ".json") {
							filePath := filepath.Join(syntheaActualOutputSubDir, fileName)
							fileBytes, readErr := os.ReadFile(filePath)
							if readErr != nil {
								log.Printf("Error reading potential FHIR bundle %s: %v", filePath, readErr)
								continue
							}
							var bundleData interface{}
							if unmarshalErr := json.Unmarshal(fileBytes, &bundleData); unmarshalErr == nil {
								if bundleMap, ok := bundleData.(map[string]interface{}); ok {
									if resourceType, ok := bundleMap["resourceType"].(string); ok && resourceType == "Bundle" {
										outputFileContentList = append(outputFileContentList, bundleData)
										log.Printf("Added FHIR bundle from file: %s to response list (multiple patients)", fileName)
									}
								}
							} else {
								log.Printf("Error unmarshalling potential FHIR bundle %s (multiple patients): %v", filePath, unmarshalErr)
							}
						}
					}
				} else if requestedOutputFormat == "csv" {
					log.Printf("CSV output for multiple patients: collecting all CSV files.")
					for _, file := range files {
						if file.IsDir() {
							continue
						}
						fileName := file.Name()
						if strings.HasSuffix(fileName, ".csv") {
							filePath := filepath.Join(syntheaActualOutputSubDir, fileName)
							fileBytes, readErr := os.ReadFile(filePath)
							if readErr != nil {
								log.Printf("Error reading CSV file %s: %v", filePath, readErr)
								continue
							}
							// For CSV, add as a map with filename and content
							outputFileContentList = append(outputFileContentList, map[string]string{
								"fileName": fileName,
								"content":  string(fileBytes),
							})
							log.Printf("Added CSV file to response object: %s (multiple patients)", fileName)
						}
					}
				} else { // For other formats like CCDA with multiple patients
					log.Printf("Automatic output file inclusion in response for format '%s' with multiple patients is not fully implemented for individual file extraction. No file content will be added by this block.", requestedOutputFormat)
				}
			}
		}
	}
	// If only one file content was found, return it directly, otherwise return the list.
	// This simplifies the response for the common case of one patient, one primary file.
	// For CSV, we always want to return the list of file objects, even if it's just one.
	if requestedOutputFormat == "csv" {
		if len(outputFileContentList) > 0 {
			response["outputFileContent"] = outputFileContentList
		} else {
			response["outputFileContent"] = nil // Or an empty list: []interface{}{}
		}
	} else { // For non-CSV formats (like FHIR, CCDA)
		if len(outputFileContentList) == 1 {
			response["outputFileContent"] = outputFileContentList[0]
		} else if len(outputFileContentList) > 0 {
			response["outputFileContent"] = outputFileContentList
		} else {
			response["outputFileContent"] = nil
		}
	}

	response["nameFormatExplanation"] = "Patient names (e.g., Alicia629) are generated by Synthea. The appended numbers are part of its synthetic data generation process to help ensure uniqueness or due to its naming algorithms."

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding patient summaries response: %v", err)
	}
	// --- MODIFICATION END ---
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
			// logFilesInDirectory(subDirPath)
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
