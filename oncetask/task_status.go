package oncetask

import (
	"fmt"
	"time"
)

// TaskStatus represents the complete execution status of a task.
// Status is derived from the task's timestamp fields and retry tracking,
// combining both transient (pending, leased, waiting) and terminal (completed, failed) states.
//
// | Status         | doneAt | waitUntil    | leasedUntil  | attempts | errors      | Description                    |
// |----------------|--------|--------------|--------------|----------|-------------|--------------------------------|
// | Pending        | ""     | "" or past   | "" or past   | 0+       | varies      | Ready to execute               |
// | Waiting        | ""     | future       | "" or past   | 0+       | varies      | Scheduled for future / backoff |
// | Leased         | ""     | unchanged    | future       | 1+       | unchanged   | Currently being executed       |
// | Completed      | set    | unchanged    | ""           | N        | < N         | Last attempt succeeded         |
// | Failed         | set    | unchanged    | ""           | N        | exactly N   | All attempts failed            |
type TaskStatus string

const (
	// TaskStatusWaiting indicates the task is scheduled for future execution (waitUntil > now)
	// or is in backoff period after a failed attempt.
	TaskStatusWaiting TaskStatus = "waiting"

	// TaskStatusPending indicates the task is ready to execute immediately
	// (waitUntil <= now, not leased, not done).
	TaskStatusPending TaskStatus = "pending"

	// TaskStatusLeased indicates the task is currently being executed by a worker
	// (leasedUntil > now).
	TaskStatusLeased TaskStatus = "leased"

	// TaskStatusCompleted indicates the task has finished successfully
	// (doneAt is set, attempts > len(errors)).
	TaskStatusCompleted TaskStatus = "completed"

	// TaskStatusFailed indicates all retry attempts were exhausted without success
	// (doneAt is set, attempts == len(errors)).
	TaskStatusFailed TaskStatus = "failed"
)

// GetStatus computes and returns the complete execution status of the task.
// The status is derived from the task's timestamp fields and retry tracking
// relative to the provided current time.
//
// This method combines the semantics of both transient states (pending, leased, waiting)
// and terminal states (completed, failed).
//
// Returns an error if any timestamp field contains invalid RFC3339 format.
func (t *OnceTask[TaskKind]) GetStatus(now time.Time) (TaskStatus, error) {
	// Check terminal states first (doneAt is set)
	if t.DoneAt != "" {
		doneTime, err := time.Parse(time.RFC3339, t.DoneAt)
		if err != nil {
			return "", fmt.Errorf("failed to parse doneAt: %w", err)
		}
		if !doneTime.IsZero() {
			// Terminal state - determine success vs failure
			if t.Attempts > len(t.Errors) {
				return TaskStatusCompleted, nil
			}
			return TaskStatusFailed, nil
		}
	}

	// Check if currently leased (actively executing)
	if t.LeasedUntil != "" {
		leaseTime, err := time.Parse(time.RFC3339, t.LeasedUntil)
		if err != nil {
			return "", fmt.Errorf("failed to parse leasedUntil: %w", err)
		}
		if !leaseTime.IsZero() && leaseTime.After(now) {
			return TaskStatusLeased, nil
		}
	}

	// Check if scheduled for future (waiting or in backoff)
	if t.WaitUntil != "" {
		waitTime, err := time.Parse(time.RFC3339, t.WaitUntil)
		if err != nil {
			return "", fmt.Errorf("failed to parse waitUntil: %w", err)
		}
		if !waitTime.IsZero() && waitTime.After(now) {
			return TaskStatusWaiting, nil
		}
	}

	// PENDING: doneAt == "" AND leasedUntil expired AND waitUntil <= now
	return TaskStatusPending, nil
}
