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

	mu                  sync.RWMutex
	taskHandlers        map[TaskKind]OnceTaskHandler[TaskKind]
	resourceKeyHandlers map[TaskKind]OnceTaskResourceKeyHandler[TaskKind]
	evaluateChans       map[TaskKind]chan struct{} // Per task type channels for immediate evaluation
	ctx                 context.Context            // background context in which the task handlers run, can be cancelled during shutdown
	env                 string                     // environment identifier for task segregation
}

// NewFirestoreOnceTaskManager creates a new firestore once task manager.
// Returns the repository and a cancel function that cancels all running goroutines.
func NewFirestoreOnceTaskManager[TaskKind ~string](client *firestore.Client) (OnceTaskManager[TaskKind], func()) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &firestoreOnceTaskManager[TaskKind]{
		client:              client,
		collection:          client.Collection(CollectionOnceTasks),
		taskHandlers:        make(map[TaskKind]OnceTaskHandler[TaskKind]),
		resourceKeyHandlers: make(map[TaskKind]OnceTaskResourceKeyHandler[TaskKind]),
		evaluateChans:       make(map[TaskKind]chan struct{}),
		ctx:                 ctx,
		env:                 getTaskEnv(),
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

	// Check if handler already exists (either single or resource key)
	if _, exists := m.taskHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s", ErrHandlerAlreadyExists, taskType)
	}
	if _, exists := m.resourceKeyHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s (resource key handler registered)", ErrHandlerAlreadyExists, taskType)
	}

	// Store the handler in the map
	m.taskHandlers[taskType] = handler

	// Create evaluate channel for this task type (buffered to avoid blocking)
	evaluateChan := make(chan struct{}, 1)
	m.evaluateChans[taskType] = evaluateChan

	// Create an adapter that makes the single task handler compatible with the resource key handler.
	singleTaskProcessor := func(ctx context.Context, tasks []OnceTask[TaskKind]) error {
		// We must process only one task here, so that we can mark the task as done before starting the next task.
		// Otherwise, we won't be marking a single task as done before starting the next task.
		if len(tasks) != 1 {
			// The limit in the claimTasks function must ensure that we only claim one task.
			slog.ErrorContext(ctx, "We claimed more than one task", "taskType", taskType, "taskCount", len(tasks))
			return fmt.Errorf("expected 1 task, got %d", len(tasks))
		}
		task := tasks[0]
		return handler(withTaskID(ctx, task.Id), &task)
	}

	go m.runLoop(taskType, singleTaskProcessor, evaluateChan)

	return nil
}

// RegisterResourceKeyHandler registers a resource key handler and starts a goroutine for that task type.
// All pending tasks with the same resource key are grouped together and sorted by CreatedAt.
// If the handler returns nil, all tasks for that resource key are marked as done.
// If the handler returns an error, all tasks for that resource key will be retried.
// Returns ErrHandlerAlreadyExists if a handler for this task type is already registered.
func (m *firestoreOnceTaskManager[TaskKind]) RegisterResourceKeyHandler(
	taskType TaskKind,
	handler OnceTaskResourceKeyHandler[TaskKind],
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if handler already exists (either single or resource key)
	if _, exists := m.taskHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s (single handler registered)", ErrHandlerAlreadyExists, taskType)
	}
	if _, exists := m.resourceKeyHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s", ErrHandlerAlreadyExists, taskType)
	}

	// Store the handler in the map
	m.resourceKeyHandlers[taskType] = handler

	// Create evaluate channel for this task type (buffered to avoid blocking)
	evaluateChan := make(chan struct{}, 1)
	m.evaluateChans[taskType] = evaluateChan

	go m.runLoop(taskType, handler, evaluateChan)

	return nil
}

