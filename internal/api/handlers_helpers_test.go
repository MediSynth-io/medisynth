package api

import (
	"testing"
	"time"
)

func TestPstr(t *testing.T) {
	val := "hello"
	ptr := Pstr(val)
	if ptr == nil || *ptr != val {
		t.Errorf("Pstr did not return pointer to correct value: got %v", ptr)
	}
}

func TestPint(t *testing.T) {
	val := 42
	ptr := Pint(val)
	if ptr == nil || *ptr != val {
		t.Errorf("Pint did not return pointer to correct value: got %v", ptr)
	}
}

func TestNewJobID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := newJobID()
		if ids[id] {
			t.Errorf("Duplicate job ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestWaitForJobStatus(t *testing.T) {
	resetGlobalJobStore()
	jobID := "wait-test-job"
	job := &GenerationJob{
		ID:        jobID,
		Status:    StatusPending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	globalJobStore.AddJob(job)

	// Change status after a short delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		job.Status = StatusCompleted
		globalJobStore.UpdateJob(job)
	}()

	ok := waitForJobStatus(jobID, StatusCompleted, 1*time.Second)
	if !ok {
		t.Error("waitForJobStatus did not detect status change in time")
	}

	// Test for non-existent job
	ok = waitForJobStatus("nonexistent", StatusCompleted, 100*time.Millisecond)
	if ok {
		t.Error("waitForJobStatus should return false for nonexistent job")
	}
}
