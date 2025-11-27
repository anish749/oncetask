package oncetask

import (
	"context"
)

// OnceTaskHandler processes a single pending task.
// Returns (result, nil) on success. Result can be nil if no output is needed.
// Returns (nil, error) on failure; task will be retried according to config.
//
// One task kind can either have a single handler OR a resource key handler, but not both.
//
// This handler supports two execution strategies (determined dynamically by the task's ResourceKey):
//
//  1. Concurrent (ResourceKey is empty):
//     Multiple tasks of the same kind execute concurrently.
//     No locking constraints between tasks.
//
//  2. OnePerResourceKey (ResourceKey is non-empty):
//     Only one task with the same ResourceKey executes at a time.
//     Execution is serialized (queued) for that specific ResourceKey - ordering not guaranteed.
type OnceTaskHandler[TaskKind ~string] func(ctx context.Context, task *OnceTask[TaskKind]) (any, error)

// OnceTaskResourceKeyHandler processes all pending tasks with the
// same non-empty resource key in a single execution (Pipelined Execution).
//
// This handler corresponds to the "AllPerResourceKey" strategy:
//   - All pending tasks for a specific ResourceKey are claimed and executed together.
//   - Tasks are ordered by WaitUntil timestamp. The handler is responsible for any additional ordering logic.
//
// If the ResourceKey is empty, the handler behaves like a single task handler (Concurrent strategy).
//
// Return Values:
//   - (result, nil): All tasks in the batch are marked as successfully completed. Result is stored in each task's Result field.
//   - (nil, error): All tasks in the batch are failed and will be retried.
type OnceTaskResourceKeyHandler[TaskKind ~string] func(ctx context.Context, tasks []OnceTask[TaskKind]) (any, error)

// NoResult adapts a single-task handler that doesn't return a result.
// Use this for handlers that only need to return an error on failure.
func NoResult[TaskKind ~string](fn func(ctx context.Context, task *OnceTask[TaskKind]) error) OnceTaskHandler[TaskKind] {
	return func(ctx context.Context, task *OnceTask[TaskKind]) (any, error) {
		return nil, fn(ctx, task)
	}
}

// NoResultResourceKey adapts a resource-key handler that doesn't return a result.
// Use this for handlers that only need to return an error on failure.
func NoResultResourceKey[TaskKind ~string](fn func(ctx context.Context, tasks []OnceTask[TaskKind]) error) OnceTaskResourceKeyHandler[TaskKind] {
	return func(ctx context.Context, tasks []OnceTask[TaskKind]) (any, error) {
		return nil, fn(ctx, tasks)
	}
}

// OnceTaskManager defines the interface for managing once-execution tasks.
type OnceTaskManager[TaskKind ~string] interface {
	// CreateTask creates a once task to Firestore.
	// The task parameter should be created using fs_models/once.NewOnceTask().
	// If a task with the same ID already exists, logs and returns nil (idempotent).
	// Returns true if the task was created, false if it already existed.
	// Returns false if a task with the same ID already exists or error.
	CreateTask(ctx context.Context, taskData OnceTaskData[TaskKind]) (bool, error)

	// CreateTasks creates multiple once tasks using Firestore BulkWriter.
	//
	// This method is NON-ATOMIC: individual task creations can succeed or fail independently.
	// This is intentional - it allows partial success when some tasks already exist or fail,
	// rather than failing the entire batch.
	//
	// Idempotency: Tasks that already exist (same ID) are silently skipped and do not
	// contribute to the returned error. This makes it safe to retry the entire batch.
	//
	// Returns:
	//   - (created count, nil) if all tasks were created or already existed
	//   - (created count, error) if at least one task failed for a reason other than already-existing
	//
	// The returned error aggregates all non-AlreadyExists failures using errors.Join.
	CreateTasks(ctx context.Context, taskDataList []OnceTaskData[TaskKind]) (int, error)

	// RegisterTaskHandler listens for new tasks and executes the handler function for each task.
	// Handler returns (result, nil) on success or (nil, error) on failure.
	// Use NoResult() adapter for handlers that don't produce a result.
	//
	// Configuration options (see HandlerOption for details and examples):
	//   - WithRetryPolicy: Configure retry behavior for task execution
	//   - WithCancellationHandler: Register a cleanup handler for cancelled tasks (optional)
	//   - WithCancellationRetryPolicy: Configure retry behavior for cancellation handlers
	//   - WithLeaseDuration: Set how long a task is leased during execution
	//   - WithConcurrency: Set number of concurrent workers
	RegisterTaskHandler(taskType TaskKind, handler OnceTaskHandler[TaskKind], opts ...HandlerOption) error

	// RegisterResourceKeyHandler listens for new tasks and executes the handler for all tasks with the same resource key.
	// All pending tasks with the same resource key are grouped together and ordered by WaitUntil.
	// The handler is responsible for any additional ordering logic.
	// If the handler returns nil, all tasks for that resource key are marked as done.
	// If the handler returns an error, all tasks for that resource key will be retried.
	// Tasks without a resource key are processed individually.
	// Handler returns (result, nil) on success or (nil, error) on failure.
	// Use NoResultResourceKey() adapter for handlers that don't produce a result.
	//
	// Configuration options (see HandlerOption for details and examples):
	//   - WithRetryPolicy: Configure retry behavior for task execution
	//   - WithCancellationHandler: Register a cleanup handler for cancelled tasks (optional)
	//   - WithCancellationRetryPolicy: Configure retry behavior for cancellation handlers
	//   - WithLeaseDuration: Set how long a task is leased during execution
	//   - WithConcurrency: Set number of concurrent workers
	RegisterResourceKeyHandler(taskType TaskKind, handler OnceTaskResourceKeyHandler[TaskKind], opts ...HandlerOption) error

	// GetTasksByResourceKey retrieves all tasks with the given resource key.
	// Returns tasks ordered by CreatedAt (oldest first).
	// The tasks must belong to the current environment (from ONCE_TASK_ENV).
	GetTasksByResourceKey(ctx context.Context, resourceKey string) ([]OnceTask[TaskKind], error)

	// GetTasksByIds retrieves tasks by their IDs.
	// Returns tasks in no guaranteed order.
	// Tasks not found are omitted from the result (no error).
	// The tasks must belong to the current environment (from ONCE_TASK_ENV).
	GetTasksByIds(ctx context.Context, ids []string) ([]OnceTask[TaskKind], error)

	// CancelTask marks a single task as cancelled.
	// Idempotent: no-op if task is already done or cancelled.
	// Sets isCancelled=true, cancelledAt=now, waitUntil=epoch (immediate execution).
	CancelTask(ctx context.Context, taskID string) error

	// CancelTasksByResourceKey marks all non-done tasks with resourceKey as cancelled.
	// Returns count of tasks cancelled.
	CancelTasksByResourceKey(ctx context.Context, taskType TaskKind, resourceKey string) (int, error)

	// CancelTasksByIds marks multiple tasks as cancelled (bulk operation via BulkWriter).
	// Returns count of tasks cancelled. Partial failures return both count and aggregated error.
	CancelTasksByIds(ctx context.Context, taskIDs []string) (int, error)
}
