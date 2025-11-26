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
func processTaskFailure(
	execErr error,
	currentErrors []TaskError,
	attempts int,
	retryPolicy RetryPolicy,
	now time.Time,
) []firestore.Update {
	newError := TaskError{
		At:    now.Format(time.RFC3339),
		Error: execErr.Error(),
	}
	newErrors := append(currentErrors, newError)

	if retryPolicy.ShouldRetry(attempts) {
		backoffDuration := retryPolicy.NextRetryDelay(attempts, execErr)
		waitUntil := now.Add(backoffDuration).Format(time.RFC3339)

		return []firestore.Update{
			{Path: "leasedUntil", Value: ""},
			{Path: "waitUntil", Value: waitUntil},
			{Path: "errors", Value: newErrors},
		}
	}

	// Permanently failed
	return []firestore.Update{
		{Path: "doneAt", Value: now.Format(time.RFC3339)},
		{Path: "leasedUntil", Value: ""},
		{Path: "errors", Value: newErrors},
	}
}
