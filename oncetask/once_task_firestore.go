package oncetask

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrHandlerAlreadyExists is returned when trying to register a handler for a task type that already has a handler
var ErrHandlerAlreadyExists = errors.New("handler for this task type already exists")

// firestoreOnceTaskManager implements Manager using Firestore
//
//nolint:govet // fieldalignment: struct field order prioritizes logical grouping over memory optimization
type firestoreOnceTaskManager[TaskKind ~string] struct {
	client *firestore.Client
	ctx    context.Context // background context in which the task handlers run, can be cancelled during shutdown

	// Handler registration
	mu                  sync.RWMutex
	taskHandlers        map[TaskKind]Handler[TaskKind]
	resourceKeyHandlers map[TaskKind]ResourceKeyHandler[TaskKind]
	handlerConfigs      map[TaskKind]handlerConfig
	evaluateChans       map[TaskKind]chan struct{} // Per task type channels for immediate evaluation

	// Composed components
	queryBuilder *firestoreQueryBuilder
}

// NewFirestoreOnceTaskManager creates a new firestore once task manager.
// The provided context is used as the parent for all background task processing goroutines.
// Context values (trace IDs, tenant IDs, etc.) will be inherited by task handlers.
// Returns the manager and a cleanup function that cancels all running goroutines.
func NewFirestoreOnceTaskManager[TaskKind ~string](ctx context.Context, client *firestore.Client) (m Manager[TaskKind], cleanup func()) {
	ctx, cleanup = context.WithCancel(ctx)

	queryBuilder := newFirestoreQueryBuilder(client.Collection(CollectionOnceTasks), getTaskEnv())

	m = &firestoreOnceTaskManager[TaskKind]{
		client:              client,
		ctx:                 ctx,
		taskHandlers:        make(map[TaskKind]Handler[TaskKind]),
		resourceKeyHandlers: make(map[TaskKind]ResourceKeyHandler[TaskKind]),
		handlerConfigs:      make(map[TaskKind]handlerConfig),
		evaluateChans:       make(map[TaskKind]chan struct{}),

		queryBuilder: queryBuilder,
	}
	return m, cleanup
}