// runLoop is the main processing loop for both single and resource-key tasks.
// It repeatedly claims batches of tasks, executes them, and completes them.
func (m *firestoreOnceTaskManager[TaskKind]) runLoop(
	taskType TaskKind,
	processor func(context.Context, []OnceTask[TaskKind]) error,
	evaluateChan chan struct{},
) {
	slog.InfoContext(m.ctx, "Starting unified task consumer loop", "taskType", taskType)
	defer slog.InfoContext(m.ctx, "Unified task consumer loop stopped", "taskType", taskType)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Start with checking for work immediately
	shouldWait := false

	for {
		// 1. Wait for trigger (if needed)
		if shouldWait {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C: // Timer fired, check for work
			case <-evaluateChan: // Signal received, check for work
			}
		}

		// Check context cancellation if we skipped the select
		if m.ctx.Err() != nil {
			return
		}

		// 2. Claim Tasks
		tasks, err := m.claimTasks(m.ctx, taskType)
		if err != nil {
			if !errors.Is(err, errLeaseNotAvailable) {
				slog.ErrorContext(m.ctx, "Failed to claim task batch", "error", err, "taskType", taskType)
			}
			// On error, wait for next trigger (backoff)
			shouldWait = true
			continue
		}

		if len(tasks) == 0 {
			// No work available, wait for next trigger
			shouldWait = true
			continue
		}

		// We found work, so check again immediately after this batch (drain the queue)
		shouldWait = false

		// 3. Execute Batch (User Code)
		execErr := processor(m.ctx, tasks)
		if execErr != nil {
			slog.ErrorContext(m.ctx, "Task batch execution failed", "error", execErr, "taskType", taskType, "batchSize", len(tasks))
		}

		// 4. Complete Batch (Update DB)
		if err := m.completeBatch(m.ctx, tasks, execErr); err != nil {
			slog.ErrorContext(m.ctx, "Failed to complete task batch", "error", err, "taskType", taskType)
		}
	}
}

