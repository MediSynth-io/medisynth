package models

import (
	"encoding/json"
	"fmt"
	"time"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// Job represents a patient generation job
type Job struct {
	ID             string                 `json:"id" db:"id"`
	UserID         string                 `json:"user_id" db:"user_id"`
	JobID          string                 `json:"job_id" db:"job_id"` // Synthea job ID
	Status         JobStatus              `json:"status" db:"status"`
	Parameters     map[string]interface{} `json:"parameters" db:"-"`
	ParametersJSON string                 `json:"-" db:"parameters"` // JSON storage
	OutputFormat   string                 `json:"output_format" db:"output_format"`
	OutputPath     *string                `json:"output_path" db:"output_path"`
	OutputSize     *int64                 `json:"output_size" db:"output_size"`
	PatientCount   *int                   `json:"patient_count" db:"patient_count"`
	ErrorMessage   *string                `json:"error_message" db:"error_message"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	CompletedAt    *time.Time             `json:"completed_at" db:"completed_at"`
}

// JobFile represents a file output from a generation job
type JobFile struct {
	ID       string `json:"id"`
	JobID    string `json:"job_id"`
	Filename string `json:"filename"`
	S3Key    string `json:"s3_key"`
	Size     int64  `json:"size"`
	URL      string `json:"url"` // Presigned download URL
}

// MarshalParameters converts the Parameters map to JSON for database storage
func (j *Job) MarshalParameters() error {
	if j.Parameters == nil {
		j.ParametersJSON = "{}"
		return nil
	}

	data, err := json.Marshal(j.Parameters)
	if err != nil {
		return err
	}
	j.ParametersJSON = string(data)
	return nil
}

// UnmarshalParameters converts the JSON parameters back to a map
func (j *Job) UnmarshalParameters() error {
	if j.ParametersJSON == "" {
		j.Parameters = make(map[string]interface{})
		return nil
	}

	return json.Unmarshal([]byte(j.ParametersJSON), &j.Parameters)
}

// GetParametersSummary returns a human-readable summary of the generation parameters
func (j *Job) GetParametersSummary() string {
	if j.Parameters == nil {
		return "No parameters"
	}

	summary := ""
	if pop, ok := j.Parameters["population"].(float64); ok {
		summary += fmt.Sprintf("%d patients", int(pop))
	}

	if state, ok := j.Parameters["state"].(string); ok && state != "" {
		summary += fmt.Sprintf(", %s", state)
	}

	if ageMin, ok := j.Parameters["ageMin"].(float64); ok {
		if ageMax, ok := j.Parameters["ageMax"].(float64); ok {
			summary += fmt.Sprintf(", ages %d-%d", int(ageMin), int(ageMax))
		}
	}

	if gender, ok := j.Parameters["gender"].(string); ok && gender != "" {
		summary += fmt.Sprintf(", %s", gender)
	}

	return summary
}

// GetStatusBadgeClass returns the CSS class for status badges
func (j *Job) GetStatusBadgeClass() string {
	switch j.Status {
	case JobStatusPending:
		return "bg-yellow-100 text-yellow-800"
	case JobStatusRunning:
		return "bg-blue-100 text-blue-800"
	case JobStatusCompleted:
		return "bg-green-100 text-green-800"
	case JobStatusFailed:
		return "bg-red-100 text-red-800"
	default:
		return "bg-gray-100 text-gray-800"
	}
}

// GetFormattedSize returns human-readable file size
func (j *Job) GetFormattedSize() string {
	if j.OutputSize == nil {
		return ""
	}

	size := *j.OutputSize
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	} else if size < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	} else {
		return fmt.Sprintf("%.1f GB", float64(size)/(1024*1024*1024))
	}
}
