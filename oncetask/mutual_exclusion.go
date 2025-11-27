package oncetask

import (
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
)

var errLeaseNotAvailable = errors.New("lease not available for task")

// queryAndLock executes a query within a transaction and re-reads the resulting documents
// to establish proper read locks for Firestore's optimistic concurrency control.
//
// This ensures that if another transaction modifies any of these documents,
// Firestore will detect the conflict at commit time.
//
// Returns the document snapshots for further processing. Use docSnap.Ref to get the document reference.
func queryAndLock(tx *firestore.Transaction, query *firestore.Query) ([]*firestore.DocumentSnapshot, error) {
	// First, run the query to find matching document IDs.
	// We only need the document refs here since we'll read the full docs during locking.
	docs, err := tx.Documents(query.Select()).GetAll()
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	if len(docs) == 0 {
		return nil, nil
	}

	// Collect document refs
	docRefs := make([]*firestore.DocumentRef, len(docs))
	for i, doc := range docs {
		docRefs[i] = doc.Ref
	}

	// Re-read all documents using tx.GetAll() to establish proper read locks
	docSnaps, err := tx.GetAll(docRefs)
	if err != nil {
		return nil, fmt.Errorf("failed to lock documents: %w", err)
	}

	return docSnaps, nil
}

// checkResourceAvailable verifies that no other task with the same ResourceKey is currently leased.
// If resourceKey is empty, the check passes (no mutual exclusion needed).
// Returns nil if resource is available, errLeaseNotAvailable if another task holds the resource.
func checkResourceAvailable(
	tx *firestore.Transaction,
	queryBuilder *firestoreQueryBuilder,
	taskType string,
	resourceKey string,
	now time.Time,
) error {
	// No mutual exclusion needed if resourceKey is empty
	if resourceKey == "" {
		return nil
	}

	query := queryBuilder.activeLeasesForResourceKey(taskType, resourceKey, now).Limit(1)

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
