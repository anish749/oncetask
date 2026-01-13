package oncetask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"cloud.google.com/go/firestore"
)

// ResetTask resets a single task back to pending state for re-execution.
// Only applies to tasks in terminal states (doneAt != "").
// Idempotent: no-op if task is already pending/running.
//
// Reset behavior:
// - Clears all execution state: Attempts, Errors, DoneAt, LeasedUntil, Result
// - Clears cancellation state: IsCancelled, CancelledAt
// - Sets WaitUntil=NoWait for immediate execution
// - Triggers evaluation to wake up workers
//
// This allows re-execution of:
// - COMPLETED tasks (e.g., to reprocess data after bug fix)
// - FAILED tasks (e.g., to retry after fixing root cause)
// - CANCELLED tasks (e.g., to resume after cancellation was reverted)
func (m *firestoreOnceTaskManager[TaskKind]) ResetTask(
	ctx context.Context,
	taskID string,
) error {
	_, err := m.ResetTasksByIds(ctx, []string{taskID})
	return err
}

// ResetTasksByResourceKey resets all terminal-state tasks with resourceKey back to pending.
// Returns count of tasks reset.
// Only affects tasks in terminal states (doneAt != "").
func (m *firestoreOnceTaskManager[TaskKind]) ResetTasksByResourceKey(
	ctx context.Context,
	taskType TaskKind,
	resourceKey string,
) (int, error) {
	// Query all done tasks with resourceKey
	query := m.queryBuilder.
		doneTasksByResourceKey(string(taskType), resourceKey).
		Select() // Empty select to retrieve only document IDs

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return 0, fmt.Errorf("failed to query tasks: %w", err)
	}

	if len(docs) == 0 {
		return 0, nil
	}

	// Collect task IDs and delegate to ResetTasksByIds
	taskIDs := make([]string, len(docs))
	for i, doc := range docs {
		taskIDs[i] = doc.Ref.ID
	}
	return m.ResetTasksByIds(ctx, taskIDs)
}

// ResetTasksByIds resets multiple tasks back to pending state (bulk operation via BulkWriter).
// Returns count of tasks reset. Partial failures return both count and aggregated error.
// Only resets tasks in terminal states (doneAt != "").
//
// Reset clears all execution and cancellation state:
// - Attempts = 0
// - Errors = []
// - DoneAt = ""
// - LeasedUntil = ""
// - WaitUntil = NoWait (immediate execution)
// - IsCancelled = false
// - CancelledAt = ""
// - Result = nil
//
// Idempotent: Tasks already in non-terminal states are skipped (no-op).
func (m *firestoreOnceTaskManager[TaskKind]) ResetTasksByIds(
	ctx context.Context,
	taskIDs []string,
) (int, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}

	// Fetch all tasks first
	docRefs := make([]*firestore.DocumentRef, len(taskIDs))
	for i, id := range taskIDs {
		docRefs[i] = m.queryBuilder.doc(id)
	}

	docSnaps, err := m.client.GetAll(ctx, docRefs)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch tasks: %w", err)
	}

	bw := m.client.BulkWriter(ctx)
	jobs := make([]*firestore.BulkWriterJob, 0, len(docSnaps))
	env := getTaskEnv()
	var errs []error
	taskTypes := make(map[TaskKind]struct{})

	for _, docSnap := range docSnaps {
		if !docSnap.Exists() {
			continue
		}

		var task OnceTask[TaskKind]
		if err := docSnap.DataTo(&task); err != nil {
			errs = append(errs, fmt.Errorf("failed to parse task %s: %w", docSnap.Ref.ID, err))
			continue
		}

		if task.Env != env {
			continue // Environment isolation
		}

		if task.DoneAt == "" {
			continue // Not in terminal state - already pending/running
		}

		// Reset all execution and cancellation state
		updates := []firestore.Update{
			{Path: "attempts", Value: 0},
			{Path: "errors", Value: []TaskError{}},
			{Path: "doneAt", Value: ""},
			{Path: "leasedUntil", Value: ""},
			{Path: "waitUntil", Value: NoWait},
			{Path: "isCancelled", Value: false},
			{Path: "cancelledAt", Value: ""},
			{Path: "result", Value: nil},
		}

		job, err := bw.Update(docSnap.Ref, updates)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create update job for task %s: %w", docSnap.Ref.ID, err))
		} else {
			jobs = append(jobs, job)
			taskTypes[task.Type] = struct{}{}
		}
	}

	bw.End()

	resetCount := 0

	for _, job := range jobs {
		if _, err := job.Results(); err == nil {
			resetCount++
		} else {
			errs = append(errs, err)
		}
	}

	// Trigger evaluation for all affected task types to wake up workers
	for taskType := range taskTypes {
		m.evaluateNow(taskType)
	}

	if len(errs) > 0 {
		return resetCount, fmt.Errorf("partial reset: %d succeeded, %d failed: %w",
			resetCount, len(errs), errors.Join(errs...))
	}

	slog.InfoContext(ctx, "Reset tasks by IDs", "count", resetCount, "total", len(taskIDs))

	return resetCount, nil
}
