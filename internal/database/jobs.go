package database

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/MediSynth-io/medisynth/internal/models"
)

// CreateJob creates a new job record
func CreateJob(job *models.Job) error {
	if err := job.MarshalParameters(); err != nil {
		return fmt.Errorf("failed to marshal job parameters: %w", err)
	}

	query := `INSERT INTO jobs (id, user_id, job_id, status, parameters, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`
	if dbType == "postgres" {
		query = `INSERT INTO jobs (user_id, job_id, status, parameters, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id`
	}

	if dbType == "postgres" {
		return dbConn.QueryRow(query, job.UserID, job.JobID, job.Status, job.ParametersJSON, job.CreatedAt, job.UpdatedAt).Scan(&job.ID)
	}

	_, err := dbConn.Exec(query, job.ID, job.UserID, job.JobID, job.Status, job.ParametersJSON, job.CreatedAt, job.UpdatedAt)
	return err
}

// UpdateJobStatus updates the status and result of a job
func UpdateJobStatus(jobID string, status models.JobStatus, errorMessage *string, outputPath *string, outputSize *int64, patientCount *int) error {
	var query string
	var err error

	if dbType == "postgres" {
		query = "UPDATE jobs SET status = $1, error_message = $2, output_path = $3, output_size = $4, patient_count = $5, completed_at = NOW() WHERE id = $6"
		_, err = dbConn.Exec(query, status, errorMessage, outputPath, outputSize, patientCount, jobID)
	} else {
		query = "UPDATE jobs SET status = ?, error_message = ?, output_path = ?, output_size = ?, patient_count = ?, completed_at = ? WHERE id = ?"
		_, err = dbConn.Exec(query, status, errorMessage, outputPath, outputSize, patientCount, time.Now(), jobID)
	}

	return err
}

// GetJobByID retrieves a job by its ID
func GetJobByID(id string) (*models.Job, error) {
	job := &models.Job{}
	var query string
	if dbType == "postgres" {
		query = "SELECT id, user_id, job_id, status, parameters, output_format, output_path, output_size, patient_count, error_message, created_at, completed_at FROM jobs WHERE id = $1"
	} else {
		query = "SELECT id, user_id, job_id, status, parameters, output_format, output_path, output_size, patient_count, error_message, created_at, completed_at FROM jobs WHERE id = ?"
	}

	err := dbConn.QueryRow(query, id).Scan(
		&job.ID, &job.UserID, &job.JobID, &job.Status, &job.ParametersJSON, &job.OutputFormat,
		&job.OutputPath, &job.OutputSize, &job.PatientCount, &job.ErrorMessage, &job.CreatedAt, &job.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := job.UnmarshalParameters(); err != nil {
		log.Printf("Warning: failed to unmarshal parameters for job %s: %v", job.ID, err)
	}

	return job, nil
}

// GetJobsByUserID gets all jobs for a user
func GetJobsByUserID(userID string) ([]*models.Job, error) {
	query := `
		SELECT id, user_id, job_id, status, parameters, output_format, output_path, output_size, patient_count, error_message, s3_prefix, created_at, updated_at, completed_at
		FROM jobs
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := dbConn.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		var job models.Job
		if err := rows.Scan(
			&job.ID,
			&job.UserID,
			&job.JobID,
			&job.Status,
			&job.ParametersJSON,
			&job.OutputFormat,
			&job.OutputPath,
			&job.OutputSize,
			&job.PatientCount,
			&job.ErrorMessage,
			&job.S3Prefix,
			&job.CreatedAt,
			&job.UpdatedAt,
			&job.CompletedAt,
		); err != nil {
			return nil, err
		}

		// Parse parameters JSON
		if err := json.Unmarshal(job.ParametersJSON, &job.Parameters); err != nil {
			return nil, err
		}

		jobs = append(jobs, &job)
	}

	return jobs, nil
}

// UpdateJob updates a job in the database
func UpdateJob(job *models.Job) error {
	query := `
		UPDATE jobs
		SET status = $1,
			parameters = $2,
			output_format = $3,
			output_path = $4,
			output_size = $5,
			patient_count = $6,
			error_message = $7,
			s3_prefix = $8,
			updated_at = CURRENT_TIMESTAMP,
			completed_at = $9
		WHERE id = $10
	`

	_, err := dbConn.Exec(query,
		job.Status,
		job.ParametersJSON,
		job.OutputFormat,
		job.OutputPath,
		job.OutputSize,
		job.PatientCount,
		job.ErrorMessage,
		job.S3Prefix,
		job.CompletedAt,
		job.ID,
	)

	return err
}
