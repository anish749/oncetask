// Package oncetask provides a lease-based task management system for executing tasks exactly once
// with support for retries, recurrence, cancellation, and resource-based locking.
//
// OnceTask is designed for reliable background task processing with Firestore as the backend.
// Tasks are executed using a lease-based mechanism to prevent concurrent execution, and will be
// retried on failure according to the configured retry policy until they succeed.
//
// It provides three execution strategies:
//   - Concurrent: Multiple tasks of the same kind execute concurrently
//   - OnePerResourceKey: Only one task with the same resource key executes at a time
//   - AllPerResourceKey: All pending tasks with the same resource key are batched together
//
// Key Features:
//   - Lease-based exactly-once execution semantics with automatic retry on failure
//   - Configurable lease duration acts as a timeout for task execution
//   - Flexible retry policies (exponential backoff, fixed delay, no retry)
//   - Task cancellation with optional cleanup handlers
//   - Recurring tasks using rrule syntax
//   - Resource-based locking for coordinated execution
//   - Batch processing capabilities
//
// Basic Usage:
//
//	// Create a task manager
//	manager, cleanup := oncetask.NewFirestoreOnceTaskManager[TaskKind](firestoreClient)
//	defer cleanup()
//
//	// Register a task handler
//	manager.RegisterTaskHandler("email", func(ctx context.Context, task *oncetask.OnceTask[TaskKind]) (any, error) {
//	    // Process the task
//	    return nil, nil
//	}, oncetask.WithRetryPolicy(oncetask.ExponentialBackoffPolicy{
//	    MaxRetries: 3,
//	    BaseDelay: time.Second,
//	}))
//
//	// Create a task
//	manager.CreateTask(ctx, taskData)
package oncetask

import (
	"context"

	"cloud.google.com/go/firestore"
	"github.com/anish749/oncetask/oncetask"
)

// Re-export all public types and functions from the oncetask package

// OnceTask represents a task to be executed exactly once using a lease-based mechanism.
// The task will be retried on failure according to the configured retry policy until it succeeds.
type OnceTask[TaskKind ~string] = oncetask.OnceTask[TaskKind]
type Data[TaskKind comparable] = oncetask.Data[TaskKind]
type Manager[TaskKind ~string] = oncetask.Manager[TaskKind]

// Handler types and adapters
type Handler[TaskKind ~string] = oncetask.Handler[TaskKind]
type ResourceKeyHandler[TaskKind ~string] = oncetask.ResourceKeyHandler[TaskKind]

// NoResult adapts a single-task handler that doesn't return a result.
func NoResult[TaskKind ~string](fn func(ctx context.Context, task *OnceTask[TaskKind]) error) Handler[TaskKind] {
	return oncetask.NoResult(fn)
}

// NoResultResourceKey adapts a resource-key handler that doesn't return a result.
func NoResultResourceKey[TaskKind ~string](fn func(ctx context.Context, tasks []OnceTask[TaskKind]) error) ResourceKeyHandler[TaskKind] {
	return oncetask.NoResultResourceKey(fn)
}

// HandlerOption configures task handler behavior.
type HandlerOption = oncetask.HandlerOption

// WithRetryPolicy sets the retry policy for a task handler.
var WithRetryPolicy = oncetask.WithRetryPolicy
var WithNoRetry = oncetask.WithNoRetry
var WithLeaseDuration = oncetask.WithLeaseDuration
var WithConcurrency = oncetask.WithConcurrency

// WithCancellationHandler registers a cleanup handler for cancelled tasks.
func WithCancellationHandler[TaskKind ~string](handler Handler[TaskKind]) HandlerOption {
	return oncetask.WithCancellationHandler(handler)
}

var WithCancellationRetryPolicy = oncetask.WithCancellationRetryPolicy

// RetryPolicy defines the retry behavior for failed tasks.
type RetryPolicy = oncetask.RetryPolicy
type ExponentialBackoffPolicy = oncetask.ExponentialBackoffPolicy
type FixedDelayPolicy = oncetask.FixedDelayPolicy
type NoRetryPolicy = oncetask.NoRetryPolicy

// TaskError represents an error that occurred during task execution.
type TaskError = oncetask.TaskError
type Recurrence = oncetask.Recurrence
type TaskStatus = oncetask.TaskStatus

// TaskStatus constants
const (
	TaskStatusWaiting             = oncetask.TaskStatusWaiting
	TaskStatusPending             = oncetask.TaskStatusPending
	TaskStatusLeased              = oncetask.TaskStatusLeased
	TaskStatusCancellationPending = oncetask.TaskStatusCancellationPending
	TaskStatusCompleted           = oncetask.TaskStatusCompleted
	TaskStatusFailed              = oncetask.TaskStatusFailed
	TaskStatusCancelled           = oncetask.TaskStatusCancelled
)

// ResourceKeyProvider is an interface for types that can provide a resource key.
type ResourceKeyProvider = oncetask.ResourceKeyProvider
type ScheduledTask = oncetask.ScheduledTask
type RecurrenceProvider = oncetask.RecurrenceProvider

// NewFirestoreOnceTaskManager creates a new Firestore-backed task manager.
func NewFirestoreOnceTaskManager[TaskKind ~string](client *firestore.Client) (Manager[TaskKind], func()) {
	return oncetask.NewFirestoreOnceTaskManager[TaskKind](client)
}

// ContextHandler is a slog.Handler that automatically adds task context to logs.
type ContextHandler = oncetask.ContextHandler

var NewContextHandler = oncetask.NewContextHandler
