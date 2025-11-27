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

// TaskError represents a single failed execution attempt.
type TaskError struct {
	At    string `json:"at" firestore:"at"`       // ISO 8601 timestamp of failure
	Error string `json:"error" firestore:"error"` // Failure reason
}

// Recurrence defines a recurring task schedule using RFC 5545 RRULE.
// When set on a task, the task becomes a generator that creates occurrence tasks.
//
// Architecture:
//   - Recurrence Task (Generator): Holds the RRULE and spawns occurrence tasks.
//     Has Recurrence set, never marked as done (unless RRULE exhausted).
//   - Occurrence Task (Instance): A regular one-time task spawned by the generator.
//     Has ParentRecurrenceID set, has its own lifecycle (attempts, errors, result, doneAt).
//
// After 3 weeks of weekly runs, there are 4 tasks total:
//   - 1 recurrence task (the generator, still active)
//   - 3 occurrence tasks (each with their own execution history)
type Recurrence struct {
	RRule   string   `json:"rrule" firestore:"rrule"`     // RFC 5545 rule, e.g., "FREQ=WEEKLY;BYDAY=MO"
	DTStart string   `json:"dtstart" firestore:"dtstart"` // Recurrence anchor time (ISO 8601)
	ExDates []string `json:"exdates" firestore:"exdates"` // Exception dates to skip
}

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

	// Retry and result tracking fields
	Attempts int         `json:"attempts" firestore:"attempts"` // Number of execution attempts (incremented when lease is acquired)
	Errors   []TaskError `json:"errors" firestore:"errors"`     // Failure history (one entry per failed attempt)
	Result   any         `json:"result" firestore:"result"`     // Handler output (optional, can be nil on success)

	// Recurrence fields
	// For recurrence tasks (generators): Recurrence is set, ParentRecurrenceID is empty
	// For occurrence tasks (instances): Recurrence is nil, ParentRecurrenceID points to generator
	Recurrence          *Recurrence `json:"recurrence" firestore:"recurrence"`                   // Recurrence config (nil = one-time or occurrence task)
	ParentRecurrenceID  string      `json:"parentRecurrenceId" firestore:"parentRecurrenceId"`   // ID of parent recurrence task (empty = standalone or recurrence task)
	OccurrenceTimestamp string      `json:"occurrenceTimestamp" firestore:"occurrenceTimestamp"` // Scheduled time of this occurrence (ISO 8601)

	// Cancellation fields
	IsCancelled bool   `json:"isCancelled" firestore:"isCancelled"` // Cancellation flag (false = not cancelled)
	CancelledAt string `json:"cancelledAt" firestore:"cancelledAt"` // ISO 8601 timestamp when task was cancelled (audit trail)
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

// RecurrenceProvider is an optional interface that OnceTaskData implementations can implement
// to define recurring task schedules. When implemented, the task becomes a generator that
// spawns occurrence tasks according to the RRULE.
type RecurrenceProvider interface {
	GetRecurrence() *Recurrence
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

	// Extract Recurrence if the task data implements RecurrenceProvider
	var recurrence *Recurrence
	if recurrenceProvider, ok := taskData.(RecurrenceProvider); ok {
		recurrence = recurrenceProvider.GetRecurrence()
	}

	// Extract scheduled time if the task data implements ScheduledTask
	var scheduledTime time.Time
	if scheduledTask, ok := taskData.(ScheduledTask); ok {
		scheduledTime = scheduledTask.GetScheduledTime()
	}

	// Validate: cannot specify both Recurrence and a non-zero ScheduledTime
	// Recurring tasks get their schedule from Recurrence, not ScheduledTask
	if recurrence != nil && !scheduledTime.IsZero() {
		return nil, fmt.Errorf("task %s: cannot specify both Recurrence and ScheduledTask - recurring tasks use Recurrence for scheduling", taskID)
	}

	// Calculate WaitUntil based on task type
	var waitUntil string
	if recurrence != nil {
		// Recurring task: calculate first occurrence from DTStart
		firstOccurrence, err := calculateFirstOccurrence(recurrence)
		if err != nil {
			return nil, fmt.Errorf("task %s: %w", taskID, err)
		}

		waitUntil = firstOccurrence.UTC().Format(time.RFC3339)
	} else if !scheduledTime.IsZero() {
		// One-time scheduled task: use ScheduledTask.GetScheduledTime()
		waitUntil = scheduledTime.UTC().Format(time.RFC3339)
	} else {
		// Immediate execution: epoch time
		waitUntil = time.Time{}.Format(time.RFC3339) // Epoch: 0001-01-01T00:00:00Z
	}

	return &OnceTask[TaskKind]{
		Id:          taskID,
		Type:        taskData.GetType(),
		Data:        dataMap,
		ResourceKey: resourceKey,
		Env:         getTaskEnv(),
		WaitUntil:   waitUntil, // Derived from Recurrence.DTStart, ScheduledTime, or epoch
		LeasedUntil: "",        // Initially empty, set when task is leased to an executor.
		CreatedAt:   createdAt.UTC().Format(time.RFC3339),
		DoneAt:      "",         // Initially empty, set when task is completed
		Recurrence:  recurrence, // nil = one-time task, set from RecurrenceProvider
		IsCancelled: false,
		CancelledAt: "",
	}, nil
}
