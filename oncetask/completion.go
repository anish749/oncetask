package oncetask

import (
	"time"

	"cloud.google.com/go/firestore"
)

// processTaskSuccess generates updates for a successful task execution.
// Marks the task as done and stores the result.
func processTaskSuccess(result any, now time.Time) []firestore.Update {
	return []firestore.Update{
		{Path: "doneAt", Value: now.Format(time.RFC3339)},
		{Path: "leasedUntil", Value: ""},
		{Path: "result", Value: result},
	}
}

// processTaskFailure generates updates for a failed task execution.
// Either schedules a retry with backoff, or marks as permanently failed.
// Automatically selects the appropriate retry policy based on task.IsCancelled.
func processTaskFailure[TaskKind ~string](
	execErr error,
	config handlerConfig,
	task OnceTask[TaskKind],
	now time.Time,
) []firestore.Update {
	// Select the appropriate retry policy
	retryPolicy := config.RetryPolicy
	if task.IsCancelled {
		retryPolicy = config.CancellationRetryPolicy
	}

	newError := TaskError{
		At:    now.Format(time.RFC3339),
		Error: execErr.Error(),
	}
	task.Errors = append(task.Errors, newError)

	if retryPolicy.ShouldRetry(task.Attempts, execErr) {
		backoffDuration := retryPolicy.NextRetryDelay(task.Attempts, execErr)
		waitUntil := now.Add(backoffDuration).Format(time.RFC3339)

		return []firestore.Update{
			{Path: "leasedUntil", Value: ""},
			{Path: "waitUntil", Value: waitUntil},
			{Path: "errors", Value: task.Errors},
		}
	}

	// Permanently failed
	return []firestore.Update{
		{Path: "doneAt", Value: now.Format(time.RFC3339)},
		{Path: "leasedUntil", Value: ""},
		{Path: "errors", Value: task.Errors},
	}
}
