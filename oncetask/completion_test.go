package oncetask

import (
	"fmt"
	"testing"
	"time"
)

func TestProcessTaskFailure_RetryPolicySelection(t *testing.T) {
	now := time.Now()
	execErr := fmt.Errorf("task execution failed")

	normalRetryPolicy := ExponentialBackoffPolicy{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    5 * time.Minute,
		Multiplier:  2.0,
	}

	cancellationRetryPolicy := ExponentialBackoffPolicy{
		MaxAttempts: 5,
		BaseDelay:   2 * time.Second,
		MaxDelay:    10 * time.Minute,
		Multiplier:  2.0,
	}

	config := HandlerConfig{
		RetryPolicy:             normalRetryPolicy,
		CancellationRetryPolicy: cancellationRetryPolicy,
	}

	tests := []struct {
		name            string
		task            OnceTask[string]
		wantRetryPolicy RetryPolicy
	}{
		{
			name: "uses normal retry policy for non-cancelled task",
			task: OnceTask[string]{
				IsCancelled: false,
				Attempts:    0,
				Errors:      nil,
			},
			wantRetryPolicy: normalRetryPolicy,
		},
		{
			name: "uses cancellation retry policy for cancelled task",
			task: OnceTask[string]{
				IsCancelled: true,
				CancelledAt: now.Format(time.RFC3339),
				Attempts:    0,
				Errors:      nil,
			},
			wantRetryPolicy: cancellationRetryPolicy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updates := processTaskFailure(execErr, config, tt.task, now)

			// Verify that retry was scheduled (not permanently failed)
			var foundWaitUntil bool
			for _, update := range updates {
				if update.Path == "waitUntil" {
					foundWaitUntil = true

					// Parse waitUntil timestamp
					waitUntilStr, ok := update.Value.(string)
					if !ok {
						t.Fatalf("waitUntil value is not string: %T", update.Value)
					}
					waitUntil, err := time.Parse(time.RFC3339, waitUntilStr)
					if err != nil {
						t.Fatalf("failed to parse waitUntil: %v", err)
					}

					// Calculate expected backoff
					expectedDelay := tt.wantRetryPolicy.NextRetryDelay(tt.task.Attempts, execErr)
					expectedWaitUntil := now.Add(expectedDelay)

					// Allow 1 second tolerance for timing differences
					diff := waitUntil.Sub(expectedWaitUntil)
					if diff < -time.Second || diff > time.Second {
						t.Errorf("waitUntil = %v, want ~%v (diff: %v)", waitUntil, expectedWaitUntil, diff)
					}
				}
			}

			if !foundWaitUntil {
				t.Error("expected waitUntil field in updates")
			}
		})
	}
}

func TestProcessTaskFailure_CancellationRetryExhaustion(t *testing.T) {
	now := time.Now()
	execErr := fmt.Errorf("cancellation handler failed")

	config := HandlerConfig{
		CancellationRetryPolicy: ExponentialBackoffPolicy{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Second,
			MaxDelay:    1 * time.Minute,
			Multiplier:  2.0,
		},
	}

	tests := []struct {
		name         string
		attempts     int
		wantDone     bool
		wantWaitUtil bool
	}{
		{
			name:         "retries after first failure",
			attempts:     0,
			wantDone:     false,
			wantWaitUtil: true,
		},
		{
			name:         "retries after second failure",
			attempts:     1,
			wantDone:     false,
			wantWaitUtil: true,
		},
		{
			name:         "marks done after max attempts",
			attempts:     3,
			wantDone:     true,
			wantWaitUtil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := OnceTask[string]{
				IsCancelled: true,
				CancelledAt: now.Format(time.RFC3339),
				Attempts:    tt.attempts,
				Errors:      make([]TaskError, tt.attempts),
			}

			updates := processTaskFailure(execErr, config, task, now)

			var foundDoneAt bool
			var foundWaitUntil bool

			for _, update := range updates {
				if update.Path == "doneAt" {
					foundDoneAt = true
				}
				if update.Path == "waitUntil" {
					foundWaitUntil = true
				}
			}

			if foundDoneAt != tt.wantDone {
				t.Errorf("doneAt present = %v, want %v", foundDoneAt, tt.wantDone)
			}
			if foundWaitUntil != tt.wantWaitUtil {
				t.Errorf("waitUntil present = %v, want %v", foundWaitUntil, tt.wantWaitUtil)
			}
		})
	}
}
