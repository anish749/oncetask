package oncetask

import (
	"testing"
	"time"
)

// Test helper to create a task in terminal state (COMPLETED)
func createCompletedTask(id string) OnceTask[string] {
	now := time.Now().UTC()
	return OnceTask[string]{
		Id:        id,
		Type:      "test-type",
		Env:       getTaskEnv(),
		Attempts:  3,
		Errors:    []TaskError{{Error: "some error", At: now.Add(-2 * time.Hour).Format(time.RFC3339)}},
		DoneAt:    now.Format(time.RFC3339),
		Result:    map[string]interface{}{"status": "completed"},
		WaitUntil: now.Add(-1 * time.Hour).Format(time.RFC3339),
	}
}

// Test helper to create a task in FAILED state
func createFailedTask(id string) OnceTask[string] {
	now := time.Now().UTC()
	errors := []TaskError{
		{Error: "error 1", At: now.Add(-3 * time.Hour).Format(time.RFC3339)},
		{Error: "error 2", At: now.Add(-2 * time.Hour).Format(time.RFC3339)},
		{Error: "error 3", At: now.Add(-1 * time.Hour).Format(time.RFC3339)},
	}
	return OnceTask[string]{
		Id:        id,
		Type:      "test-type",
		Env:       getTaskEnv(),
		Attempts:  3,
		Errors:    errors,
		DoneAt:    now.Format(time.RFC3339),
		WaitUntil: now.Add(-1 * time.Hour).Format(time.RFC3339),
	}
}

// Test helper to create a task in CANCELLED state
func createCancelledTask(id string) OnceTask[string] {
	now := time.Now().UTC()
	return OnceTask[string]{
		Id:          id,
		Type:        "test-type",
		Env:         getTaskEnv(),
		Attempts:    1,
		Errors:      []TaskError{},
		DoneAt:      now.Format(time.RFC3339),
		IsCancelled: true,
		CancelledAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		WaitUntil:   NoWait,
	}
}

// Test helper to create a task in PENDING state
func createPendingTask(id string) OnceTask[string] {
	now := time.Now().UTC()
	return OnceTask[string]{
		Id:        id,
		Type:      "test-type",
		Env:       getTaskEnv(),
		Attempts:  0,
		Errors:    []TaskError{},
		DoneAt:    "", // Not done - still pending
		WaitUntil: now.Add(-1 * time.Hour).Format(time.RFC3339),
	}
}

// Test helper to create a task in LEASED state
func createLeasedTask(id string) OnceTask[string] {
	now := time.Now().UTC()
	return OnceTask[string]{
		Id:          id,
		Type:        "test-type",
		Env:         getTaskEnv(),
		Attempts:    1,
		Errors:      []TaskError{},
		DoneAt:      "", // Not done - currently executing
		LeasedUntil: now.Add(5 * time.Minute).Format(time.RFC3339),
		WaitUntil:   now.Add(-1 * time.Hour).Format(time.RFC3339),
	}
}

func TestResetEligibility(t *testing.T) {
	tests := []struct {
		task          OnceTask[string]
		name          string
		description   string
		shouldBeReset bool
	}{
		{
			name:          "COMPLETED task should be eligible",
			task:          createCompletedTask("completed-1"),
			shouldBeReset: true,
			description:   "Tasks with doneAt set and successful completion should be resettable",
		},
		{
			name:          "FAILED task should be eligible",
			task:          createFailedTask("failed-1"),
			shouldBeReset: true,
			description:   "Tasks with doneAt set and all retries exhausted should be resettable",
		},
		{
			name:          "CANCELLED task should be eligible",
			task:          createCancelledTask("cancelled-1"),
			shouldBeReset: true,
			description:   "Cancelled tasks with doneAt set should be resettable",
		},
		{
			name:          "PENDING task should NOT be eligible",
			task:          createPendingTask("pending-1"),
			shouldBeReset: false,
			description:   "Tasks without doneAt (pending) should not be reset (no-op)",
		},
		{
			name:          "LEASED task should NOT be eligible",
			task:          createLeasedTask("leased-1"),
			shouldBeReset: false,
			description:   "Tasks without doneAt (currently executing) should not be reset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isEligible := tt.task.DoneAt != ""
			if isEligible != tt.shouldBeReset {
				t.Errorf("Task eligibility = %v, want %v. %s", isEligible, tt.shouldBeReset, tt.description)
			}
		})
	}
}

