// Package oncetask provides a task management system for executing tasks exactly once
// with support for retries, recurrence, cancellation, and resource-based locking.
//
// OnceTask is designed for reliable background task processing with Firestore as the backend.
// It provides three execution strategies:
//   - Concurrent: Multiple tasks of the same kind execute concurrently
//   - OnePerResourceKey: Only one task with the same resource key executes at a time
//   - AllPerResourceKey: All pending tasks with the same resource key are batched together
//
// Key Features:
//   - Exactly-once execution semantics with retry support
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

// Core types
type OnceTask[TaskKind ~string] = oncetask.OnceTask[TaskKind]
type OnceTaskData[TaskKind comparable] = oncetask.OnceTaskData[TaskKind]
type OnceTaskManager[TaskKind ~string] = oncetask.OnceTaskManager[TaskKind]

// Handler types and adapters
type OnceTaskHandler[TaskKind ~string] = oncetask.OnceTaskHandler[TaskKind]
type OnceTaskResourceKeyHandler[TaskKind ~string] = oncetask.OnceTaskResourceKeyHandler[TaskKind]

// Handler adapters for functions that don't return results
// NoResult adapts a single-task handler that doesn't return a result.
func NoResult[TaskKind ~string](fn func(ctx context.Context, task *OnceTask[TaskKind]) error) OnceTaskHandler[TaskKind] {
	return oncetask.NoResult(fn)
}

// NoResultResourceKey adapts a resource-key handler that doesn't return a result.
func NoResultResourceKey[TaskKind ~string](fn func(ctx context.Context, tasks []OnceTask[TaskKind]) error) OnceTaskResourceKeyHandler[TaskKind] {
	return oncetask.NoResultResourceKey(fn)
}

// Configuration
type HandlerOption = oncetask.HandlerOption

// Handler configuration options
var WithRetryPolicy = oncetask.WithRetryPolicy
var WithNoRetry = oncetask.WithNoRetry
var WithLeaseDuration = oncetask.WithLeaseDuration
var WithConcurrency = oncetask.WithConcurrency

// WithCancellationHandler registers a cleanup handler for cancelled tasks.
func WithCancellationHandler[TaskKind ~string](handler OnceTaskHandler[TaskKind]) HandlerOption {
	return oncetask.WithCancellationHandler(handler)
}

var WithCancellationRetryPolicy = oncetask.WithCancellationRetryPolicy

// Retry policies
type RetryPolicy = oncetask.RetryPolicy
type ExponentialBackoffPolicy = oncetask.ExponentialBackoffPolicy
type FixedDelayPolicy = oncetask.FixedDelayPolicy
type NoRetryPolicy = oncetask.NoRetryPolicy

// Task model types
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

// Provider interfaces
type ResourceKeyProvider = oncetask.ResourceKeyProvider
type ScheduledTask = oncetask.ScheduledTask
type RecurrenceProvider = oncetask.RecurrenceProvider

// Manager constructor
// NewFirestoreOnceTaskManager creates a new Firestore-backed task manager.
func NewFirestoreOnceTaskManager[TaskKind ~string](client *firestore.Client) (OnceTaskManager[TaskKind], func()) {
	return oncetask.NewFirestoreOnceTaskManager[TaskKind](client)
}

// Context logging
type ContextHandler = oncetask.ContextHandler

var NewContextHandler = oncetask.NewContextHandler
