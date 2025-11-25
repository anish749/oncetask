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

// ErrHandlerAlreadyExists is returned when trying to register a handler for a task type that already has a handler
var ErrHandlerAlreadyExists = errors.New("handler for this task type already exists")

// firestoreOnceTaskManager implements OnceTaskManager using Firestore
type firestoreOnceTaskManager[TaskKind ~string] struct {
	client *firestore.Client
	ctx    context.Context // background context in which the task handlers run, can be cancelled during shutdown

	// Handler registration
	mu                  sync.RWMutex
	taskHandlers        map[TaskKind]OnceTaskHandler[TaskKind]
	resourceKeyHandlers map[TaskKind]OnceTaskResourceKeyHandler[TaskKind]
	handlerConfigs      map[TaskKind]HandlerConfig
	evaluateChans       map[TaskKind]chan struct{} // Per task type channels for immediate evaluation

	// Composed components
	queryBuilder *firestoreQueryBuilder
}

// NewFirestoreOnceTaskManager creates a new firestore once task manager.
// Returns the repository and a cancel function that cancels all running goroutines.
func NewFirestoreOnceTaskManager[TaskKind ~string](client *firestore.Client) (OnceTaskManager[TaskKind], func()) {
	ctx, cancel := context.WithCancel(context.Background())

	queryBuilder := newFirestoreQueryBuilder(client.Collection(CollectionOnceTasks), getTaskEnv())

	m := &firestoreOnceTaskManager[TaskKind]{
		client:              client,
		ctx:                 ctx,
		taskHandlers:        make(map[TaskKind]OnceTaskHandler[TaskKind]),
		resourceKeyHandlers: make(map[TaskKind]OnceTaskResourceKeyHandler[TaskKind]),
		handlerConfigs:      make(map[TaskKind]HandlerConfig),
		evaluateChans:       make(map[TaskKind]chan struct{}),

		queryBuilder: queryBuilder,
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
	doc := m.queryBuilder.doc(task.Id)

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
// If the handler returns an error, the task will be retried according to the handler config.
// Returns ErrHandlerAlreadyExists if a handler for this task type is already registered.
func (m *firestoreOnceTaskManager[TaskKind]) RegisterTaskHandler(
	taskType TaskKind,
	handler OnceTaskHandler[TaskKind],
	opts ...HandlerOption,
) error {
	// Build config with defaults
	config := DefaultHandlerConfig
	for _, opt := range opts {
		opt(&config)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if handler already exists (either single or resource key)
	if _, exists := m.taskHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s", ErrHandlerAlreadyExists, taskType)
	}
	if _, exists := m.resourceKeyHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s (resource key handler registered)", ErrHandlerAlreadyExists, taskType)
	}

	// Store the handler and config
	m.taskHandlers[taskType] = handler
	m.handlerConfigs[taskType] = config

	// Create evaluate channel for this task type (buffered to avoid blocking)
	evaluateChan := make(chan struct{}, 1)
	m.evaluateChans[taskType] = evaluateChan

	go m.runLoop(taskType, evaluateChan)

	return nil
}

// RegisterResourceKeyHandler registers a resource key handler and starts a goroutine for that task type.
// All pending tasks with the same resource key are grouped together and ordered by WaitUntil.
// The handler is responsible for any additional ordering logic.
// If the handler returns nil, all tasks for that resource key are marked as done.
// If the handler returns an error, all tasks for that resource key will be retried according to handler config.
// Returns ErrHandlerAlreadyExists if a handler for this task type is already registered.
func (m *firestoreOnceTaskManager[TaskKind]) RegisterResourceKeyHandler(
	taskType TaskKind,
	handler OnceTaskResourceKeyHandler[TaskKind],
	opts ...HandlerOption,
) error {
	// Build config with defaults
	config := DefaultHandlerConfig
	for _, opt := range opts {
		opt(&config)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if handler already exists (either single or resource key)
	if _, exists := m.taskHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s (single handler registered)", ErrHandlerAlreadyExists, taskType)
	}
	if _, exists := m.resourceKeyHandlers[taskType]; exists {
		return fmt.Errorf("%w: task type %s", ErrHandlerAlreadyExists, taskType)
	}

	// Store the handler and config
	m.resourceKeyHandlers[taskType] = handler
	m.handlerConfigs[taskType] = config

	// Create evaluate channel for this task type (buffered to avoid blocking)
	evaluateChan := make(chan struct{}, 1)
	m.evaluateChans[taskType] = evaluateChan

	go m.runLoop(taskType, evaluateChan)

	return nil
}

// runLoop is the main processing loop for both single and resource-key tasks.
// It repeatedly claims batches of tasks, executes them, and completes them.
//
// Recurrence tasks are handled specially: after claiming, they spawn an occurrence
// task and reschedule themselves - no handler execution needed.
func (m *firestoreOnceTaskManager[TaskKind]) runLoop(
	taskType TaskKind,
	evaluateChan chan struct{},
) {
	slog.InfoContext(m.ctx, "Starting task consumer loop", "taskType", taskType)
	defer slog.InfoContext(m.ctx, "Task consumer loop stopped", "taskType", taskType)

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

		// 2. Get handler config and handlers
		m.mu.RLock()
		config := m.handlerConfigs[taskType]
		taskHandler, hasTask := m.taskHandlers[taskType]
		resourceHandler, hasResource := m.resourceKeyHandlers[taskType]
		m.mu.RUnlock()

		// 3. Claim Tasks
		tasks, err := m.claimTasks(m.ctx, taskType, config, hasResource)
		if err != nil {
			if !errors.Is(err, errLeaseNotAvailable) {
				slog.ErrorContext(m.ctx, "Failed to claim task batch", "error", err, "taskType", taskType)
			}
			shouldWait = true
			continue
		}

		if len(tasks) == 0 {
			shouldWait = true
			continue
		}

		// We found work, check again immediately after this batch
		shouldWait = false

		// 4. Handle recurrence tasks (spawn occurrence, reschedule parent)
		// Filter out recurrence tasks - they don't need handler execution
		executableTasks := m.filterAndSpawnOccurrences(m.ctx, tasks)

		// 5. Execute remaining tasks (non-recurrence tasks)
		if len(executableTasks) == 0 {
			continue // Only had recurrence tasks, already processed
		}

		var execErr error
		var result any

		if hasTask {
			if len(executableTasks) != 1 {
				slog.ErrorContext(m.ctx, "Single task handler claimed multiple tasks", "taskType", taskType, "count", len(executableTasks))
				execErr = fmt.Errorf("expected 1 task, got %d", len(executableTasks))
			} else {
				ctx := withTaskContext(m.ctx, executableTasks[0].Id, executableTasks[0].ResourceKey)
				result, execErr = taskHandler(ctx, &executableTasks[0])
			}
		} else if hasResource {
			ctx := m.ctx
			if len(executableTasks) == 1 {
				ctx = withTaskContext(m.ctx, executableTasks[0].Id, executableTasks[0].ResourceKey)
			} else if len(executableTasks) > 0 && executableTasks[0].ResourceKey != "" {
				ctx = withTaskContext(m.ctx, "", executableTasks[0].ResourceKey)
			}
			result, execErr = resourceHandler(ctx, executableTasks)
		}

		if execErr != nil {
			slog.ErrorContext(m.ctx, "Task execution failed", "error", execErr, "taskType", taskType, "batchSize", len(executableTasks))
		}

		// 6. Complete executed tasks
		if err := m.completeBatch(m.ctx, executableTasks, execErr, result, config); err != nil {
			slog.ErrorContext(m.ctx, "Failed to complete task batch", "error", err, "taskType", taskType)
		}
	}
}