// CreateTask creates a once task to Firestore.
// The task parameter should be created using fs_models/once.NewOnceTask().
// If a task with the same ID already exists, logs and returns nil (idempotent).
func (m *firestoreOnceTaskManager[TaskKind]) CreateTask(ctx context.Context, taskData Data[TaskKind]) (bool, error) {
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
	handler Handler[TaskKind],
	opts ...HandlerOption,
) error {
	// Build config with defaults
	config := defaultHandlerConfig
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

	for i := 0; i < config.Concurrency; i++ {
		go m.runLoop(taskType, evaluateChan)
	}

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
	handler ResourceKeyHandler[TaskKind],
	opts ...HandlerOption,
) error {
	// Build config with defaults
	config := defaultHandlerConfig
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

	for i := 0; i < config.Concurrency; i++ {
		go m.runLoop(taskType, evaluateChan)
	}

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

		// Separate cancelled and normal tasks
		var cancelledTasks []OnceTask[TaskKind]
		var normalTasks []OnceTask[TaskKind]

		for _, task := range executableTasks {
			if task.IsCancelled {
				cancelledTasks = append(cancelledTasks, task)
			} else {
				normalTasks = append(normalTasks, task)
			}
		}

		// Process cancelled tasks individually with cancellation handler
		if len(cancelledTasks) > 0 {
			cancellationHandler := getCancellationHandler[TaskKind](config)
			for _, task := range cancelledTasks {
				ctx := withTaskContext(m.ctx, task.Id, task.ResourceKey)
				result, execErr := cancellationHandler(ctx, &task)
				if err := m.completeBatch(ctx, []OnceTask[TaskKind]{task}, execErr, result, config); err != nil {
					slog.ErrorContext(ctx, "Failed to complete cancelled task", "error", err, "taskId", task.Id)
				}
			}
		}

		if len(normalTasks) == 0 {
			continue
		}

		var execErr error
		var result any

		if hasTask {
			if len(normalTasks) != 1 {
				slog.ErrorContext(m.ctx, "Single task handler claimed multiple tasks", "taskType", taskType, "count", len(normalTasks))
				execErr = fmt.Errorf("expected 1 task, got %d", len(normalTasks))
			} else {
				result, execErr = taskHandler(withSingleTaskContext(m.ctx, normalTasks), &normalTasks[0])
			}
		} else if hasResource {
			result, execErr = resourceHandler(withResourceKeyTaskContext(m.ctx, normalTasks), normalTasks)
		}

		if err := m.completeBatch(m.ctx, normalTasks, execErr, result, config); err != nil {
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
	config handlerConfig,
	hasResourceKeyHandler bool,
) ([]OnceTask[TaskKind], error) {
	var tasks []OnceTask[TaskKind]

	err := m.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		now := time.Now().UTC()

		// Reset tasks slice on retry
		tasks = nil

		// 1. Find and lock a candidate task
		candidateQuery := m.queryBuilder.readyTasks(string(taskType), now).Limit(1)

		candidateSnaps, err := queryAndLock(tx, &candidateQuery)
		if err != nil {
			return fmt.Errorf("failed to query candidate task: %w", err)
		}
		if len(candidateSnaps) == 0 {
			return nil // No tasks ready
		}

		candidateSnap := candidateSnaps[0]
		var candidate OnceTask[TaskKind]
		if err := candidateSnap.DataTo(&candidate); err != nil {
			return fmt.Errorf("failed to parse candidate task: %w", err)
		}

		// Double check lease (in case of transaction retry with fresh data)
		if err := checkTaskLease(candidate.LeasedUntil, now); err != nil {
			return err
		}

		newLeaseExpiry := now.Add(config.LeaseDuration).Format(time.RFC3339)

		// 2. Determine Batch Scope (Single vs Resource Group)
		if hasResourceKeyHandler && candidate.ResourceKey != "" {
			// RESOURCE KEY MODE
			// Fetch and lock ALL ready tasks for this resource key
			batchQuery := m.queryBuilder.readyTasksForResourceKey(string(taskType), candidate.ResourceKey, now)

			docSnaps, err := queryAndLock(tx, &batchQuery)
			if err != nil {
				return fmt.Errorf("failed to fetch resource key tasks: %w", err)
			}

			tasks = make([]OnceTask[TaskKind], 0, len(docSnaps))

			// Check lease status and lease all tasks
			for _, docSnap := range docSnaps {
				var t OnceTask[TaskKind]
				if err := docSnap.DataTo(&t); err != nil {
					return fmt.Errorf("failed to parse task: %w", err)
				}

				// Check if this task is currently running (leased)
				if err := checkTaskLease(t.LeasedUntil, now); err != nil {
					return err
				}

				// Task is free, lease it and add to batch
				if err := leaseTask(tx, docSnap.Ref, &t, newLeaseExpiry); err != nil {
					return err
				}
				tasks = append(tasks, t)
			}
		} else {
			// SINGLE TASK MODE
			// Check if another task with the same resourceKey is already being processed.
			// This ensures mutual exclusion even when using a single task handler with resourceKey.
			if err := checkResourceAvailable(tx, m.queryBuilder, string(taskType), candidate.ResourceKey, now); err != nil {
				return err
			}

			// Lease the single candidate
			if err := leaseTask(tx, candidateSnap.Ref, &candidate, newLeaseExpiry); err != nil {
				return err
			}
			tasks = []OnceTask[TaskKind]{candidate}
		}

		return nil
	})

	if err != nil {
		// Aborted errors are treated as errLeaseNotAvailable for graceful retry.
		// Cross-transaction contention is expected with concurrent workers, so we don't log it.
		// Other Aborted reasons are unexpected and should be logged.
		if status.Code(err) == codes.Aborted {
			if !strings.Contains(err.Error(), "cross-transaction contention") {
				slog.WarnContext(ctx, "Transaction aborted for unexpected reason", "error", err)
			}
			return nil, errLeaseNotAvailable
		}
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
	config handlerConfig,
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
				updates = processTaskFailure(execErr, config, currentTask, now)
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

// GetTasksByIds retrieves tasks by their IDs from Firestore.
// Returns tasks in no guaranteed order.
// Tasks not found are omitted from the result (no error).
// The tasks must belong to the current environment (from ONCE_TASK_ENV).
func (m *firestoreOnceTaskManager[TaskKind]) GetTasksByIds(ctx context.Context, ids []string) ([]OnceTask[TaskKind], error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build document references for all IDs
	docRefs := make([]*firestore.DocumentRef, len(ids))
	for i, id := range ids {
		docRefs[i] = m.queryBuilder.doc(id)
	}

	// Fetch all documents in a single batch
	docSnaps, err := m.client.GetAll(ctx, docRefs)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to fetch tasks by IDs", "count", len(ids), "error", err)
		return nil, fmt.Errorf("failed to fetch tasks by IDs: %w", err)
	}

	env := getTaskEnv()
	tasks := make([]OnceTask[TaskKind], 0, len(docSnaps))
	for _, docSnap := range docSnaps {
		if !docSnap.Exists() {
			continue // Task not found, skip
		}

		var task OnceTask[TaskKind]
		if err := docSnap.DataTo(&task); err != nil {
			slog.ErrorContext(ctx, "Failed to unmarshal task", "taskId", docSnap.Ref.ID, "error", err)
			return nil, fmt.Errorf("failed to unmarshal task %s: %w", docSnap.Ref.ID, err)
		}

		// Filter by environment (since GetAll doesn't support queries)
		if task.Env != env {
			continue
		}

		tasks = append(tasks, task)
	}

	return tasks, nil
}

