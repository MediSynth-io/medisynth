package api

import (
	"sync"
	"time"
)

// JobStatus represents the current state of a generation job
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
)

// SyntheaParams represents the parameters for a Synthea generation job
type SyntheaParams struct {
	Population    *int     `json:"population"`
	OutputFormat  *string  `json:"outputFormat,omitempty"`
	KeepModules   []string `json:"keepModules,omitempty"`
	CustomModules []string `json:"customModules,omitempty"`
	State         *string  `json:"state,omitempty"`
	City          *string  `json:"city,omitempty"`
	Gender        *string  `json:"gender,omitempty"`
	AgeMin        *int     `json:"ageMin,omitempty"`
	AgeMax        *int     `json:"ageMax,omitempty"`
	Seed          *int64   `json:"seed,omitempty"`
}

// GenerationJob represents a single patient generation job
type GenerationJob struct {
	ID            string                 `json:"id"`
	Status        JobStatus              `json:"status"`
	RequestParams SyntheaParams          `json:"requestParams"`
	Result        map[string]interface{} `json:"result,omitempty"`
	Error         string                 `json:"error,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
	UpdatedAt     time.Time              `json:"updatedAt"`
}

// JobStore manages the collection of generation jobs
type JobStore struct {
	mu   sync.RWMutex
	jobs map[string]*GenerationJob
}

// Global job store instance
var globalJobStore = &JobStore{
	jobs: make(map[string]*GenerationJob),
}

// AddJob adds a new job to the store
func (s *JobStore) AddJob(job *GenerationJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

// GetJob retrieves a job by ID
func (s *JobStore) GetJob(id string) (*GenerationJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, exists := s.jobs[id]
	return job, exists
}

// UpdateJob updates an existing job
func (s *JobStore) UpdateJob(job *GenerationJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

// DeleteJob removes a job from the store
func (s *JobStore) DeleteJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}

// GetAllJobs returns a copy of all jobs
func (s *JobStore) GetAllJobs() map[string]*GenerationJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make(map[string]*GenerationJob)
	for id, job := range s.jobs {
		jobs[id] = job
	}
	return jobs
}