// claimTasks attempts to find and lock ready tasks (one or more, depending on handler type).
// Returns a slice of locked tasks. If no tasks are available, returns empty slice and nil error.
func (m *firestoreOnceTaskManager[TaskKind]) claimTasks(ctx context.Context, taskType TaskKind) ([]OnceTask[TaskKind], error) {
	var tasks []OnceTask[TaskKind]
	now := time.Now().UTC()

	err := m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// 1. Find a candidate task
		// We look for ONE task that is ready.
		query := m.collection.
			Where("type", "==", string(taskType)).
			Where("doneAt", "==", "").
			Where("env", "==", m.env).
			Where("waitUntil", "<=", now.Format(time.RFC3339)).
			Where("leasedUntil", "<=", now.Format(time.RFC3339)).
			OrderBy("leasedUntil", firestore.Asc).
			OrderBy("waitUntil", firestore.Asc).
			Limit(1)

		docs, err := tx.Documents(query).GetAll()
		if err != nil {
			return fmt.Errorf("failed to query candidate task: %w", err)
		}
		if len(docs) == 0 {
			return nil // No tasks ready
		}

		candidateDoc := docs[0]
		candidateRef := candidateDoc.Ref
		candidateID := candidateRef.ID

		var candidate OnceTask[TaskKind]
		if err := candidateDoc.DataTo(&candidate); err != nil {
			return fmt.Errorf("failed to parse candidate task: %w", err)
		}

		// Double check lease (in case query was slightly stale or concurrent)
		if err := checkTaskLease(candidate, now); err != nil {
			return err
		}

		// 2. Determine Batch Scope (Single vs Resource Group)
		// Check if we have a ResourceKeyHandler registered for this type
		m.mu.RLock()
		_, hasResourceKeyHandler := m.resourceKeyHandlers[taskType]
		m.mu.RUnlock()

		if hasResourceKeyHandler && candidate.ResourceKey != "" {
			// RESOURCE KEY MODE
			// We need to lock ALL ready tasks for this resource key.
			// We fetch ALL tasks that are "ready" (waitUntil <= now).
			// This includes tasks that might currently be leased (running).
			batchQuery := m.collection.
				Where("type", "==", string(taskType)).
				Where("resourceKey", "==", candidate.ResourceKey).
				Where("doneAt", "==", "").
				Where("env", "==", m.env).
				Where("waitUntil", "<=", now.Format(time.RFC3339)).
				OrderBy("waitUntil", firestore.Asc).
				OrderBy("createdAt", firestore.Asc)

			batchDocs, err := tx.Documents(batchQuery).GetAll()
			if err != nil {
				return fmt.Errorf("failed to fetch resource key tasks: %w", err)
			}

			tasks = make([]OnceTask[TaskKind], 0, len(batchDocs))
			newLeaseExpiry := now.Add(leaseDuration).Format(time.RFC3339)

			// In-memory check for mutual exclusion
			for _, doc := range batchDocs {
				var t OnceTask[TaskKind]
				if err := doc.DataTo(&t); err != nil {
					return fmt.Errorf("failed to parse task: %w", err)
				}

				// Check if ANY task in this group is currently running (leased)
				if err := checkTaskLease(t, now); err != nil {
					return err
				}

				// Task is free, add to batch and prepare lease update
				if err := tx.Update(doc.Ref, []firestore.Update{{Path: "leasedUntil", Value: newLeaseExpiry}}); err != nil {
					return err
				}
				t.LeasedUntil = newLeaseExpiry
				tasks = append(tasks, t)
			}

		} else {
			// SINGLE TASK MODE
			// Even in single mode, if ResourceKey is present, we must ensure mutual exclusion
			// with other tasks of the same ResourceKey.
			if candidate.ResourceKey != "" {
				leaseCheckQuery := m.collection.
					Where("type", "==", string(taskType)).
					Where("resourceKey", "==", candidate.ResourceKey).
					Where("doneAt", "==", "").
					Where("leasedUntil", ">", now.Format(time.RFC3339)).
					Where("id", "!=", candidateID). // Don't block on self
					Where("env", "==", m.env).
					Limit(1)

				leasedDocs, err := tx.Documents(leaseCheckQuery).GetAll()
				if err != nil {
					return fmt.Errorf("failed to check resource key conflict: %w", err)
				}
				if len(leasedDocs) > 0 {
					return errLeaseNotAvailable
				}
			}

			// Lease the single candidate
			newLeaseExpiry := now.Add(leaseDuration).Format(time.RFC3339)
			if err := tx.Update(candidateRef, []firestore.Update{{Path: "leasedUntil", Value: newLeaseExpiry}}); err != nil {
				return err
			}
			candidate.LeasedUntil = newLeaseExpiry
			tasks = []OnceTask[TaskKind]{candidate}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return tasks, nil
}

// completeBatch updates the tasks in Firestore based on the execution result.
func (m *firestoreOnceTaskManager[TaskKind]) completeBatch(ctx context.Context, tasks []OnceTask[TaskKind], execErr error) error {
	if len(tasks) == 0 {
		return nil
	}

	now := time.Now().UTC()
	err := m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		for _, task := range tasks {
			doc := m.collection.Doc(task.Id)
			var updates []firestore.Update

			if execErr == nil {
				// Success: Mark done and clear lease
				updates = []firestore.Update{
					{Path: "doneAt", Value: now.Format(time.RFC3339)},
					{Path: "leasedUntil", Value: nil},
				}
			} else {
				// Failure: Clear lease immediately to allow retry
				updates = []firestore.Update{
					{Path: "leasedUntil", Value: nil},
				}
			}

			if err := tx.Update(doc, updates); err != nil {
				return fmt.Errorf("failed to update task %s: %w", task.Id, err)
			}
		}
		return nil
	})

	return err
}

// checkTaskLease checks if a task is currently leased (locked by another executor).
// Returns errLeaseNotAvailable if the task is leased, nil if available, or an error if parsing fails.
func checkTaskLease[TaskKind ~string](task OnceTask[TaskKind], now time.Time) error {
	if task.LeasedUntil == "" {
		return nil
	}

	leasedUntil, err := time.Parse(time.RFC3339, task.LeasedUntil)
	if err != nil {
		return fmt.Errorf("failed to parse leasedUntil: %w", err)
	}

	if leasedUntil.After(now) {
		return errLeaseNotAvailable
	}

	return nil
}
