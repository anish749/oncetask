package oncetask

import (
	"context"
	"testing"
)

func TestTaskIDFromContext(t *testing.T) {
	t.Run("returns empty string when no task ID in context", func(t *testing.T) {
		ctx := context.Background()
		if got := GetCurrentTaskID(ctx); got != "" {
			t.Errorf("Expected empty string, got: %q", got)
		}
	})

	t.Run("returns task ID when present in context", func(t *testing.T) {
		ctx := withTaskContext(context.Background(), "test-task-123", "")
		if got := GetCurrentTaskID(ctx); got != "test-task-123" {
			t.Errorf("Expected 'test-task-123', got: %q", got)
		}
	})

	t.Run("returns task ID when both task ID and resource key are present", func(t *testing.T) {
		ctx := withTaskContext(context.Background(), "task-456", "resource-789")
		if got := GetCurrentTaskID(ctx); got != "task-456" {
			t.Errorf("Expected 'task-456', got: %q", got)
		}
	})
}

func TestResourceKeyFromContext(t *testing.T) {
	t.Run("returns empty string when no resource key in context", func(t *testing.T) {
		ctx := context.Background()
		if got := GetCurrentTaskResourceKey(ctx); got != "" {
			t.Errorf("Expected empty string, got: %q", got)
		}
	})

	t.Run("returns resource key when present in context", func(t *testing.T) {
		ctx := withTaskContext(context.Background(), "", "user-123")
		if got := GetCurrentTaskResourceKey(ctx); got != "user-123" {
			t.Errorf("Expected 'user-123', got: %q", got)
		}
	})

	t.Run("returns resource key when both task ID and resource key are present", func(t *testing.T) {
		ctx := withTaskContext(context.Background(), "task-456", "resource-789")
		if got := GetCurrentTaskResourceKey(ctx); got != "resource-789" {
			t.Errorf("Expected 'resource-789', got: %q", got)
		}
	})
}
