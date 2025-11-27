# oncetask

Idempotent task execution with Firestore. Ensures tasks run exactly once with built-in retries, leasing, and recurrence support.

## Features

- **Idempotent execution** - Tasks identified by deterministic IDs, safe to create multiple times
- **Leasing** - Prevents concurrent execution; auto-expires on crashes
- **Resource key serialization** - Tasks with the same resource key execute one at a time
- **Grouped execution** - Process all pending tasks for a resource key together in a single handler call
- **Retries** - Configurable retry policies (exponential backoff, fixed delay, no retry)
- **Recurring tasks** - RFC 5545 RRULE support for scheduled recurring execution
- **Environment isolation** - Logical separation via `ONCE_TASK_ENV`

## Quick Start

```go
// Initialize
manager, cancel := oncetask.NewFirestoreOnceTaskManager[MyTaskKind](firestoreClient)
defer cancel()

// Register handler
manager.RegisterTaskHandler(TaskKindSendEmail, func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data SendEmailData
    task.ReadInto(&data)
    return sendEmail(data)
})

// Create task
manager.CreateTask(ctx, SendEmailData{To: "user@example.com", Subject: "Hello"})
```

## Task Types

### Single Task Handler
Each task executes independently. Use `ResourceKey` to serialize execution per resource:

```go
manager.RegisterTaskHandler(TaskKindSync, handler)
```

### Resource Key Handler
All pending tasks with the same resource key are grouped and passed to the handler together:

```go
manager.RegisterResourceKeyHandler(TaskKindSync, func(ctx context.Context, tasks []oncetask.OnceTask[MyTaskKind]) (any, error) {
    // Process all tasks for this resource key together
    return nil, processAll(tasks)
})
```

## Task Data Interface

Implement `OnceTaskData` for your task types:

```go
type SendEmailData struct {
    To      string
    Subject string
}

func (d SendEmailData) GetType() MyTaskKind { return TaskKindSendEmail }
func (d SendEmailData) GenerateIdempotentID() string {
    return fmt.Sprintf("email:%s:%s", d.To, hashContent(d.Subject))
}
```

Optional interfaces:
- `ResourceKeyProvider` - Enable resource-level serialization
- `ScheduledTask` - Delay execution until a specific time
- `RecurrenceProvider` - Define recurring schedules

## Configuration

```go
manager.RegisterTaskHandler(taskKind, handler,
    oncetask.WithRetryPolicy(oncetask.ExponentialBackoffPolicy{
        MaxAttempts: 5,
        BaseDelay:   time.Second,
        MaxDelay:    time.Minute,
    }),
    oncetask.WithLeaseDuration(15*time.Minute),
    oncetask.WithConcurrency(3), // Number of concurrent workers (default: 1)
)
```

Available options:
- `WithRetryPolicy(policy)` - Custom retry policy (exponential backoff, fixed delay)
- `WithNoRetry()` - Disable retries; tasks fail permanently on first error
- `WithLeaseDuration(duration)` - How long a task is leased during execution (default: 10 minutes)
- `WithConcurrency(n)` - Number of concurrent workers processing tasks (default: 1)
- `WithCancellationHandler(handler)` - Register cleanup handler for cancelled tasks (optional)
- `WithCancellationRetryPolicy(policy)` - Configure retry behavior for cancellation handlers

## Recurrence

Tasks can spawn recurring occurrences using RFC 5545 RRULE:

```go
type WeeklyReportData struct {
    UserID string
}

func (d WeeklyReportData) GetRecurrence() *oncetask.Recurrence {
    return &oncetask.Recurrence{
        RRule:   "FREQ=WEEKLY;BYDAY=MO",
        DTStart: time.Now().Format(time.RFC3339),
    }
}
```

The recurrence task acts as a generator, spawning individual occurrence tasks that are processed by your handler.

## Cancellation

Tasks can be cancelled at any time, with optional cleanup handlers:

```go
// Cancel a single task
manager.CancelTask(ctx, taskID)

// Cancel all tasks for a resource key
count, err := manager.CancelTasksByResourceKey(ctx, TaskKindSync, "resource-123")

// Cancel multiple tasks by IDs
count, err := manager.CancelTasksByIds(ctx, []string{"id1", "id2", "id3"})
```

### Cancellation Behavior

When a task is cancelled:
- `isCancelled` is set to `true`
- `waitUntil` is set to epoch (immediate execution)
- If a cancellation handler is registered, it runs with the cancellation retry policy
- If no cancellation handler is registered, the task is immediately marked as done-cancelled

### Cancellation Handler Example

```go
manager.RegisterTaskHandler(TaskKindSync, handler,
    oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        // Perform cleanup operations
        var data SyncData
        task.ReadInto(&data)
        return nil, cleanupResources(data)
    }),
    oncetask.WithCancellationRetryPolicy(oncetask.ExponentialBackoffPolicy{
        MaxAttempts: 5,
        BaseDelay:   time.Second,
        MaxDelay:    10 * time.Minute,
    }),
)
```

### Recurrence Task Cancellation

When a recurrence task (generator) is cancelled:
- The recurrence task is marked as done
- No new occurrence tasks are spawned
- Already-spawned occurrence tasks continue normally
- Individual occurrences can be cancelled separately if needed
