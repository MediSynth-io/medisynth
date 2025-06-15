package api

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJobStoreOperations(t *testing.T) {
	// Reset the job store before each test
	resetGlobalJobStore()

	// Test data
	jobID := "test-job-1"
	job := &GenerationJob{
		ID:        jobID,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	t.Run("Add and Get Job", func(t *testing.T) {
		// Add job
		globalJobStore.AddJob(job)

		// Get job
		retrievedJob, exists := globalJobStore.GetJob(jobID)
		assert.True(t, exists)
		assert.Equal(t, job.ID, retrievedJob.ID)
		assert.Equal(t, job.Status, retrievedJob.Status)
	})

	t.Run("Update Job", func(t *testing.T) {
		// Update job status
		job.Status = StatusCompleted
		globalJobStore.UpdateJob(job)

		// Verify update
		retrievedJob, exists := globalJobStore.GetJob(jobID)
		assert.True(t, exists)
		assert.Equal(t, StatusCompleted, retrievedJob.Status)
	})

	t.Run("Delete Job", func(t *testing.T) {
		// Delete job
		globalJobStore.DeleteJob(jobID)

		// Verify deletion
		_, exists := globalJobStore.GetJob(jobID)
		assert.False(t, exists)
	})

	t.Run("Get All Jobs", func(t *testing.T) {
		// Add multiple jobs
		job1 := &GenerationJob{
			ID:        "job-1",
			Status:    StatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		job2 := &GenerationJob{
			ID:        "job-2",
			Status:    StatusRunning,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		globalJobStore.AddJob(job1)
		globalJobStore.AddJob(job2)

		// Get all jobs
		allJobs := globalJobStore.GetAllJobs()
		assert.Equal(t, 2, len(allJobs))
		assert.Equal(t, StatusPending, allJobs["job-1"].Status)
		assert.Equal(t, StatusRunning, allJobs["job-2"].Status)
	})
}

func TestJobStoreConcurrentOperations(t *testing.T) {
	// Reset the job store before each test
	resetGlobalJobStore()

	// Test concurrent operations
	t.Run("Concurrent Add and Get", func(t *testing.T) {
		// Add multiple jobs concurrently
		for i := 0; i < 10; i++ {
			go func(i int) {
				job := &GenerationJob{
					ID:        fmt.Sprintf("concurrent-job-%d", i),
					Status:    StatusPending,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				globalJobStore.AddJob(job)
			}(i)
		}

		// Wait a bit for goroutines to complete
		time.Sleep(100 * time.Millisecond)

		// Verify all jobs were added
		allJobs := globalJobStore.GetAllJobs()
		assert.Equal(t, 10, len(allJobs))
	})

	t.Run("Concurrent Update", func(t *testing.T) {
		// Add a job
		job := &GenerationJob{
			ID:        "concurrent-update-job",
			Status:    StatusPending,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		globalJobStore.AddJob(job)

		// Update job status concurrently
		for i := 0; i < 5; i++ {
			go func() {
				job.Status = StatusRunning
				globalJobStore.UpdateJob(job)
			}()
		}

		// Wait a bit for goroutines to complete
		time.Sleep(100 * time.Millisecond)

		// Verify final state
		retrievedJob, exists := globalJobStore.GetJob("concurrent-update-job")
		assert.True(t, exists)
		assert.Equal(t, StatusRunning, retrievedJob.Status)
	})
}

func TestJobStoreEdgeCases(t *testing.T) {
	resetGlobalJobStore()

	t.Run("UpdateNonExistentJob", func(t *testing.T) {
		job := &GenerationJob{ID: "does-not-exist", Status: StatusPending}
		globalJobStore.UpdateJob(job) // Should not panic or error
		_, exists := globalJobStore.GetJob("does-not-exist")
		if exists {
			t.Error("Expected job to not exist after update attempt")
		}
	})

	t.Run("DeleteNonExistentJob", func(t *testing.T) {
		globalJobStore.DeleteJob("does-not-exist") // Should not panic or error
		// No assertion needed, just ensure no panic
	})

	t.Run("GetAllJobsEmpty", func(t *testing.T) {
		jobs := globalJobStore.GetAllJobs()
		if len(jobs) != 0 {
			t.Errorf("Expected 0 jobs, got %d", len(jobs))
		}
	})
}
