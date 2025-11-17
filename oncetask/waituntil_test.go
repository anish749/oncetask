package oncetask

import (
	"testing"
	"time"
)

// mockScheduledTaskData is a mock task data that implements ScheduledTask
type mockScheduledTaskData struct {
	id            string
	scheduledTime time.Time
}

func (m *mockScheduledTaskData) GetType() string {
	return "mockScheduled"
}

func (m *mockScheduledTaskData) GenerateIdempotentID() string {
	return m.id
}

func (m *mockScheduledTaskData) GetScheduledTime() time.Time {
	return m.scheduledTime
}

// mockNormalTaskData is a mock task data that does NOT implement ScheduledTask
type mockNormalTaskData struct {
	id string
}

func (m *mockNormalTaskData) GetType() string {
	return "mockNormal"
}

func (m *mockNormalTaskData) GenerateIdempotentID() string {
	return m.id
}

// TestNewOnceTask_WithScheduledTask verifies that NewOnceTask populates waitUntil
// when task data implements ScheduledTask
func TestNewOnceTask_WithScheduledTask(t *testing.T) {
	scheduledTime := time.Now().Add(1 * time.Hour).UTC()
	taskData := &mockScheduledTaskData{
		id:            "test-scheduled-123",
		scheduledTime: scheduledTime,
	}

	task, err := newOnceTask(taskData)
	if err != nil {
		t.Fatalf("NewOnceTask failed: %v", err)
	}

	if task.WaitUntil == "" {
		t.Error("Expected WaitUntil to be set, got empty string")
	}

	// Parse and verify the timestamp
	parsedTime, err := time.Parse(time.RFC3339, task.WaitUntil)
	if err != nil {
		t.Errorf("Failed to parse WaitUntil timestamp: %v", err)
	}

	// Allow 1 second difference for rounding
	if parsedTime.Sub(scheduledTime).Abs() > 1*time.Second {
		t.Errorf("Expected WaitUntil to be %v, got %v", scheduledTime, parsedTime)
	}
}

// TestNewOnceTask_WithZeroScheduledTime verifies that NewOnceTask uses epoch time
// when GetScheduledTime returns zero time (immediate execution)
func TestNewOnceTask_WithZeroScheduledTime(t *testing.T) {
	taskData := &mockScheduledTaskData{
		id:            "test-zero-time",
		scheduledTime: time.Time{}, // Zero time
	}

	task, err := newOnceTask(taskData)
	if err != nil {
		t.Fatalf("NewOnceTask failed: %v", err)
	}

	expectedEpoch := time.Time{}.Format(time.RFC3339)
	if task.WaitUntil != expectedEpoch {
		t.Errorf("Expected WaitUntil to be epoch time %s, got %s", expectedEpoch, task.WaitUntil)
	}
}

// TestNewOnceTask_WithoutScheduledTask verifies that NewOnceTask uses epoch time
// for immediate execution when task data does NOT implement ScheduledTask
func TestNewOnceTask_WithoutScheduledTask(t *testing.T) {
	taskData := &mockNormalTaskData{
		id: "test-normal-456",
	}

	task, err := newOnceTask(taskData)
	if err != nil {
		t.Fatalf("NewOnceTask failed: %v", err)
	}

	expectedEpoch := time.Time{}.Format(time.RFC3339)
	if task.WaitUntil != expectedEpoch {
		t.Errorf("Expected WaitUntil to be epoch time %s, got %s", expectedEpoch, task.WaitUntil)
	}
}
