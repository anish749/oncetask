package oncetask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
)

// CancelTask marks a single task as cancelled.
// Idempotent: no-op if task is already done or cancelled.
// Sets isCancelled=true, cancelledAt=now, and waitUntil=NoWait for immediate processing.
//
// Cancellation behavior:
// - Sets waitUntil to NoWait (immediate) to ensure the cancellation handler runs as soon as possible.
// - This prevents the task from being executed later if it was scheduled for the future.
//
// Note on race conditions with leased tasks:
//   - If the task is currently leased (running), setting isCancelled=true and waitUntil=NoWait is still correct.
//   - The currently running handler may complete successfully after we mark it cancelled - that's acceptable.
//   - However, if the lease expires without completion, we don't want the task to be retried later.
//   - By setting waitUntil=NoWait, we ensure that once the lease expires, the task is immediately picked up
//     and either the cancellation handler runs, or it's marked as done-cancelled.
func (m *firestoreOnceTaskManager[TaskKind]) CancelTask(
	ctx context.Context,
	taskID string,
) error {
	_, err := m.CancelTasksByIds(ctx, []string{taskID})
	return err
}

// CancelTasksByResourceKey marks all non-done tasks with resourceKey as cancelled.
// Returns count of tasks cancelled.
func (m *firestoreOnceTaskManager[TaskKind]) CancelTasksByResourceKey(
	ctx context.Context,
	taskType TaskKind,
	resourceKey string,
) (int, error) {
	// Query all non-done tasks with resourceKey
	query := m.queryBuilder.
		nonDoneTasksByResourceKey(string(taskType), resourceKey).
		Select() // Empty select to retrieve only document IDs

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return 0, fmt.Errorf("failed to query tasks: %w", err)
	}

	if len(docs) == 0 {
		return 0, nil
	}

	// Collect task IDs and delegate to CancelTasksByIds
	taskIDs := make([]string, len(docs))
	for i, doc := range docs {
		taskIDs[i] = doc.Ref.ID
	}
	return m.CancelTasksByIds(ctx, taskIDs)
}

// CancelTasksByIds marks multiple tasks as cancelled (bulk operation via BulkWriter).
// Returns count of tasks cancelled. Partial failures return both count and aggregated error.
func (m *firestoreOnceTaskManager[TaskKind]) CancelTasksByIds(
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
	now := time.Now().UTC()
	env := getTaskEnv()
	var errs []error

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

		if task.DoneAt != "" || task.IsCancelled {
			continue // Already done or cancelled
		}

		updates := []firestore.Update{
			{Path: "isCancelled", Value: true},
			{Path: "cancelledAt", Value: now.Format(time.RFC3339)},
			{Path: "waitUntil", Value: NoWait},
		}

		job, err := bw.Update(docSnap.Ref, updates)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create update job for task %s: %w", docSnap.Ref.ID, err))
		}
		jobs = append(jobs, job)
	}

	bw.End()

	cancelled := 0

	for _, job := range jobs {
		if _, err := job.Results(); err == nil {
			cancelled++
		} else {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return cancelled, fmt.Errorf("partial cancellation: %d succeeded, %d failed: %w",
			cancelled, len(errs), errors.Join(errs...))
	}

	slog.InfoContext(ctx, "Cancelled tasks by IDs", "count", cancelled, "total", len(taskIDs))

	return cancelled, nil
}

// getCancellationHandler returns the registered cancellation handler or a no-op handler.
// Cancelled tasks are always processed one at a time, regardless of the normal handler type.
func getCancellationHandler[TaskKind ~string](config handlerConfig) OnceTaskHandler[TaskKind] {
	if config.cancellationTaskHandler != nil {
		if handler, ok := config.cancellationTaskHandler.(OnceTaskHandler[TaskKind]); ok {
			return handler
		}
	}
	// No cancellation handler - return no-op that succeeds immediately
	return func(ctx context.Context, task *OnceTask[TaskKind]) (any, error) {
		return nil, nil
	}
}
