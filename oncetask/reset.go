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
//
// Returns an error if:
// - Task does not exist (ResetStatusNotFound)
// - Task exists in a different environment (ResetStatusDifferentEnv)
// - Error occurred during reset operation (ResetStatusError)
//
// Returns nil (success) if:
// - Task was successfully reset (ResetStatusSuccess)
// - Task is already pending/running (ResetStatusNotTerminal, idempotent)
func (m *firestoreOnceTaskManager[TaskKind]) ResetTask(
	ctx context.Context,
	taskID string,
) error {
	result := m.ResetTasksByIds(ctx, []string{taskID})

	if len(result.Results) == 0 {
		return fmt.Errorf("task %s: no result returned", taskID)
	}

	taskResult := result.Results[0]

	switch taskResult.Status {
	case ResetStatusSuccess:
		return nil
	case ResetStatusNotTerminal:
		// Idempotent case - task is already pending/running
		return nil
	case ResetStatusNotFound:
		return fmt.Errorf("task %s does not exist", taskID)
	case ResetStatusDifferentEnv:
		return fmt.Errorf("task %s exists in a different environment", taskID)
	case ResetStatusError:
		return fmt.Errorf("task %s: %w", taskID, taskResult.Error)
	default:
		return fmt.Errorf("task %s: unknown reset status %s", taskID, taskResult.Status)
	}
}

// ResetTasksByIds resets multiple tasks back to pending state (bulk operation via BulkWriter).
// Returns ResetTasksResult containing the result for each task.
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
// Possible statuses for each task:
// - ResetStatusSuccess: Task was successfully reset
// - ResetStatusNotFound: Task does not exist
// - ResetStatusDifferentEnv: Task exists in a different environment
// - ResetStatusNotTerminal: Task is already pending/running (idempotent)
// - ResetStatusError: Error occurred during reset
func (m *firestoreOnceTaskManager[TaskKind]) ResetTasksByIds(
	ctx context.Context,
	taskIDs []string,
) ResetTasksResult {
	results := make([]ResetResult, 0, len(taskIDs))

	if len(taskIDs) == 0 {
		return ResetTasksResult{Results: results}
	}

	// Fetch all tasks first
	docRefs := make([]*firestore.DocumentRef, len(taskIDs))
	taskIDToIndex := make(map[string]int, len(taskIDs))
	for i, id := range taskIDs {
		docRefs[i] = m.queryBuilder.doc(id)
		taskIDToIndex[id] = i
		// Pre-populate results with not found status
		results = append(results, ResetResult{
			TaskID: id,
			Status: ResetStatusNotFound,
		})
	}

	docSnaps, err := m.client.GetAll(ctx, docRefs)
	if err != nil {
		// If we can't fetch tasks at all, mark all as errors
		for i := range results {
			results[i].Status = ResetStatusError
			results[i].Error = fmt.Errorf("failed to fetch tasks: %w", err)
		}
		return ResetTasksResult{Results: results}
	}

	bw := m.client.BulkWriter(ctx)
	type jobInfo struct {
		job      *firestore.BulkWriterJob
		taskID   string
		taskType TaskKind
	}
	jobInfos := make([]jobInfo, 0, len(docSnaps))
	env := getTaskEnv()
	taskTypes := make(map[TaskKind]struct{})

	for _, docSnap := range docSnaps {
		taskID := docSnap.Ref.ID
		idx := taskIDToIndex[taskID]

		if !docSnap.Exists() {
			results[idx] = ResetResult{
				TaskID: taskID,
				Status: ResetStatusNotFound,
			}
			continue
		}

		var task OnceTask[TaskKind]
		if err := docSnap.DataTo(&task); err != nil {
			results[idx] = ResetResult{
				TaskID: taskID,
				Status: ResetStatusError,
				Error:  fmt.Errorf("failed to parse task: %w", err),
			}
			continue
		}

		if task.Env != env {
			results[idx] = ResetResult{
				TaskID: taskID,
				Status: ResetStatusDifferentEnv,
			}
			continue
		}

		if task.DoneAt == "" {
			results[idx] = ResetResult{
				TaskID: taskID,
				Status: ResetStatusNotTerminal,
			}
			continue
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
			results[idx] = ResetResult{
				TaskID: taskID,
				Status: ResetStatusError,
				Error:  fmt.Errorf("failed to create update job: %w", err),
			}
		} else {
			jobInfos = append(jobInfos, jobInfo{
				job:      job,
				taskID:   taskID,
				taskType: task.Type,
			})
			taskTypes[task.Type] = struct{}{}
		}
	}

	bw.End()

	// Process job results
	for _, info := range jobInfos {
		idx := taskIDToIndex[info.taskID]
		if _, err := info.job.Results(); err == nil {
			results[idx] = ResetResult{
				TaskID: info.taskID,
				Status: ResetStatusSuccess,
			}
		} else {
			results[idx] = ResetResult{
				TaskID: info.taskID,
				Status: ResetStatusError,
				Error:  err,
			}
		}
	}

	// Trigger evaluation for all affected task types to wake up workers
	for taskType := range taskTypes {
		m.evaluateNow(taskType)
	}

	result := ResetTasksResult{Results: results}
	slog.InfoContext(ctx, "Reset tasks by IDs",
		"resetCount", result.ResetCount(),
		"total", len(taskIDs))

	return result
}
