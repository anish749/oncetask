package oncetask

import (
	"fmt"
	"time"
)

// TaskStatus represents the complete execution status of a task.
// Status is derived from the task's timestamp fields and retry tracking,
// combining both transient (pending, leased, waiting) and terminal (completed, failed, cancelled) states.
//
// | Status              | doneAt | isCancelled | leasedUntil  | waitUntil    | Description                              |
// |---------------------|--------|-------------|--------------|--------------|------------------------------------------|
// | cancellationPending | ""     | true        | "" or past   | "" or past   | Cancelled, waiting for handler           |
// | cancelled           | set    | true        | ""           | unchanged    | Cancelled and marked done                |
// | pending             | ""     | false       | "" or past   | "" or past   | Ready to execute                         |
// | waiting             | ""     | false       | future       | "" or past   | Scheduled for future / backoff           |
// | leased              | ""     | false       | future       | unchanged    | Currently being executed                 |
// | completed           | set    | false       | ""           | unchanged    | Last attempt succeeded                   |
// | failed              | set    | false       | ""           | unchanged    | All attempts failed                      |
type TaskStatus string

const (
	// TaskStatusWaiting indicates the task is scheduled for future execution (waitUntil > now)
	// or is in backoff period after a failed attempt.
	TaskStatusWaiting TaskStatus = "waiting"

	// TaskStatusPending indicates the task is ready to execute immediately
	// (waitUntil <= now, not leased, not done, not cancelled).
	TaskStatusPending TaskStatus = "pending"

	// TaskStatusLeased indicates the task is currently being executed by a worker
	// (leasedUntil > now, not cancelled).
	TaskStatusLeased TaskStatus = "leased"

	// TaskStatusCancellationPending indicates the task was cancelled but not yet marked as done
	// (isCancelled is true, doneAt is not set). The cancellation handler will run.
	TaskStatusCancellationPending TaskStatus = "cancellationPending"

	// TaskStatusCompleted indicates the task has finished successfully
	// (doneAt is set, isCancelled is false, attempts > len(errors)).
	TaskStatusCompleted TaskStatus = "completed"

	// TaskStatusFailed indicates all retry attempts were exhausted without success
	// (doneAt is set, isCancelled is false, attempts == len(errors)).
	TaskStatusFailed TaskStatus = "failed"

	// TaskStatusCancelled indicates the task was cancelled and marked as done
	// (doneAt is set, isCancelled is true).
	TaskStatusCancelled TaskStatus = "cancelled"
)

// GetStatus computes and returns the complete execution status of the task.
// The status is derived from the task's timestamp fields and retry tracking
// relative to the provided current time.
//
// This method combines the semantics of both transient states (pending, leased, waiting, cancellation_pending)
// and terminal states (completed, failed, cancelled).
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
			// Check cancellation before completed/failed
			if t.IsCancelled {
				return TaskStatusCancelled, nil
			}
			// Terminal state - determine success vs failure
			if t.Attempts > len(t.Errors) {
				return TaskStatusCompleted, nil
			}
			return TaskStatusFailed, nil
		}
	}

	// Check if task is cancelled but not yet done (cancellation pending)
	if t.IsCancelled {
		return TaskStatusCancellationPending, nil
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

	// PENDING: doneAt == "" AND isCancelled == false AND leasedUntil expired AND waitUntil <= now
	return TaskStatusPending, nil
}
