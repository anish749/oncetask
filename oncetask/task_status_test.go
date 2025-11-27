package oncetask

import (
	"testing"
	"time"
)

func TestGetStatus(t *testing.T) {
	now := time.Now()
	epochTime := time.Time{}.Format(time.RFC3339)

	tests := []struct {
		name       string
		task       OnceTask[string]
		wantStatus TaskStatus
	}{
		{
			name: "completed when done with more attempts than errors",
			task: OnceTask[string]{
				DoneAt:   now.Add(-1 * time.Hour).Format(time.RFC3339),
				Attempts: 2,
				Errors:   []TaskError{{At: now.Add(-2 * time.Hour).Format(time.RFC3339), Error: "first failure"}},
			},
			wantStatus: TaskStatusCompleted,
		},
		{
			name: "failed when done with attempts equal to errors",
			task: OnceTask[string]{
				DoneAt:   now.Add(-1 * time.Hour).Format(time.RFC3339),
				Attempts: 3,
				Errors: []TaskError{
					{At: now.Add(-3 * time.Hour).Format(time.RFC3339), Error: "first failure"},
					{At: now.Add(-2 * time.Hour).Format(time.RFC3339), Error: "second failure"},
					{At: now.Add(-1 * time.Hour).Format(time.RFC3339), Error: "third failure"},
				},
			},
			wantStatus: TaskStatusFailed,
		},
		{
			name: "leased when lease is active",
			task: OnceTask[string]{
				LeasedUntil: now.Add(5 * time.Minute).Format(time.RFC3339),
				Attempts:    1,
				DoneAt:      "",
			},
			wantStatus: TaskStatusLeased,
		},
		{
			name: "waiting when waitUntil is in the future",
			task: OnceTask[string]{
				WaitUntil:   now.Add(1 * time.Hour).Format(time.RFC3339),
				LeasedUntil: "",
				DoneAt:      "",
			},
			wantStatus: TaskStatusWaiting,
		},
		{
			name: "waiting in backoff after failure",
			task: OnceTask[string]{
				WaitUntil:   now.Add(30 * time.Second).Format(time.RFC3339),
				LeasedUntil: "",
				DoneAt:      "",
				Attempts:    1,
				Errors:      []TaskError{{At: now.Add(-1 * time.Minute).Format(time.RFC3339), Error: "first failure"}},
			},
			wantStatus: TaskStatusWaiting,
		},
		{
			name: "pending when waitUntil is in the past",
			task: OnceTask[string]{
				WaitUntil:   now.Add(-1 * time.Hour).Format(time.RFC3339),
				LeasedUntil: "",
				DoneAt:      "",
				Attempts:    0,
			},
			wantStatus: TaskStatusPending,
		},
		{
			name: "pending after lease expired",
			task: OnceTask[string]{
				LeasedUntil: now.Add(-1 * time.Minute).Format(time.RFC3339),
				WaitUntil:   "",
				DoneAt:      "",
				Attempts:    1,
			},
			wantStatus: TaskStatusPending,
		},
		{
			name: "pending with epoch time (immediate execution)",
			task: OnceTask[string]{
				WaitUntil:   epochTime,
				LeasedUntil: "",
				DoneAt:      "",
				Attempts:    0,
			},
			wantStatus: TaskStatusPending,
		},
		{
			name: "cancellationPending when cancelled but not done",
			task: OnceTask[string]{
				IsCancelled: true,
				CancelledAt: now.Add(-1 * time.Minute).Format(time.RFC3339),
				DoneAt:      "",
				WaitUntil:   epochTime,
				LeasedUntil: "",
				Attempts:    0,
				Errors:      nil,
			},
			wantStatus: TaskStatusCancellationPending,
		},
		{
			name: "cancelled when both cancelled and done",
			task: OnceTask[string]{
				IsCancelled: true,
				CancelledAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				DoneAt:      now.Add(-1 * time.Minute).Format(time.RFC3339),
				Attempts:    1,
				Errors:      nil,
			},
			wantStatus: TaskStatusCancelled,
		},
		{
			name: "cancelled takes priority over completed",
			task: OnceTask[string]{
				IsCancelled: true,
				CancelledAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				DoneAt:      now.Add(-1 * time.Minute).Format(time.RFC3339),
				Attempts:    2,
				Errors:      []TaskError{{At: now.Format(time.RFC3339), Error: "first failure"}},
			},
			wantStatus: TaskStatusCancelled,
		},
		{
			name: "cancelled takes priority over failed",
			task: OnceTask[string]{
				IsCancelled: true,
				CancelledAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				DoneAt:      now.Add(-1 * time.Minute).Format(time.RFC3339),
				Attempts:    2,
				Errors: []TaskError{
					{At: now.Format(time.RFC3339), Error: "first failure"},
					{At: now.Format(time.RFC3339), Error: "second failure"},
				},
			},
			wantStatus: TaskStatusCancelled,
		},
		{
			name: "cancellationPending when cancelled and leased",
			task: OnceTask[string]{
				IsCancelled: true,
				CancelledAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
				DoneAt:      "",
				WaitUntil:   epochTime,
				LeasedUntil: now.Add(5 * time.Minute).Format(time.RFC3339),
				Attempts:    0,
				Errors:      nil,
			},
			wantStatus: TaskStatusCancellationPending,
		},
		{
			name: "cancellationPending takes priority over waiting",
			task: OnceTask[string]{
				IsCancelled: true,
				WaitUntil:   now.Add(1 * time.Hour).Format(time.RFC3339),
				DoneAt:      "",
			},
			wantStatus: TaskStatusCancellationPending,
		},
		{
			name: "cancellationPending takes priority over pending",
			task: OnceTask[string]{
				IsCancelled: true,
				WaitUntil:   epochTime,
				DoneAt:      "",
			},
			wantStatus: TaskStatusCancellationPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := tt.task.GetStatus(now)
			if err != nil {
				t.Fatalf("GetStatus failed: %v", err)
			}

			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}
		})
	}
}

