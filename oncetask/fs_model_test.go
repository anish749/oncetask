package oncetask

import (
	"strings"
	"testing"
	"time"
)

// Test task data types for different scenarios

type immediateTask struct {
	id string
}

func (t immediateTask) GetType() string              { return "immediate" }
func (t immediateTask) GenerateIdempotentID() string { return t.id }

type scheduledTask struct {
	id           string
	scheduledFor time.Time
}

func (t scheduledTask) GetType() string              { return "scheduled" }
func (t scheduledTask) GenerateIdempotentID() string { return t.id }
func (t scheduledTask) GetScheduledTime() time.Time  { return t.scheduledFor }

type recurringTask struct {
	id         string
	recurrence Recurrence
}

func (t recurringTask) GetType() string              { return "recurring" }
func (t recurringTask) GenerateIdempotentID() string { return t.id }
func (t recurringTask) GetRecurrence() *Recurrence   { return &t.recurrence }

type conflictingTask struct {
	id           string
	scheduledFor time.Time
	recurrence   Recurrence
}

func (t *conflictingTask) GetType() string              { return "conflicting" }
func (t *conflictingTask) GenerateIdempotentID() string { return t.id }
func (t *conflictingTask) GetScheduledTime() time.Time  { return t.scheduledFor }
func (t *conflictingTask) GetRecurrence() *Recurrence   { return &t.recurrence }

type taskWithResourceKey struct {
	id          string
	resourceKey string
}

func (t taskWithResourceKey) GetType() string              { return "withResource" }
func (t taskWithResourceKey) GenerateIdempotentID() string { return t.id }
func (t taskWithResourceKey) GetResourceKey() string       { return t.resourceKey }

// Test cases

func TestNewOnceTask_WaitUntil(t *testing.T) {
	epochTime := NoWait
	scheduledTime := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)
	dtstart := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		task           Data[string]
		wantWaitUntil  string
		wantRecurrence bool
	}{
		{
			name:           "immediate task uses epoch time",
			task:           immediateTask{id: "task-1"},
			wantWaitUntil:  epochTime,
			wantRecurrence: false,
		},
		{
			name:           "scheduled task uses scheduled time",
			task:           scheduledTask{id: "task-2", scheduledFor: scheduledTime},
			wantWaitUntil:  scheduledTime.Format(time.RFC3339),
			wantRecurrence: false,
		},
		{
			name:           "scheduled task with zero time uses epoch",
			task:           scheduledTask{id: "task-3", scheduledFor: time.Time{}},
			wantWaitUntil:  epochTime,
			wantRecurrence: false,
		},
		{
			name: "recurring task uses DTStart",
			task: recurringTask{
				id: "task-4",
				recurrence: Recurrence{
					RRule:   "FREQ=DAILY",
					DTStart: dtstart.Format(time.RFC3339),
				},
			},
			wantWaitUntil:  dtstart.Format(time.RFC3339),
			wantRecurrence: true,
		},
		{
			name: "recurring task with zero scheduled time uses DTStart",
			task: &conflictingTask{
				id:           "task-5",
				scheduledFor: time.Time{}, // Zero time - should be ignored
				recurrence: Recurrence{
					RRule:   "FREQ=DAILY",
					DTStart: dtstart.Format(time.RFC3339),
				},
			},
			wantWaitUntil:  dtstart.Format(time.RFC3339),
			wantRecurrence: true,
		},
		{
			name: "recurring task with ExDate on DTStart skips to next occurrence",
			task: recurringTask{
				id: "task-6",
				recurrence: Recurrence{
					RRule:   "FREQ=DAILY",
					DTStart: dtstart.Format(time.RFC3339),
					ExDates: []string{dtstart.Format(time.RFC3339)},
				},
			},
			wantWaitUntil:  time.Date(2025, 1, 2, 9, 0, 0, 0, time.UTC).Format(time.RFC3339),
			wantRecurrence: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			onceTask, err := newOnceTask(tt.task)
			if err != nil {
				t.Fatalf("newOnceTask failed: %v", err)
			}

			if onceTask.WaitUntil != tt.wantWaitUntil {
				t.Errorf("WaitUntil = %q, want %q", onceTask.WaitUntil, tt.wantWaitUntil)
			}

			hasRecurrence := onceTask.Recurrence != nil
			if hasRecurrence != tt.wantRecurrence {
				t.Errorf("has Recurrence = %v, want %v", hasRecurrence, tt.wantRecurrence)
			}
		})
	}
}

