package oncetask

import (
	"context"
	"time"
)

// OnceTaskHandler processes a single pending task.
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
type OnceTaskHandler[TaskKind ~string] func(ctx context.Context, task *OnceTask[TaskKind]) error

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
//   - nil: All tasks in the batch are marked as successfully completed.
//   - error: All tasks in the batch are failed and will be retried.
type OnceTaskResourceKeyHandler[TaskKind ~string] func(ctx context.Context, tasks []OnceTask[TaskKind]) error

// Lease duration for task execution
const leaseDuration = 10 * time.Minute

// OnceTaskManager defines the interface for managing once-execution tasks.
type OnceTaskManager[TaskKind ~string] interface {
	// CreateTask creates a once task to Firestore.
	// The task parameter should be created using fs_models/once.NewOnceTask().
	// If a task with the same ID already exists, logs and returns nil (idempotent).
	// Returns true if the task was created, false if it already existed.
	// Returns false if a task with the same ID already exists or error.
	CreateTask(ctx context.Context, taskData OnceTaskData[TaskKind]) (bool, error)

	// RegisterTaskHandler listens for new tasks and executes the handler function for each task.
	// The handler function is expected to successfully execute only once.
	// If the handler returns an error, the task will be retried.
	RegisterTaskHandler(taskType TaskKind, handler OnceTaskHandler[TaskKind]) error

	// RegisterResourceKeyHandler listens for new tasks and executes the handler for all tasks with the same resource key.
	// All pending tasks with the same resource key are grouped together and ordered by WaitUntil.
	// The handler is responsible for any additional ordering logic.
	// If the handler returns nil, all tasks for that resource key are marked as done.
	// If the handler returns an error, all tasks for that resource key will be retried.
	// Tasks without a resource key are processed individually.
	RegisterResourceKeyHandler(taskType TaskKind, handler OnceTaskResourceKeyHandler[TaskKind]) error

	// GetTasksByResourceKey retrieves all tasks with the given resource key.
	// Returns tasks ordered by CreatedAt (oldest first).
	// The tasks must belong to the current environment (from ONCE_TASK_ENV).
	GetTasksByResourceKey(ctx context.Context, resourceKey string) ([]OnceTask[TaskKind], error)
}
