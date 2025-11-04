package oncetask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// ErrHandlerAlreadyExists is returned when trying to register a handler for a task type that already has a handler
	ErrHandlerAlreadyExists = errors.New("handler for this task type already exists")
	errLeaseNotAvailable    = errors.New("lease not available for task")
)

// firestoreOnceTaskRepository implements OnceTaskRepository using Firestore
type firestoreOnceTaskManager[TaskKind ~string] struct {
	client     *firestore.Client
	collection *firestore.CollectionRef

	mu           sync.RWMutex
	taskHandlers map[TaskKind]OnceTaskHandler[TaskKind]
	ctx          context.Context // background context in which the task handlers run, can be cancelled during shutdown
}

// NewFirestoreOnceTaskManager creates a new firestore once task manager.
// Returns the repository and a cancel function that cancels all running goroutines.
func NewFirestoreOnceTaskManager[TaskKind ~string](client *firestore.Client) (OnceTaskManager[TaskKind], func()) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &firestoreOnceTaskManager[TaskKind]{
		client:       client,
		collection:   client.Collection(CollectionOnceTasks),
		taskHandlers: make(map[TaskKind]OnceTaskHandler[TaskKind]),
		ctx:          ctx,
	}
	return m, cancel
}

// CreateTask creates a once task to Firestore.
// The task parameter should be created using fs_models/once.NewOnceTask().
// If a task with the same ID already exists, logs and returns nil (idempotent).
func (m *firestoreOnceTaskManager[TaskKind]) CreateTask(ctx context.Context, taskData OnceTaskData[TaskKind]) (bool, error) {
	task, err := NewOnceTask(taskData)
	if err != nil {
		return false, fmt.Errorf("failed to create once task: %w", err)
	}
	doc := m.collection.Doc(task.Id)

	// Create() will fail if the document already exists, making this atomic
	_, err = doc.Create(ctx, task)
	if err != nil && status.Code(err) == codes.AlreadyExists {
		slog.InfoContext(ctx, "Task already exists, skipping creation",
			"taskId", task.Id,
			"taskType", task.Type,
		)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to create task: %w", err)
	}

	return true, nil
}

// RegisterTaskHandler registers a task handler and starts a goroutine for that task type.
// The handler function is expected to successfully execute only once.
// If the handler returns an error, the task will be retried.
// Returns ErrHandlerAlreadyExists if a handler for this task type is already registered.
func (m *firestoreOnceTaskManager[TaskKind]) RegisterTaskHandler(
	taskType TaskKind,
	handler OnceTaskHandler[TaskKind],
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if handler already exists
	if _, exists := m.taskHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s", ErrHandlerAlreadyExists, taskType)
	}

	// Store the handler in the map
	m.taskHandlers[taskType] = handler

	// Start a goroutine for this task type
	go m.runTaskHandlerLoop(taskType, handler)

	return nil
}

// runTaskHandlerLoop runs the query loop for a specific task type.
// It automatically restarts the query on errors (but not on context cancellation).
func (m *firestoreOnceTaskManager[TaskKind]) runTaskHandlerLoop(taskType TaskKind, handler OnceTaskHandler[TaskKind]) {
	for {
		err := m.runQueryLoop(taskType, handler)

		// If context was cancelled, exit
		if m.ctx.Err() != nil {
			return
		}

		// Otherwise, log the error and restart the loop after a delay
		if err != nil {
			slog.ErrorContext(m.ctx, "Task handler loop error, restarting after delay",
				"error", err,
				"taskType", taskType,
			)
			// Wait a bit before restarting to avoid tight restart loops
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(5 * time.Second):
				// Restart loop
			}
		}
	}
}

// runQueryLoop runs a single iteration of the query loop for a task type
func (m *firestoreOnceTaskManager[TaskKind]) runQueryLoop(
	taskType TaskKind,
	handler OnceTaskHandler[TaskKind],
) error {
	// Query for tasks that are:
	// 1. Of the specified type
	// 2. Not done (doneAt is empty)
	query := m.collection.
		Where("type", "==", string(taskType)).
		Where("doneAt", "==", "")

	iter := query.Snapshots(m.ctx)
	defer iter.Stop()

	for {
		snap, err := iter.Next()
		if err != nil {
			// Check if context was cancelled
			if m.ctx.Err() != nil {
				return m.ctx.Err()
			}
			slog.ErrorContext(m.ctx, "Error receiving snapshot in query loop", "error", err, "taskType", taskType)
			return fmt.Errorf("error receiving snapshot: %w", err)
		}

		// Process each document change in the snapshot
		for _, docChange := range snap.Changes {
			// Only process added/modified documents (not deleted)
			if docChange.Kind != firestore.DocumentAdded && docChange.Kind != firestore.DocumentModified {
				continue
			}
			var task OnceTask[TaskKind]
			if err := docChange.Doc.DataTo(&task); err != nil {
				slog.ErrorContext(m.ctx, "Failed to parse task from snapshot", "error", err, "taskId", docChange.Doc.Ref.ID)
				continue
			}

			// Skip if task is already done (shouldn't happen due to query, but be safe)
			if task.DoneAt != "" || task.Type != taskType {
				continue
			}

			taskId := task.Id
			err := m.executeTaskWithLease(m.ctx, taskId, handler)
			if err != nil {
				if errors.Is(err, errLeaseNotAvailable) {
					slog.InfoContext(m.ctx, "Lease not available for task, skipping", "taskId", taskId, "taskType", taskType)
					continue
				}
				slog.ErrorContext(m.ctx, "Error executing handler with lease in query loop", "error", err, "taskId", taskId, "taskType", taskType)
				// Continue processing other tasks even if one fails
				continue
			}
		}
	}
}