// CreateTasks creates multiple once tasks using Firestore BulkWriter.
//
// This method is NON-ATOMIC: individual task creations can succeed or fail independently.
// This is intentional - it allows partial success when some tasks already exist or fail,
// rather than failing the entire batch.
//
// Idempotency: Tasks that already exist (same ID) are silently skipped and do not
// contribute to the returned error. This makes it safe to retry the entire batch.
//
// Returns:
//   - (created count, nil) if all tasks were created or already existed
//   - (created count, error) if at least one task failed for a reason other than already-existing
//
// The returned error aggregates all non-AlreadyExists failures using errors.Join.
func (m *firestoreOnceTaskManager[TaskKind]) CreateTasks(ctx context.Context, taskDataList []Data[TaskKind]) (int, error) {
	if len(taskDataList) == 0 {
		return 0, nil
	}

	bw := m.client.BulkWriter(ctx)

	// Track jobs to collect results after flush.
	// BulkWriter queues writes and executes them in batches of up to 20.
	type jobInfo struct {
		job    *firestore.BulkWriterJob
		taskID string
	}
	jobs := make([]jobInfo, 0, len(taskDataList))
	taskTypes := make(map[TaskKind]struct{})

	// Collect all errors (bulk writer queuing + write failures). We don't return early
	// because some bulk writes may still succeed.
	var errs []error

	// Queue all creates - these are not executed until End() is called
	for _, taskData := range taskDataList {
		task, err := newOnceTask(taskData)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to create task struct: %w", err))
			continue
		}

		doc := m.queryBuilder.doc(task.Id)
		job, err := bw.Create(doc, task)
		if err != nil {
			// BulkWriter is closed or other immediate error
			errs = append(errs, fmt.Errorf("failed to queue task %s: %w", task.Id, err))
			continue
		}

		jobs = append(jobs, jobInfo{job: job, taskID: task.Id})
		taskTypes[task.Type] = struct{}{}
	}

	// Flush all writes and close the BulkWriter.
	// After End(), all jobs will have results available.
	bw.End()

	// Collect results from each job.
	// Each job can succeed or fail independently (non-atomic).
	var created int
	for _, j := range jobs {
		_, err := j.job.Results()
		if err == nil {
			created++
			continue
		}

		// AlreadyExists is expected for idempotent retries - silently skip
		if status.Code(err) == codes.AlreadyExists {
			slog.InfoContext(ctx, "Task already exists, skipped", "taskId", j.taskID)
			continue
		}

		// Any other error is a real failure
		slog.ErrorContext(ctx, "Failed to create task", "taskId", j.taskID, "error", err)
		errs = append(errs, fmt.Errorf("task %s: %w", j.taskID, err))
	}

	// Trigger evaluation for all task types
	for taskType := range taskTypes {
		m.evaluateNow(taskType)
	}

	// Return aggregated error if any tasks failed (excluding AlreadyExists)
	if len(errs) > 0 {
		return created, fmt.Errorf("failed to create %d tasks: %w", len(errs), errors.Join(errs...))
	}

	return created, nil
}
