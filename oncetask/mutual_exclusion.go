package oncetask

import (
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

var errLeaseNotAvailable = errors.New("lease not available for task")

// checkResourceAvailable verifies that no other task with the same ResourceKey is currently leased.
// If resourceKey is empty, the check passes (no mutual exclusion needed).
// excludeTaskID allows excluding a specific task ID from the check (e.g., the task being claimed).
// Returns nil if resource is available, errLeaseNotAvailable if another task holds the resource.
func checkResourceAvailable(
	tx *firestore.Transaction,
	queryBuilder *firestoreQueryBuilder,
	taskType string,
	resourceKey string,
	excludeTaskID string,
	now time.Time,
) error {
	// No mutual exclusion needed if resourceKey is empty
	if resourceKey == "" {
		return nil
	}

	query := queryBuilder.activeLeasesForResourceKey(taskType, resourceKey, excludeTaskID, now).Limit(1)

	docs, err := tx.Documents(query).GetAll()
	if err != nil {
		return fmt.Errorf("failed to check resource key conflict: %w", err)
	}

	if len(docs) > 0 {
		return errLeaseNotAvailable
	}

	return nil
}

// checkTaskLease verifies a specific task's lease is available (expired or empty).
// Returns nil if available, errLeaseNotAvailable if leased, or error if parsing fails.
func checkTaskLease(leasedUntil string, now time.Time) error {
	if leasedUntil == "" {
		return nil
	}

	leasedUntilTime, err := time.Parse(time.RFC3339, leasedUntil)
	if err != nil {
		return fmt.Errorf("failed to parse leasedUntil: %w", err)
	}

	if leasedUntilTime.After(now) {
		return errLeaseNotAvailable
	}

	return nil
}