func TestNewOnceTask_RecurrenceErrors(t *testing.T) {
	dtstart := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	scheduledTime := time.Date(2025, 6, 15, 14, 30, 0, 0, time.UTC)

	tests := []struct {
		name            string
		task            Data[string]
		wantErrContains string
	}{
		{
			name: "missing DTStart",
			task: recurringTask{
				id: "task-1",
				recurrence: Recurrence{
					RRule:   "FREQ=DAILY",
					DTStart: "",
				},
			},
			wantErrContains: "DTStart is required",
		},
		{
			name: "missing RRule",
			task: recurringTask{
				id: "task-2",
				recurrence: Recurrence{
					RRule:   "",
					DTStart: dtstart.Format(time.RFC3339),
				},
			},
			wantErrContains: "RRule is required",
		},
		{
			name: "invalid DTStart",
			task: recurringTask{
				id: "task-3",
				recurrence: Recurrence{
					RRule:   "FREQ=DAILY",
					DTStart: "not-a-valid-timestamp",
				},
			},
			wantErrContains: "invalid DTStart",
		},
		{
			name: "invalid RRule",
			task: recurringTask{
				id: "task-4",
				recurrence: Recurrence{
					RRule:   "INVALID_RRULE_FORMAT",
					DTStart: dtstart.Format(time.RFC3339),
				},
			},
			wantErrContains: "invalid RRule",
		},
		{
			name: "conflicting Recurrence and ScheduledTask",
			task: &conflictingTask{
				id:           "task-5",
				scheduledFor: scheduledTime,
				recurrence: Recurrence{
					RRule:   "FREQ=DAILY",
					DTStart: dtstart.Format(time.RFC3339),
				},
			},
			wantErrContains: "cannot specify both Recurrence and ScheduledTask",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newOnceTask(tt.task)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErrContains)
			}
		})
	}
}

func TestNewOnceTask_FieldsInitialization(t *testing.T) {
	t.Run("initial fields are set correctly", func(t *testing.T) {
		onceTask, err := newOnceTask(immediateTask{id: "task-1"})
		if err != nil {
			t.Fatalf("newOnceTask failed: %v", err)
		}

		if onceTask.Id != "task-1" {
			t.Errorf("Id = %q, want %q", onceTask.Id, "task-1")
		}

		if onceTask.LeasedUntil != "" {
			t.Errorf("LeasedUntil = %q, want empty", onceTask.LeasedUntil)
		}

		if onceTask.DoneAt != "" {
			t.Errorf("DoneAt = %q, want empty", onceTask.DoneAt)
		}

		if onceTask.Attempts != 0 {
			t.Errorf("Attempts = %d, want 0", onceTask.Attempts)
		}

		if len(onceTask.Errors) != 0 {
			t.Errorf("Errors = %v, want empty", onceTask.Errors)
		}

		if onceTask.Result != nil {
			t.Errorf("Result = %v, want nil", onceTask.Result)
		}

		if onceTask.CreatedAt == "" {
			t.Error("CreatedAt should be set")
		}

		if _, err := time.Parse(time.RFC3339, onceTask.CreatedAt); err != nil {
			t.Errorf("CreatedAt is not valid RFC3339: %v", err)
		}

		if onceTask.Env == "" {
			t.Error("Env should be set")
		}
	})

	t.Run("type and data are set correctly", func(t *testing.T) {
		onceTask, err := newOnceTask(immediateTask{id: "task-2"})
		if err != nil {
			t.Fatalf("newOnceTask failed: %v", err)
		}

		if string(onceTask.Type) != "immediate" {
			t.Errorf("Type = %q, want %q", onceTask.Type, "immediate")
		}

		if onceTask.Data == nil {
			t.Error("Data should not be nil")
		}
	})

	t.Run("resource key is extracted", func(t *testing.T) {
		onceTask, err := newOnceTask(taskWithResourceKey{id: "task-3", resourceKey: "calendar-123"})
		if err != nil {
			t.Fatalf("newOnceTask failed: %v", err)
		}

		if onceTask.ResourceKey != "calendar-123" {
			t.Errorf("ResourceKey = %q, want %q", onceTask.ResourceKey, "calendar-123")
		}
	})

	t.Run("recurrence fields are preserved", func(t *testing.T) {
		dtstart := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
		exdate1 := time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC)
		exdate2 := time.Date(2025, 2, 1, 9, 0, 0, 0, time.UTC)

		onceTask, err := newOnceTask(recurringTask{
			id: "task-4",
			recurrence: Recurrence{
				RRule:   "FREQ=DAILY",
				DTStart: dtstart.Format(time.RFC3339),
				ExDates: []string{
					exdate1.Format(time.RFC3339),
					exdate2.Format(time.RFC3339),
				},
			},
		})
		if err != nil {
			t.Fatalf("newOnceTask failed: %v", err)
		}

		if onceTask.Recurrence == nil {
			t.Fatal("Recurrence should be set")
		}

		if onceTask.Recurrence.RRule != "FREQ=DAILY" {
			t.Errorf("RRule = %q, want %q", onceTask.Recurrence.RRule, "FREQ=DAILY")
		}

		if len(onceTask.Recurrence.ExDates) != 2 {
			t.Errorf("ExDates count = %d, want 2", len(onceTask.Recurrence.ExDates))
		}
	})
}
