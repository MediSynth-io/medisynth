package database

import (
	"log"
	"time"

	"github.com/MediSynth-io/medisynth/internal/models"
)

// CreateJob creates a new job record
func CreateJob(job *models.Job) error {
	var query string
	if dbType == "postgres" {
		query = "INSERT INTO jobs (id, user_id, job_id, status, parameters, output_format) VALUES ($1, $2, $3, $4, $5, $6) RETURNING created_at"
		return dbConn.QueryRow(query, job.ID, job.UserID, job.JobID, job.Status, job.ParametersJSON, job.OutputFormat).Scan(&job.CreatedAt)
	}

	query = "INSERT INTO jobs (id, user_id, job_id, status, parameters, output_format, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)"
	_, err := dbConn.Exec(query, job.ID, job.UserID, job.JobID, job.Status, job.ParametersJSON, job.OutputFormat, job.CreatedAt)
	return err
}

// UpdateJobStatus updates the status and result of a job
func UpdateJobStatus(jobID string, status models.JobStatus, errorMessage *string, outputPath *string, outputSize *int64, patientCount *int) error {
	var query string
	if dbType == "postgres" {
		query = "UPDATE jobs SET status = $1, error_message = $2, output_path = $3, output_size = $4, patient_count = $5, completed_at = NOW() WHERE id = $6"
	} else {
		query = "UPDATE jobs SET status = ?, error_message = ?, output_path = ?, output_size = ?, patient_count = ?, completed_at = ? WHERE id = ?"
	}

	_, err := dbConn.Exec(query, status, errorMessage, outputPath, outputSize, patientCount, time.Now(), jobID)
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
		log.Printf("Warning: could not unmarshal job parameters for job %s: %v", job.ID, err)
	}

	return job, nil
}

// GetJobsByUserID retrieves all jobs for a user
func GetJobsByUserID(userID string) ([]*models.Job, error) {
	var query string
	if dbType == "postgres" {
		query = "SELECT id, user_id, job_id, status, parameters, output_format, output_path, patient_count, error_message, created_at, completed_at FROM jobs WHERE user_id = $1 ORDER BY created_at DESC"
	} else {
		query = "SELECT id, user_id, job_id, status, parameters, output_format, output_path, patient_count, error_message, created_at, completed_at FROM jobs WHERE user_id = ? ORDER BY created_at DESC"
	}

	rows, err := dbConn.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		job := &models.Job{}
		err := rows.Scan(
			&job.ID, &job.UserID, &job.JobID, &job.Status, &job.ParametersJSON, &job.OutputFormat,
			&job.OutputPath, &job.PatientCount, &job.ErrorMessage, &job.CreatedAt, &job.CompletedAt,
		)
		if err != nil {
			return nil, err
		}

		if err := job.UnmarshalParameters(); err != nil {
			log.Printf("Warning: could not unmarshal job parameters for job %s: %v", job.ID, err)
		}

		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}
