package oncetask

import (
	"context"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	taskIDContextKey      contextKey = "oncetask.taskID"
	resourceKeyContextKey contextKey = "oncetask.resourceKey"
	taskTypeContextKey    contextKey = "oncetask.taskType"
)

// withTaskContext adds task ID, resource key, and task type to the context for automatic logging
func withTaskContext(ctx context.Context, taskID, resourceKey, taskType string) context.Context {
	if taskID != "" {
		ctx = context.WithValue(ctx, taskIDContextKey, taskID)
	}
	if resourceKey != "" {
		ctx = context.WithValue(ctx, resourceKeyContextKey, resourceKey)
	}
	ctx = context.WithValue(ctx, taskTypeContextKey, taskType)
	return ctx
}

func withSingleTaskContext[TaskKind ~string](ctx context.Context, tasks []OnceTask[TaskKind]) context.Context {
	if len(tasks) == 0 {
		return ctx
	}
	return withTaskContext(ctx, tasks[0].Id, tasks[0].ResourceKey, string(tasks[0].Type))
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

	return withTaskContext(ctx, taskID, tasks[0].ResourceKey, string(tasks[0].Type))
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
