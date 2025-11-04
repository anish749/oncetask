package oncetask

import (
	"context"
	"time"
)

type OnceTaskHandler[TaskKind ~string] func(ctx context.Context, task *OnceTask[TaskKind]) error

// Lease duration for task execution
const leaseDuration = 10 * time.Minute

// OnceTaskRepository defines the interface for managing once-execution tasks.
type OnceTaskManager[TaskKind ~string] interface {
	// CreateTask creates a once task to Firestore.
	// The task parameter should be created using fs_models/once.NewOnceTask().
	// If a task with the same ID already exists, logs and returns nil (idempotent).
	CreateTask(ctx context.Context, taskData OnceTaskData[TaskKind]) error

	// RegisterTaskHandler listens for new tasks and executes the handler function for each task.
	// The handler function is expected to successfully execute only once.
	// If the handler returns an error, the task will be retried.
	RegisterTaskHandler(
		taskType TaskKind,
		handler OnceTaskHandler[TaskKind],
	) error
}
