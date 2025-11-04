package oncetask

import (
	"encoding/json"
	"fmt"

	"time"
)

const (
	CollectionOnceTasks string = "onceTasks"
)

// Once Queue is a set of tools and utilities
// used to execute something only once, asynchronously.
type OnceTask[TaskKind ~string] struct {
	Id   string                 `json:"id" firestore:"id"` // Also the idempotency key.
	Type TaskKind               `json:"type" firestore:"type"`
	Data map[string]interface{} `json:"data" firestore:"data"`

	// Optional - identifies a resource that requires serialization (e.g., calendarId, conversationId)
	// When set, lease acquisition checks for active leases on other tasks with the same ResourceKey
	ResourceKey string `json:"resourceKey" firestore:"resourceKey"`

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

func NewOnceTask[TaskKind ~string](taskData OnceTaskData[TaskKind]) (*OnceTask[TaskKind], error) {
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

	return &OnceTask[TaskKind]{
		Id:          taskID,
		Type:        taskData.GetType(),
		Data:        dataMap,
		ResourceKey: resourceKey,
		LeasedUntil: "", // Initially empty, set when task is leased to an executor.
		CreatedAt:   createdAt.UTC().Format(time.RFC3339),
		DoneAt:      "", // Initially empty, set when task is completed
	}, nil
}
