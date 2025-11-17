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

// firestoreOnceTaskManager implements OnceTaskManager using Firestore
type firestoreOnceTaskManager[TaskKind ~string] struct {
	client     *firestore.Client
	collection *firestore.CollectionRef

	mu            sync.RWMutex
	taskHandlers  map[TaskKind]OnceTaskHandler[TaskKind]
	evaluateChans map[TaskKind]chan struct{} // Per task type channels for immediate evaluation
	ctx           context.Context            // background context in which the task handlers run, can be cancelled during shutdown
	env           string                     // environment identifier for task segregation
}

// NewFirestoreOnceTaskManager creates a new firestore once task manager.
// Returns the repository and a cancel function that cancels all running goroutines.
func NewFirestoreOnceTaskManager[TaskKind ~string](client *firestore.Client) (OnceTaskManager[TaskKind], func()) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &firestoreOnceTaskManager[TaskKind]{
		client:        client,
		collection:    client.Collection(CollectionOnceTasks),
		taskHandlers:  make(map[TaskKind]OnceTaskHandler[TaskKind]),
		evaluateChans: make(map[TaskKind]chan struct{}),
		ctx:           ctx,
		env:           getTaskEnv(),
	}
	return m, cancel
}

// CreateTask creates a once task to Firestore.
// The task parameter should be created using fs_models/once.NewOnceTask().
// If a task with the same ID already exists, logs and returns nil (idempotent).
func (m *firestoreOnceTaskManager[TaskKind]) CreateTask(ctx context.Context, taskData OnceTaskData[TaskKind]) (bool, error) {
	task, err := newOnceTask(taskData)
	if err != nil {
		return false, fmt.Errorf("failed to create once task: %w", err)
	}
	doc := m.collection.Doc(task.Id)

	// Create() will fail if the document already exists, making this atomic
	_, err = doc.Create(ctx, task)
	if err != nil && status.Code(err) == codes.AlreadyExists {
		slog.InfoContext(ctx, "Task already exists, skipping creation", "taskId", task.Id, "taskType", task.Type)
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to create task: %w", err)
	}

	// Trigger immediate evaluation for this task type
	m.evaluateNow(task.Type)

	return true, nil
}

// evaluateNow triggers immediate evaluation for a specific task type.
// This causes the task consumer loop for this type to check for ready tasks immediately,
// bypassing the normal polling interval.
func (m *firestoreOnceTaskManager[TaskKind]) evaluateNow(taskType TaskKind) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ch, exists := m.evaluateChans[taskType]
	if !exists {
		slog.WarnContext(m.ctx, "No handler registered for task type", "taskType", taskType)
		return
	}

	select {
	case ch <- struct{}{}:
		slog.InfoContext(m.ctx, "Triggered immediate evaluation", "taskType", taskType)
	default:
		// Channel buffer full, evaluation will happen soon anyway
	}
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

	// Create evaluate channel for this task type (buffered to avoid blocking)
	evaluateChan := make(chan struct{}, 1)
	m.evaluateChans[taskType] = evaluateChan

	// Start a goroutine for this task type
	go m.runTaskHandlerLoop(taskType, handler, evaluateChan)

	return nil
}

// runTaskHandlerLoop runs the consumer loop for a specific task type.
// It processes pending tasks immediately on startup, then queries for ready tasks
// every minute and processes them synchronously.
// Can be triggered immediately via the evaluateChan channel.
// Runs indefinitely until the context is cancelled.
func (m *firestoreOnceTaskManager[TaskKind]) runTaskHandlerLoop(
	taskType TaskKind,
	handler OnceTaskHandler[TaskKind],
	evaluateChan chan struct{},
) {
	slog.InfoContext(m.ctx, "Starting task consumer loop", "taskType", taskType)
	defer slog.InfoContext(m.ctx, "Task consumer loop stopped", "taskType", taskType)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Process pending tasks immediately on startup
	m.processPendingTasks(taskType, handler)

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.processPendingTasks(taskType, handler)
		case <-evaluateChan:
			m.processPendingTasks(taskType, handler)
		}
	}
}