func TestGetStatus_InvalidTimestamps(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name string
		task OnceTask[string]
	}{
		{
			name: "invalid doneAt",
			task: OnceTask[string]{
				DoneAt: "invalid-timestamp",
			},
		},
		{
			name: "invalid leasedUntil",
			task: OnceTask[string]{
				DoneAt:      "",
				LeasedUntil: "invalid-timestamp",
			},
		},
		{
			name: "invalid waitUntil",
			task: OnceTask[string]{
				DoneAt:      "",
				LeasedUntil: "",
				WaitUntil:   "invalid-timestamp",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.task.GetStatus(now)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestGetStatus_Priority(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name       string
		task       OnceTask[string]
		wantStatus TaskStatus
	}{
		{
			name: "doneAt takes priority over leasedUntil and waitUntil",
			task: OnceTask[string]{
				DoneAt:      now.Add(-1 * time.Hour).Format(time.RFC3339),
				LeasedUntil: now.Add(1 * time.Hour).Format(time.RFC3339),
				WaitUntil:   now.Add(2 * time.Hour).Format(time.RFC3339),
				Attempts:    1,
				Errors:      []TaskError{},
			},
			wantStatus: TaskStatusCompleted,
		},
		{
			name: "leasedUntil takes priority over waitUntil",
			task: OnceTask[string]{
				DoneAt:      "",
				LeasedUntil: now.Add(5 * time.Minute).Format(time.RFC3339),
				WaitUntil:   now.Add(1 * time.Hour).Format(time.RFC3339),
				Attempts:    1,
			},
			wantStatus: TaskStatusLeased,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := tt.task.GetStatus(now)
			if err != nil {
				t.Fatalf("GetStatus failed: %v", err)
			}

			if status != tt.wantStatus {
				t.Errorf("status = %v, want %v", status, tt.wantStatus)
			}
		})
	}
}
