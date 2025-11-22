package oncetask

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestContextHandler_WithoutContext(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a JSON handler that writes to the buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Wrap it with our ContextHandler
	contextHandler := NewContextHandler(jsonHandler)
	logger := slog.New(contextHandler)

	// Log without task context
	ctx := context.Background()
	logger.InfoContext(ctx, "Regular log message")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	// Verify that taskId and resourceKey are NOT present
	if _, ok := logEntry["taskId"]; ok {
		t.Errorf("Expected taskId to be absent, but it was present: %v", logEntry["taskId"])
	}
	if _, ok := logEntry["resourceKey"]; ok {
		t.Errorf("Expected resourceKey to be absent, but it was present: %v", logEntry["resourceKey"])
	}
}

func TestContextHandler_WithTaskIDOnly(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a JSON handler that writes to the buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Wrap it with our ContextHandler
	contextHandler := NewContextHandler(jsonHandler)
	logger := slog.New(contextHandler)

	// Create a context with only task ID
	ctx := withTaskContext(context.Background(), "test-task-123", "")

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

	// Verify resourceKey is NOT present
	if _, ok := logEntry["resourceKey"]; ok {
		t.Errorf("Expected resourceKey to be absent, but it was present: %v", logEntry["resourceKey"])
	}

	// Verify other fields
	if msg, ok := logEntry["msg"].(string); !ok || msg != "Processing task" {
		t.Errorf("Expected msg to be 'Processing task', got: %v", logEntry["msg"])
	}

	if operation, ok := logEntry["operation"].(string); !ok || operation != "sync" {
		t.Errorf("Expected operation to be 'sync', got: %v", logEntry["operation"])
	}
}

func TestContextHandler_WithResourceKeyOnly(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a JSON handler that writes to the buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Wrap it with our ContextHandler
	contextHandler := NewContextHandler(jsonHandler)
	logger := slog.New(contextHandler)

	// Create a context with only resource key (batch processing scenario)
	ctx := withTaskContext(context.Background(), "", "user-123")

	// Log using the context
	logger.InfoContext(ctx, "Processing resource batch", "operation", "update")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	// Verify that resourceKey was automatically added
	if resourceKey, ok := logEntry["resourceKey"].(string); !ok || resourceKey != "user-123" {
		t.Errorf("Expected resourceKey to be 'user-123', got: %v", logEntry["resourceKey"])
	}

	// Verify taskId is NOT present
	if _, ok := logEntry["taskId"]; ok {
		t.Errorf("Expected taskId to be absent, but it was present: %v", logEntry["taskId"])
	}

	// Verify other fields
	if msg, ok := logEntry["msg"].(string); !ok || msg != "Processing resource batch" {
		t.Errorf("Expected msg to be 'Processing resource batch', got: %v", logEntry["msg"])
	}

	if operation, ok := logEntry["operation"].(string); !ok || operation != "update" {
		t.Errorf("Expected operation to be 'update', got: %v", logEntry["operation"])
	}
}

func TestContextHandler_WithBothTaskIDAndResourceKey(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer

	// Create a JSON handler that writes to the buffer
	jsonHandler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})

	// Wrap it with our ContextHandler
	contextHandler := NewContextHandler(jsonHandler)
	logger := slog.New(contextHandler)

	// Create a context with both task ID and resource key
	ctx := withTaskContext(context.Background(), "task-999", "user-456")

	// Log using the context
	logger.InfoContext(ctx, "Processing task with resource key")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log output: %v", err)
	}

	// Verify that both taskId and resourceKey were automatically added
	if taskID, ok := logEntry["taskId"].(string); !ok || taskID != "task-999" {
		t.Errorf("Expected taskId to be 'task-999', got: %v", logEntry["taskId"])
	}

	if resourceKey, ok := logEntry["resourceKey"].(string); !ok || resourceKey != "user-456" {
		t.Errorf("Expected resourceKey to be 'user-456', got: %v", logEntry["resourceKey"])
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

	ctx := withTaskContext(context.Background(), "task-789", "")
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