// processPendingTasks queries for ready tasks and processes them one by one.
// A task is ready if:
// - waitUntil <= now (scheduled time has arrived or immediate execution with epoch time)
// - Not already leased by another instance
// Processes up to 100 tasks per call.
func (m *firestoreOnceTaskManager[TaskKind]) processPendingTasks(
	taskType TaskKind,
	handler OnceTaskHandler[TaskKind],
) {
	ctx := m.ctx
	now := time.Now().UTC()

	// Query for ready tasks that are not currently leased
	query := m.collection.
		Select("id").
		Where("type", "==", string(taskType)).
		Where("doneAt", "==", "").
		Where("env", "==", m.env).
		Where("waitUntil", "<=", now.Format(time.RFC3339)).
		Where("leasedUntil", "<=", now.Format(time.RFC3339)).
		OrderBy("leasedUntil", firestore.Asc).
		OrderBy("waitUntil", firestore.Asc).
		Limit(100)

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to query for pending tasks", "error", err, "taskType", taskType)
		return
	}

	// No tasks ready
	if len(docs) == 0 {
		slog.InfoContext(ctx, "No pending tasks found", "taskType", taskType)
		return
	}

	slog.InfoContext(ctx, "Processing pending tasks", "taskType", taskType, "count", len(docs))

	for _, doc := range docs {
		taskId := doc.Ref.ID
		// Execute the task synchronously
		err = m.executeTaskWithLease(ctx, taskId, handler)
		if err != nil {
			if errors.Is(err, errLeaseNotAvailable) {
				// Another instance already processing this task, skip silently
				continue
			}
			slog.ErrorContext(ctx, "Error executing task", "error", err, "taskId", taskId, "taskType", taskType)
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
				slog.ErrorContext(ctx, "Failed to parse lease expiry in lease transaction", "error", err, "taskId", taskId, "taskType", task.Type)
				return fmt.Errorf("failed to parse lease expiry: %w", err)
			}
			if leaseExpiry.After(now) {
				// Lease is still active - cannot acquire
				slog.InfoContext(ctx, "Lease already held for task", "taskId", taskId, "leasedUntil", task.LeasedUntil, "taskType", task.Type)
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
				Where("env", "==", m.env).
				Limit(1)

			docs, err := tx.Documents(query).GetAll()
			if err != nil {
				slog.ErrorContext(ctx, "Failed to query for resource-level leases", "error", err, "taskId", taskId, "resourceKey", task.ResourceKey, "taskType", task.Type)
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
			slog.ErrorContext(ctx, "Failed to acquire lease in transaction", "error", err, "taskId", taskId, "taskType", task.Type)
			return fmt.Errorf("failed to acquire lease: %w", err)
		}

		task.LeasedUntil = newLeaseExpiry
		slog.InfoContext(ctx, "Lease acquired for task", "taskId", taskId, "leasedUntil", newLeaseExpiry, "taskType", task.Type)

		return nil
	})

	if err != nil {
		return err
	}

	// Step 2: Execute handler function (outside transaction)
	// Add task ID to context for automatic logging
	handlerErr := handler(withTaskID(ctx, taskId), &task)

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
				slog.ErrorContext(ctx, "Failed to mark task as done in transaction", "error", txErr, "taskId", taskId, "taskType", task.Type)
				return fmt.Errorf("failed to mark task as done: %w", txErr)
			}

			slog.InfoContext(ctx, "Task completed successfully", "taskId", taskId, "taskType", task.Type)
		} else {
			// Handler failed - release lease to allow retry
			updates := []firestore.Update{
				{Path: "leasedUntil", Value: nil}, // Clear lease - task will be retried
			}

			if txErr := tx.Update(doc, updates); txErr != nil {
				slog.ErrorContext(ctx, "Failed to release lease in transaction", "error", txErr, "taskId", taskId, "taskType", task.Type)
				return fmt.Errorf("failed to release lease: %w", txErr)
			}

			slog.ErrorContext(ctx, "Handler failed, releasing lease for retry", "error", handlerErr, "taskId", taskId, "taskType", task.Type)
		}

		// Return the original handler error if it occurred
		return handlerErr
	})
}
