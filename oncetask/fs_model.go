package oncetask

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const (
	CollectionOnceTasks string = "onceTasks"
	EnvVariable         string = "ONCE_TASK_ENV"
	DefaultEnv          string = "DEFAULT"
)

// TaskState represents the derived execution state of a task.
// States are computed from the task's timestamp fields, not stored directly.
type TaskState string

const (
	// TaskStateWaiting indicates the task is scheduled for future execution (waitUntil > now)
	TaskStateWaiting TaskState = "waiting"
	// TaskStatePending indicates the task is ready to execute (waitUntil <= now, not leased, not done)
	TaskStatePending TaskState = "pending"
	// TaskStateLeased indicates the task is currently being executed (leasedUntil > now)
	TaskStateLeased TaskState = "leased"
	// TaskStateCompleted indicates the task has finished execution (doneAt is set)
	TaskStateCompleted TaskState = "completed"
)

// getTaskEnv returns the task environment from the `EnvVariable` environment variable.
// If not set, returns `DefaultEnv`.
func getTaskEnv() string {
	env := os.Getenv(EnvVariable)
	if env == "" {
		return DefaultEnv
	}
	return env
}

// Once Queue is a set of tools and utilities
// used to execute something only once, asynchronously.
type OnceTask[TaskKind ~string] struct {
	Id   string                 `json:"id" firestore:"id"` // Also the idempotency key.
	Type TaskKind               `json:"type" firestore:"type"`
	Data map[string]interface{} `json:"data" firestore:"data"`

	// Optional - identifies a resource that requires serialization (e.g., calendarId, conversationId)
	// When set, lease acquisition checks for active leases on other tasks with the same ResourceKey
	ResourceKey string `json:"resourceKey" firestore:"resourceKey"`

	// Environment identifier for logical separation of tasks (e.g., "dev", "staging", "prod")
	// Read from `EnvVariable` (ONCE_TASK_ENV) environment variable, defaults to "DEFAULT"
	Env string `json:"env" firestore:"env"`

	WaitUntil   string `json:"waitUntil" firestore:"waitUntil"`     // ISO 8601 - wait until this time to execute the task.
	LeasedUntil string `json:"leasedUntil" firestore:"leasedUntil"` // ISO 8601 - lease expiration for the task executor.
	CreatedAt   string `json:"createdAt" firestore:"createdAt"`     // ISO 8601
	DoneAt      string `json:"doneAt" firestore:"doneAt"`           // ISO 8601
}

// OnceTaskData defines the interface for task-specific data that can be stored in OnceTask.
// Each implementation represents a specific type of once-execution task with its own data structure.
type OnceTaskData[TaskKind comparable] interface {
	GetType() TaskKind

	// Generate a deterministic, idempotent ID based on the task's natural key.
	// This ensures that reprocessing the same task produces the same task ID,
	// allowing for safe overwrites instead of creating duplicates.
	GenerateIdempotentID() string
}

// ResourceKeyProvider is an optional interface that OnceTaskData implementations can implement
// to enable resource-level serialization. When GetResourceKey() returns a non-empty string,
// lease acquisition will check for active leases on other tasks with the same ResourceKey,
// ensuring only one task executes at a time per resource (e.g., per calendarId or conversationId).
// If GetResourceKey() returns an empty string or the interface is not implemented,
// task-level leasing is used (only one handler processes the specific task).
type ResourceKeyProvider interface {
	GetResourceKey() string
}

// ScheduledTask is an optional interface that OnceTaskData implementations can implement
// to specify a scheduled time for the task. When GetScheduledTime() returns a non-empty time,
// the task will not be executed until the specified time.
type ScheduledTask interface {
	GetScheduledTime() time.Time
}

// This allows for reading the data field into a specific type.
func (t *OnceTask[TaskKind]) ReadInto(v OnceTaskData[TaskKind]) error {
	if t.Type != v.GetType() {
		return fmt.Errorf("expected task type %s, got %s", v.GetType(), t.Type)
	}
	jsonBytes, err := json.Marshal(t.Data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonBytes, v)
}

// GetTaskState computes and returns the current execution state of the task.
// The state is derived from the task's timestamp fields relative to the provided current time.
// This method mirrors the frontend's getTaskStatus() logic for consistency.
// Returns an error if any timestamp field contains invalid RFC3339 format.
func (t *OnceTask[TaskKind]) GetTaskState(now time.Time) (TaskState, error) {
	// Parse and check doneAt
	if t.DoneAt != "" {
		doneTime, err := time.Parse(time.RFC3339, t.DoneAt)
		if err != nil {
			return "", fmt.Errorf("failed to parse doneAt: %w", err)
		}
		if !doneTime.IsZero() {
			return TaskStateCompleted, nil
		}
	}

	// Parse and check leasedUntil
	if t.LeasedUntil != "" {
		leaseTime, err := time.Parse(time.RFC3339, t.LeasedUntil)
		if err != nil {
			return "", fmt.Errorf("failed to parse leasedUntil: %w", err)
		}
		if !leaseTime.IsZero() && leaseTime.After(now) {
			return TaskStateLeased, nil
		}
	}

	// Parse and check waitUntil
	if t.WaitUntil != "" {
		waitTime, err := time.Parse(time.RFC3339, t.WaitUntil)
		if err != nil {
			return "", fmt.Errorf("failed to parse waitUntil: %w", err)
		}
		if !waitTime.IsZero() && waitTime.After(now) {
			return TaskStateWaiting, nil
		}
	}

	// PENDING: doneAt == "" AND leasedUntil expired AND waitUntil <= now
	return TaskStatePending, nil
}

// Use CreateTask() on the OnceTaskManager to create a new OnceTask.
func newOnceTask[TaskKind ~string](taskData OnceTaskData[TaskKind]) (*OnceTask[TaskKind], error) {
	taskID := taskData.GenerateIdempotentID()
	createdAt := time.Now()
	data, err := json.Marshal(taskData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task data: %w", err)
	}
	var dataMap map[string]interface{}
	if err := json.Unmarshal(data, &dataMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task data: %w", err)
	}

	// Extract ResourceKey if the task data implements ResourceKeyProvider
	var resourceKey string
	if provider, ok := taskData.(ResourceKeyProvider); ok {
		resourceKey = provider.GetResourceKey()
	}

	// Extract WaitUntil if the task data implements ScheduledTask
	// Default to epoch time (zero time) for immediate execution
	waitUntil := time.Time{}.Format(time.RFC3339) // Epoch: 0001-01-01T00:00:00Z
	if scheduledTask, ok := taskData.(ScheduledTask); ok {
		scheduledTime := scheduledTask.GetScheduledTime()
		if !scheduledTime.IsZero() {
			waitUntil = scheduledTime.UTC().Format(time.RFC3339)
		}
	}

	return &OnceTask[TaskKind]{
		Id:          taskID,
		Type:        taskData.GetType(),
		Data:        dataMap,
		ResourceKey: resourceKey,
		Env:         getTaskEnv(),
		WaitUntil:   waitUntil, // Epoch time for immediate tasks, or scheduled time
		LeasedUntil: "",        // Initially empty, set when task is leased to an executor.
		CreatedAt:   createdAt.UTC().Format(time.RFC3339),
		DoneAt:      "", // Initially empty, set when task is completed
	}, nil
}
