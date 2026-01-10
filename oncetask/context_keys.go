package oncetask

import (
	"context"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	taskIDContextKey      contextKey = "oncetask.taskID"
	resourceKeyContextKey contextKey = "oncetask.resourceKey"
)

// withTaskContext adds both task ID and resource key to the context for automatic logging
func withTaskContext(ctx context.Context, taskID, resourceKey string) context.Context {
	if taskID != "" {
		ctx = context.WithValue(ctx, taskIDContextKey, taskID)
	}
	if resourceKey != "" {
		ctx = context.WithValue(ctx, resourceKeyContextKey, resourceKey)
	}
	return ctx
}

func withSingleTaskContext[TaskKind ~string](ctx context.Context, tasks []OnceTask[TaskKind]) context.Context {
	if len(tasks) == 0 {
		return ctx
	}
	taskID := tasks[0].Id
	resourceKey := tasks[0].ResourceKey
	return withTaskContext(ctx, taskID, resourceKey)
}

// withResourceKeyTaskContext is used for resource key batched tasks and adds only the resource key to the context for automatic logging
// If there is only one task in the batch, the task ID is also added to the context.
func withResourceKeyTaskContext[TaskKind ~string](ctx context.Context, tasks []OnceTask[TaskKind]) context.Context {
	if len(tasks) == 0 {
		return ctx
	}
	taskID := ""
	if len(tasks) == 1 {
		taskID = tasks[0].Id
	}

	return withTaskContext(ctx, taskID, tasks[0].ResourceKey)
}

// GetCurrentTaskID returns the task ID stored in the context, or an empty string if not present.
// This is useful for debugging or when you need to access the current task ID within a handler.
func GetCurrentTaskID(ctx context.Context) string {
	if taskID, ok := ctx.Value(taskIDContextKey).(string); ok {
		return taskID
	}
	return ""
}

// GetCurrentTaskResourceKey returns the resource key stored in the context, or an empty string if not present.
// This is useful for debugging or when you need to access the current resource key within a handler.
func GetCurrentTaskResourceKey(ctx context.Context) string {
	if resourceKey, ok := ctx.Value(resourceKeyContextKey).(string); ok {
		return resourceKey
	}
	return ""
}
