package oncetask

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestContextHandler_AddsTaskID(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a JSON handler that writes to the buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Wrap it with our ContextHandler
	contextHandler := NewContextHandler(jsonHandler)
	logger := slog.New(contextHandler)

	// Create a context with a task ID
	ctx := withTaskID(context.Background(), "test-task-123")

	// Log using the context
	logger.InfoContext(ctx, "Processing task", "operation", "sync")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	// Verify that taskId was automatically added
	if taskID, ok := logEntry["taskId"].(string); !ok || taskID != "test-task-123" {
		t.Errorf("Expected taskId to be 'test-task-123', got: %v", logEntry["taskId"])
	}

	// Verify other fields
	if msg, ok := logEntry["msg"].(string); !ok || msg != "Processing task" {
		t.Errorf("Expected msg to be 'Processing task', got: %v", logEntry["msg"])
	}

	if operation, ok := logEntry["operation"].(string); !ok || operation != "sync" {
		t.Errorf("Expected operation to be 'sync', got: %v", logEntry["operation"])
	}
}

func TestContextHandler_WithoutTaskID(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a JSON handler that writes to the buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Wrap it with our ContextHandler
	contextHandler := NewContextHandler(jsonHandler)
	logger := slog.New(contextHandler)

	// Log without task ID in context
	ctx := context.Background()
	logger.InfoContext(ctx, "Regular log message")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	// Verify that taskId is NOT present
	if _, ok := logEntry["taskId"]; ok {
		t.Errorf("Expected taskId to be absent, but it was present: %v", logEntry["taskId"])
	}
}

func TestTaskIDFromContext(t *testing.T) {
	ctx := context.Background()

	// Test with no task ID
	if taskID, ok := taskIDFromContext(ctx); ok {
		t.Errorf("Expected no task ID, got: %s", taskID)
	}

	// Test with task ID
	ctx = withTaskID(ctx, "task-456")
	if taskID, ok := taskIDFromContext(ctx); !ok || taskID != "task-456" {
		t.Errorf("Expected task ID 'task-456', got: %s (ok=%v)", taskID, ok)
	}
}

func TestContextHandler_WithAttrs(t *testing.T) {
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	contextHandler := NewContextHandler(jsonHandler)

	// Create a logger with additional attributes
	logger := slog.New(contextHandler).With("service", "oncetask")

	ctx := withTaskID(context.Background(), "task-789")
	logger.InfoContext(ctx, "Test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	// Both taskId and service should be present
	if taskID, ok := logEntry["taskId"].(string); !ok || taskID != "task-789" {
		t.Errorf("Expected taskId to be 'task-789', got: %v", logEntry["taskId"])
	}

	if service, ok := logEntry["service"].(string); !ok || service != "oncetask" {
		t.Errorf("Expected service to be 'oncetask', got: %v", logEntry["service"])
	}
}
