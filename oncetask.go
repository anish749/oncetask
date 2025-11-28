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
//	// Create a task manager with context
//	ctx := context.Background()
//	manager, cleanup := oncetask.NewFirestoreOnceTaskManager[TaskKind](ctx, firestoreClient)
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

// Data defines the interface for task-specific data that can be stored in OnceTask.
type Data[TaskKind comparable] = oncetask.Data[TaskKind]

// Manager manages the lifecycle of once-execution tasks backed by Firestore.
type Manager[TaskKind ~string] = oncetask.Manager[TaskKind]

// Handler types and adapters

// Handler is a function that processes a single task.
type Handler[TaskKind ~string] = oncetask.Handler[TaskKind]

// ResourceKeyHandler is a function that processes multiple tasks with the same resource key.
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

// WithNoRetry disables retry for a task handler.
var WithNoRetry = oncetask.WithNoRetry

// WithLeaseDuration sets the lease duration for a task handler.
var WithLeaseDuration = oncetask.WithLeaseDuration

// WithConcurrency sets the concurrency level for a task handler.
var WithConcurrency = oncetask.WithConcurrency

// WithCancellationHandler registers a cleanup handler for cancelled tasks.
func WithCancellationHandler[TaskKind ~string](handler Handler[TaskKind]) HandlerOption {
	return oncetask.WithCancellationHandler(handler)
}

// WithCancellationRetryPolicy sets the retry policy for cancellation handlers.
var WithCancellationRetryPolicy = oncetask.WithCancellationRetryPolicy

// RetryPolicy defines the retry behavior for failed tasks.
type RetryPolicy = oncetask.RetryPolicy

// ExponentialBackoffPolicy retries with exponential backoff delays.
type ExponentialBackoffPolicy = oncetask.ExponentialBackoffPolicy

// FixedDelayPolicy retries with a constant delay between attempts.
type FixedDelayPolicy = oncetask.FixedDelayPolicy

// NoRetryPolicy never retries - tasks fail permanently on first error.
type NoRetryPolicy = oncetask.NoRetryPolicy

// TaskError represents an error that occurred during task execution.
type TaskError = oncetask.TaskError

// Recurrence defines a recurring task schedule using RFC 5545 RRULE.
type Recurrence = oncetask.Recurrence

// TaskStatus represents the current state of a task.
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

// ScheduledTask is an interface for tasks that can specify a scheduled execution time.
type ScheduledTask = oncetask.ScheduledTask

// RecurrenceProvider is an interface for tasks that can define recurring schedules.
type RecurrenceProvider = oncetask.RecurrenceProvider

// NewFirestoreOnceTaskManager creates a new Firestore-backed task manager.
// The provided context is used as the parent for all background task processing.
// Context values (trace IDs, tenant IDs, etc.) will be inherited by task handlers.
func NewFirestoreOnceTaskManager[TaskKind ~string](ctx context.Context, client *firestore.Client) (manager Manager[TaskKind], cleanup func()) {
	return oncetask.NewFirestoreOnceTaskManager[TaskKind](ctx, client)
}

// ContextHandler is a slog.Handler that automatically adds task context to logs.
type ContextHandler = oncetask.ContextHandler

// NewContextHandler creates a new ContextHandler that wraps the provided handler.
var NewContextHandler = oncetask.NewContextHandler
