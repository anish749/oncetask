package oncetask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"cloud.google.com/go/firestore"
)

// DeleteTask permanently removes a single task from Firestore.
// Idempotent: no error if task doesn't exist or belongs to different environment.
func (m *firestoreOnceTaskManager[TaskKind]) DeleteTask(
	ctx context.Context,
	taskID string,
) error {
	_, err := m.DeleteTasksByIds(ctx, []string{taskID})
	return err
}

// DeleteTasksByIds permanently removes multiple tasks from Firestore.
// Returns count of tasks deleted. Partial failures return both count and aggregated error.
func (m *firestoreOnceTaskManager[TaskKind]) DeleteTasksByIds(
	ctx context.Context,
	taskIDs []string,
) (int, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}

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
			continue
		}

		job, err := bw.Delete(docSnap.Ref)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to queue delete for task %s: %w", docSnap.Ref.ID, err))
			continue
		}
		jobs = append(jobs, job)
	}

	bw.End()

	deleted := 0
	for _, job := range jobs {
		if _, err := job.Results(); err == nil {
			deleted++
		} else {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return deleted, fmt.Errorf("partial deletion: %d succeeded, %d failed: %w",
			deleted, len(errs), errors.Join(errs...))
	}

	slog.InfoContext(ctx, "Deleted tasks by IDs", "count", deleted, "total", len(taskIDs))

	return deleted, nil
}