// executeTaskWithLease executes a task handler with distributed lease management
// - Looks up task by taskId
// - Acquires lease (or returns ErrLeaseNotAvailable if already held by another instance)
// - Calls handler with the task
// - Marks task as done if handler succeeds
// - Releases lease if handler fails (to allow retry)
func (m *firestoreOnceTaskManager[TaskKind]) executeTaskWithLease(
	ctx context.Context,
	taskId string,
	handler OnceTaskHandler[TaskKind],
) error {
	// Step 1: Acquire lease and get current task
	var task OnceTask[TaskKind]
	now := time.Now().UTC()

	err := m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Look up task by taskId
		doc := m.collection.Doc(taskId)
		snap, err := tx.Get(doc)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to get task in lease transaction", "error", err, "taskId", taskId)
			return fmt.Errorf("failed to get task: %w", err)
		}

		if err := snap.DataTo(&task); err != nil {
			return fmt.Errorf("failed to parse task: %w", err)
		}

		// Skip if task is already done
		if task.DoneAt != "" {
			return nil
		}

		// Check if this specific task already has a lease
		if task.LeasedUntil != "" {
			leaseExpiry, err := time.Parse(time.RFC3339, task.LeasedUntil)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to parse lease expiry in lease transaction", "error", err, "taskId", taskId)
				return fmt.Errorf("failed to parse lease expiry: %w", err)
			}
			if leaseExpiry.After(now) {
				// Lease is still active - cannot acquire
				slog.InfoContext(ctx, "Lease already held for task", "taskId", taskId, "leasedUntil", task.LeasedUntil)
				return errLeaseNotAvailable
			}
		}

		// If ResourceKey is set, check for active leases on other tasks with the same ResourceKey
		// This ensures resource-level serialization (e.g., only one sync per calendarId at a time)
		if task.ResourceKey != "" {
			// Query for other tasks with same ResourceKey, same type, not done, with active lease, and excluding current task
			// The query filters for leasedUntil > now, so any matching document has an active lease
			// Limit to 1 since we only need to check existence
			query := m.collection.
				Where("type", "==", string(task.Type)).
				Where("resourceKey", "==", task.ResourceKey).
				Where("doneAt", "==", "").
				Where("leasedUntil", ">", now.Format(time.RFC3339)).
				Where("id", "!=", taskId).
				Limit(1)

			docs, err := tx.Documents(query).GetAll()
			if err != nil {
				slog.ErrorContext(ctx, "Failed to query for resource-level leases", "error", err, "taskId", taskId, "resourceKey", task.ResourceKey)
				return fmt.Errorf("failed to query for resource-level leases: %w", err)
			}

			// If any documents match, another task with the same ResourceKey has an active lease
			if len(docs) > 0 {
				slog.InfoContext(ctx, "Resource-level lease already held",
					"taskId", taskId,
					"resourceKey", task.ResourceKey,
					"otherTaskId", docs[0].Ref.ID,
				)
				return errLeaseNotAvailable
			}
		}

		// Acquire lease
		newLeaseExpiry := now.Add(leaseDuration).Format(time.RFC3339)
		updates := []firestore.Update{
			{Path: "leasedUntil", Value: newLeaseExpiry},
		}

		if err := tx.Update(doc, updates); err != nil {
			slog.ErrorContext(ctx, "Failed to acquire lease in transaction", "error", err, "taskId", taskId)
			return fmt.Errorf("failed to acquire lease: %w", err)
		}

		task.LeasedUntil = newLeaseExpiry
		slog.InfoContext(ctx, "Lease acquired for task", "taskId", taskId, "leasedUntil", newLeaseExpiry)

		return nil
	})

	if err != nil {
		return err
	}

	// Step 2: Execute handler function (outside transaction)
	handlerErr := handler(ctx, &task)

	// Step 3: Update task and release lease
	return m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		doc := m.collection.Doc(taskId)

		if handlerErr == nil {
			// Handler succeeded - mark task as done
			updates := []firestore.Update{
				{Path: "doneAt", Value: now.Format(time.RFC3339)},
				{Path: "leasedUntil", Value: nil}, // Clear lease
			}

			if txErr := tx.Update(doc, updates); txErr != nil {
				slog.ErrorContext(ctx, "Failed to mark task as done in transaction", "error", txErr, "taskId", taskId)
				return fmt.Errorf("failed to mark task as done: %w", txErr)
			}

			slog.InfoContext(ctx, "Task completed successfully", "taskId", taskId)
		} else {
			// Handler failed - release lease to allow retry
			updates := []firestore.Update{
				{Path: "leasedUntil", Value: nil}, // Clear lease - task will be retried
			}

			if txErr := tx.Update(doc, updates); txErr != nil {
				slog.ErrorContext(ctx, "Failed to release lease in transaction", "error", txErr, "taskId", taskId)
				return fmt.Errorf("failed to release lease: %w", txErr)
			}

			slog.ErrorContext(ctx, "Handler failed, releasing lease for retry", "error", handlerErr, "taskId", taskId)
		}

		// Return the original handler error if it occurred
		return handlerErr
	})
}