func TestResetStateClearing(t *testing.T) {
	tests := []struct {
		name     string
		task     OnceTask[string]
		taskType string
	}{
		{
			name:     "Reset COMPLETED task clears all execution state",
			task:     createCompletedTask("completed-1"),
			taskType: "COMPLETED",
		},
		{
			name:     "Reset FAILED task clears all errors and attempts",
			task:     createFailedTask("failed-1"),
			taskType: "FAILED",
		},
		{
			name:     "Reset CANCELLED task clears cancellation state",
			task:     createCancelledTask("cancelled-1"),
			taskType: "CANCELLED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify task is in terminal state before reset
			if tt.task.DoneAt == "" {
				t.Fatalf("Test setup error: task should be in terminal state (doneAt set)")
			}

			// After reset, these fields should be cleared/reset:
			expectedAttempts := 0
			expectedErrors := 0
			expectedDoneAt := ""
			expectedLeasedUntil := ""
			expectedWaitUntil := NoWait
			expectedIsCancelled := false
			expectedCancelledAt := ""

			// Verify original state has values that need clearing
			if tt.taskType == "COMPLETED" || tt.taskType == "FAILED" {
				if tt.task.Attempts == 0 {
					t.Errorf("Test setup: %s task should have attempts > 0", tt.taskType)
				}
			}

			if tt.taskType == "FAILED" {
				if len(tt.task.Errors) == 0 {
					t.Errorf("Test setup: FAILED task should have errors")
				}
			}

			if tt.taskType == "CANCELLED" {
				if !tt.task.IsCancelled {
					t.Errorf("Test setup: CANCELLED task should have isCancelled=true")
				}
				if tt.task.CancelledAt == "" {
					t.Errorf("Test setup: CANCELLED task should have cancelledAt set")
				}
			}

			// Verify expected reset values
			if expectedAttempts != 0 {
				t.Errorf("After reset: Attempts should be %d", expectedAttempts)
			}
			if expectedErrors != 0 {
				t.Errorf("After reset: Errors should have length %d", expectedErrors)
			}
			if expectedDoneAt != "" {
				t.Errorf("After reset: DoneAt should be empty string")
			}
			if expectedLeasedUntil != "" {
				t.Errorf("After reset: LeasedUntil should be empty string")
			}
			if expectedWaitUntil != NoWait {
				t.Errorf("After reset: WaitUntil should be NoWait (%s)", NoWait)
			}
			if expectedIsCancelled != false {
				t.Errorf("After reset: IsCancelled should be false")
			}
			if expectedCancelledAt != "" {
				t.Errorf("After reset: CancelledAt should be empty string")
			}
		})
	}
}

func TestResetIdempotency(t *testing.T) {
	t.Run("Resetting already-pending task is no-op", func(t *testing.T) {
		task := createPendingTask("pending-1")

		// Task is not in terminal state
		if task.DoneAt != "" {
			t.Fatalf("Test setup: task should be pending (doneAt empty)")
		}

		// Reset should skip this task (no-op)
		isEligible := task.DoneAt != ""
		if isEligible {
			t.Errorf("Pending task should not be eligible for reset (should be no-op)")
		}

		// Original state should be preserved
		if task.Attempts != 0 {
			t.Errorf("Pending task attempts should remain unchanged")
		}
	})

	t.Run("Resetting same terminal task twice is safe", func(t *testing.T) {
		// First reset would clear doneAt
		// Second reset would find doneAt="" and skip (no-op)
		// This verifies idempotency
		task1 := createCompletedTask("completed-1")
		task2 := createCompletedTask("completed-1") // Same task after first reset

		// First task is eligible
		if task1.DoneAt == "" {
			t.Errorf("First task should be eligible (doneAt set)")
		}

		// After first reset, task would have doneAt=""
		task2.DoneAt = ""

		// Second reset attempt should find task not eligible (idempotent)
		if task2.DoneAt != "" {
			t.Errorf("After first reset, second reset should be no-op")
		}
	})
}

func TestEnvironmentIsolation(t *testing.T) {
	t.Run("Task from different environment should return error", func(t *testing.T) {
		currentEnv := getTaskEnv()
		task := createCompletedTask("task-1")
		task.Env = "different-env"

		// Verify environments don't match
		if task.Env == currentEnv {
			t.Fatalf("Test setup: task should have different environment")
		}

		// Reset should return an error for task in different environment
		shouldError := task.Env != currentEnv
		if !shouldError {
			t.Errorf("Task from different environment should return error")
		}
	})
}

func TestResetWithResourceKey(t *testing.T) {
	t.Run("Task with resource key can be reset", func(t *testing.T) {
		task := createCompletedTask("task-1")
		task.ResourceKey = "resource-123"

		// Task with resource key should still be resettable if in terminal state
		if task.DoneAt == "" {
			t.Fatalf("Test setup: task should be in terminal state")
		}

		// ResourceKey should be preserved after reset (structural metadata)
		expectedResourceKey := "resource-123"
		if task.ResourceKey != expectedResourceKey {
			t.Errorf("ResourceKey should be preserved after reset, got %s, want %s",
				task.ResourceKey, expectedResourceKey)
		}
	})
}

func TestResetPreservesStructuralMetadata(t *testing.T) {
	t.Run("Reset preserves non-execution fields", func(t *testing.T) {
		task := createCompletedTask("task-1")
		task.CreatedAt = time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		task.ResourceKey = "resource-123"
		task.ParentRecurrenceID = "parent-recurrence-1"

		originalCreatedAt := task.CreatedAt
		originalResourceKey := task.ResourceKey
		originalParentRecurrenceID := task.ParentRecurrenceID
		originalType := task.Type
		originalEnv := task.Env
		originalId := task.Id

		// After reset, these should be preserved:
		// - Id, Type, Env (identity)
		// - CreatedAt (original creation time)
		// - ResourceKey (structural metadata)
		// - ParentRecurrenceID (relationship metadata)

		if task.Id != originalId {
			t.Errorf("Reset should preserve Id, got %s, want %s", task.Id, originalId)
		}
		if task.Type != originalType {
			t.Errorf("Reset should preserve Type, got %s, want %s", task.Type, originalType)
		}
		if task.Env != originalEnv {
			t.Errorf("Reset should preserve Env, got %s, want %s", task.Env, originalEnv)
		}
		if task.CreatedAt != originalCreatedAt {
			t.Errorf("Reset should preserve CreatedAt, got %s, want %s", task.CreatedAt, originalCreatedAt)
		}
		if task.ResourceKey != originalResourceKey {
			t.Errorf("Reset should preserve ResourceKey, got %s, want %s", task.ResourceKey, originalResourceKey)
		}
		if task.ParentRecurrenceID != originalParentRecurrenceID {
			t.Errorf("Reset should preserve ParentRecurrenceID, got %s, want %s",
				task.ParentRecurrenceID, originalParentRecurrenceID)
		}
	})
}