// leaseTask updates the task's lease in Firestore and updates the local copy.
// Sets leasedUntil to newLeaseExpiry and increments attempts.
func leaseTask[TaskKind ~string](tx *firestore.Transaction, docRef *firestore.DocumentRef, task *OnceTask[TaskKind], newLeaseExpiry string) error {
	updates := []firestore.Update{
		{Path: "leasedUntil", Value: newLeaseExpiry},
		{Path: "attempts", Value: firestore.Increment(1)},
	}
	if err := tx.Update(docRef, updates); err != nil {
		return err
	}
	task.LeasedUntil = newLeaseExpiry
	task.Attempts++
	return nil
}

// claimTasks attempts to find and lock ready tasks (one or more, depending on handler type).
// Returns a slice of locked tasks. If no tasks are available, returns empty slice and nil error.
func (m *firestoreOnceTaskManager[TaskKind]) claimTasks(
	ctx context.Context,
	taskType TaskKind,
	config HandlerConfig,
	hasResourceKeyHandler bool,
) ([]OnceTask[TaskKind], error) {
	var tasks []OnceTask[TaskKind]
	now := time.Now().UTC()

	err := m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// 1. Find a candidate task
		// We look for ONE task that is ready.
		query := m.queryBuilder.
			readyTasks(string(taskType), now).
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
		if err := checkTaskLease(candidate.LeasedUntil, now); err != nil {
			return err
		}

		// 2. Determine Batch Scope (Single vs Resource Group)
		newLeaseExpiry := now.Add(config.LeaseDuration).Format(time.RFC3339)

		if hasResourceKeyHandler && candidate.ResourceKey != "" {
			// RESOURCE KEY MODE
			// Fetch ALL ready tasks for this resource key
			batchQuery := m.queryBuilder.readyTasksForResourceKey(string(taskType), candidate.ResourceKey, now)

			batchDocs, err := tx.Documents(batchQuery).GetAll()
			if err != nil {
				return fmt.Errorf("failed to fetch resource key tasks: %w", err)
			}

			tasks = make([]OnceTask[TaskKind], 0, len(batchDocs))

			// In-memory check for mutual exclusion and lease all tasks
			for _, doc := range batchDocs {
				var t OnceTask[TaskKind]
				if err := doc.DataTo(&t); err != nil {
					return fmt.Errorf("failed to parse task: %w", err)
				}

				// Check if ANY task in this group is currently running (leased)
				if err := checkTaskLease(t.LeasedUntil, now); err != nil {
					return err
				}

				// Task is free, lease it and add to batch
				if err := leaseTask(tx, doc.Ref, &t, newLeaseExpiry); err != nil {
					return err
				}
				tasks = append(tasks, t)
			}

		} else {
			// SINGLE TASK MODE
			// Check mutual exclusion (handles empty resourceKey internally)
			if err := checkResourceAvailable(tx, m.queryBuilder, string(taskType), candidate.ResourceKey, candidateID, now); err != nil {
				return err
			}

			// Lease the single candidate
			if err := leaseTask(tx, candidateRef, &candidate, newLeaseExpiry); err != nil {
				return err
			}
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
// Only handles non-recurrence tasks (recurrence tasks are handled separately in processRecurrenceTasks).
func (m *firestoreOnceTaskManager[TaskKind]) completeBatch(
	ctx context.Context,
	tasks []OnceTask[TaskKind],
	execErr error,
	result any,
	config HandlerConfig,
) error {
	if len(tasks) == 0 {
		return nil
	}

	now := time.Now().UTC()
	return m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Phase 1: Read all tasks (Firestore requires all reads before writes)
		docRefs := make([]*firestore.DocumentRef, len(tasks))
		for i, task := range tasks {
			docRefs[i] = m.queryBuilder.doc(task.Id)
		}

		docSnaps, err := tx.GetAll(docRefs)
		if err != nil {
			return fmt.Errorf("failed to get tasks: %w", err)
		}

		// Phase 2: Parse and write updates
		for i, docSnap := range docSnaps {
			var currentTask OnceTask[TaskKind]
			if err := docSnap.DataTo(&currentTask); err != nil {
				return fmt.Errorf("failed to parse task %s: %w", tasks[i].Id, err)
			}

			var updates []firestore.Update
			if execErr == nil {
				updates = processTaskSuccess(result, now)
			} else {
				updates = processTaskFailure(execErr, currentTask.Errors, currentTask.Attempts, config.RetryPolicy, now)
			}

			if err := tx.Update(docRefs[i], updates); err != nil {
				return fmt.Errorf("failed to update task %s: %w", currentTask.Id, err)
			}
		}
		return nil
	})
}

// GetTasksByResourceKey retrieves all tasks with the given resource key from Firestore.
// Returns tasks ordered by CreatedAt (oldest first).
// The tasks must belong to the current environment (from ONCE_TASK_ENV).
func (m *firestoreOnceTaskManager[TaskKind]) GetTasksByResourceKey(ctx context.Context, resourceKey string) ([]OnceTask[TaskKind], error) {
	query := m.queryBuilder.byResourceKey(resourceKey)

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to query tasks by resource key", "resourceKey", resourceKey, "error", err)
		return nil, fmt.Errorf("failed to query tasks by resource key %s: %w", resourceKey, err)
	}

	tasks := make([]OnceTask[TaskKind], 0, len(docs))
	for _, doc := range docs {
		var task OnceTask[TaskKind]
		if err := doc.DataTo(&task); err != nil {
			slog.ErrorContext(ctx, "Failed to unmarshal task", "taskId", doc.Ref.ID, "error", err)
			return nil, fmt.Errorf("failed to unmarshal task %s: %w", doc.Ref.ID, err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}
