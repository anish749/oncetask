# Cancellation

oncetask provides robust task cancellation with optional cleanup handlers, allowing you to gracefully terminate tasks and release resources.

## Cancellation Methods

### Cancel a Single Task

```go
err := manager.CancelTask(ctx, taskID)
```

Cancels a specific task by its ID.

### Cancel Tasks by Resource Key

```go
count, err := manager.CancelTasksByResourceKey(ctx, TaskKindSync, "resource-123")
```

Cancels all tasks of a specific type with the given resource key. Returns the number of tasks cancelled.

### Cancel Multiple Tasks by IDs

```go
count, err := manager.CancelTasksByIds(ctx, []string{"id1", "id2", "id3"})
```

Cancels multiple tasks by their IDs in a single operation. Returns the number of tasks cancelled.

## Cancellation Behavior

When a task is cancelled:

1. **Task is marked for cancellation:**
   - `isCancelled` flag is set to `true`
   - `waitUntil` is set to `NoWait` (immediate execution)

2. **Cancellation handler runs (if registered):**
   - If a cancellation handler is configured, it executes with the cancellation retry policy
   - The handler can perform cleanup operations

3. **Task is marked as done:**
   - If no cancellation handler is registered, task is immediately marked as `done-cancelled`
   - If a cancellation handler is registered and succeeds, task is marked as `done-cancelled`
   - If cancellation handler fails and exhausts retries, task is marked as `done-failed`

## Cancellation Handlers

Cancellation handlers allow you to perform cleanup when a task is cancelled.

### Registering a Cancellation Handler

```go
manager.RegisterTaskHandler(
    TaskKindProcessVideo,
    func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data ProcessVideoData
        task.ReadInto(&data)
        return processVideo(ctx, data)
    },
    oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data ProcessVideoData
        task.ReadInto(&data)

        // Cleanup: delete temporary files, release resources, etc.
        return nil, cleanupVideoProcessing(ctx, data)
    }),
)
```

### When to Use Cancellation Handlers

Use cancellation handlers when:

- Tasks acquire resources that need cleanup (files, connections, locks)
- Tasks create side effects that should be rolled back
- External systems need to be notified of cancellation
- Partial work needs to be cleaned up

### Cancellation Handler Example

```go
type FileProcessingData struct {
    TempFileID string
    OutputPath string
}

manager.RegisterTaskHandler(
    TaskKindFileProcessing,
    func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data FileProcessingData
        task.ReadInto(&data)

        // Process file
        return processFile(ctx, data)
    },
    // Cleanup handler
    oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data FileProcessingData
        task.ReadInto(&data)

        // Delete temporary files
        if err := storage.DeleteFile(ctx, data.TempFileID); err != nil {
            return nil, fmt.Errorf("cleanup failed: %w", err)
        }

        return nil, nil
    }),
    // Retry cleanup aggressively
    oncetask.WithCancellationRetryPolicy(oncetask.ExponentialBackoffPolicy{
        MaxAttempts: 10,
        BaseDelay:   time.Second,
        MaxDelay:    1 * time.Minute,
    }),
)
```

## Cancellation Retry Policy

Cancellation handlers have their own retry policy, separate from the main task retry policy.

### Default Behavior

If not specified, cancellation handlers use the same retry policy as the main task.

### Custom Cancellation Retry Policy

```go
manager.RegisterTaskHandler(
    taskKind,
    handler,
    oncetask.WithCancellationHandler(cleanupHandler),
    oncetask.WithCancellationRetryPolicy(oncetask.ExponentialBackoffPolicy{
        MaxAttempts: 5,
        BaseDelay:   time.Second,
        MaxDelay:    10 * time.Minute,
    }),
)
```

**Recommendation:** Use aggressive retry policies for cancellation handlers to ensure cleanup completes.

## Cancellation States

```
┌─────────────┐
│ Task        │
│ (Running)   │
└──────┬──────┘
       │
       │ CancelTask() called
       │
       ▼
┌─────────────────┐
│ Task            │
│ (isCancelled=true)
└──────┬──────────┘
       │
       ├─> No cancellation handler
       │   └─> done-cancelled
       │
       └─> Cancellation handler registered
           │
           ├─> Handler succeeds
           │   └─> done-cancelled
           │
           └─> Handler fails (after retries)
               └─> done-failed
```

## Recurrence Task Cancellation

Cancelling a recurrence task (generator) has special behavior:

```go
// Cancel the recurrence task
manager.CancelTask(ctx, recurrenceTaskID)
```

**Result:**

- The recurrence task is marked as done
- No new occurrence tasks are spawned
- Already-spawned occurrence tasks continue running normally

**To cancel everything:**

```go
// 1. Cancel the recurrence task
manager.CancelTask(ctx, recurrenceTaskID)

// 2. Cancel all spawned occurrences by resource key
manager.CancelTasksByResourceKey(ctx, taskKind, resourceKey)
```

See [Recurrence documentation](recurrence.md) for more details.

## Best Practices

### 1. Make Cancellation Handlers Idempotent

Cancellation handlers may be retried, so make them idempotent:

```go
oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data MyTaskData
    task.ReadInto(&data)

    // Check if cleanup already done
    if alreadyCleanedUp(data.ResourceID) {
        return nil, nil
    }

    // Perform cleanup
    return nil, cleanup(data)
})
```

### 2. Use Aggressive Retry Policies

Cleanup is critical, so retry aggressively:

```go
oncetask.WithCancellationRetryPolicy(oncetask.ExponentialBackoffPolicy{
    MaxAttempts: 10,
    BaseDelay:   time.Second,
    MaxDelay:    5 * time.Minute,
})
```

### 3. Log Cleanup Failures

Always log when cleanup fails:

```go
oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    err := cleanup()
    if err != nil {
        log.Printf("Cleanup failed for task %s: %v", task.ID, err)
        return nil, err
    }
    return nil, nil
})
```

### 4. Handle Partial Cleanup

Consider partial cleanup states:

```go
oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data MyTaskData
    task.ReadInto(&data)

    // Cleanup each resource independently
    var errs []error

    if err := cleanupResource1(data); err != nil {
        errs = append(errs, err)
    }

    if err := cleanupResource2(data); err != nil {
        errs = append(errs, err)
    }

    if len(errs) > 0 {
        return nil, fmt.Errorf("cleanup errors: %v", errs)
    }

    return nil, nil
})
```

### 5. Avoid Long-Running Cleanup

Keep cancellation handlers fast:

- Delegate heavy cleanup to separate tasks if needed
- Use timeouts to prevent hanging
- Mark resources for async cleanup if necessary

## Complete Example

```go
type LongRunningJobData struct {
    JobID       string
    TempFiles   []string
    ReservedCPU int
}

// Main task handler
manager.RegisterTaskHandler(
    TaskKindLongRunningJob,
    func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data LongRunningJobData
        task.ReadInto(&data)

        // Reserve CPU
        reserveCPU(data.ReservedCPU)

        // Process job
        result, err := processJob(ctx, data)

        // Release CPU
        releaseCPU(data.ReservedCPU)

        return result, err
    },
    // Cancellation handler
    oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data LongRunningJobData
        task.ReadInto(&data)

        // Release reserved CPU
        releaseCPU(data.ReservedCPU)

        // Delete temporary files
        for _, file := range data.TempFiles {
            if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
                log.Printf("Failed to delete temp file %s: %v", file, err)
            }
        }

        // Notify job was cancelled
        notifyJobCancelled(data.JobID)

        return nil, nil
    }),
    // Retry cleanup aggressively
    oncetask.WithCancellationRetryPolicy(oncetask.ExponentialBackoffPolicy{
        MaxAttempts: 10,
        BaseDelay:   time.Second,
        MaxDelay:    1 * time.Minute,
    }),
)

// Later: cancel the job
manager.CancelTask(ctx, jobTaskID)
```

## Monitoring Cancellation

Track cancellation metrics:

- Number of tasks cancelled
- Cancellation handler success rate
- Cleanup failure reasons
- Time to complete cancellation

## Next Steps

- Learn about [Configuration](configuration.md) for retry policies and concurrency
- Explore [Recurrence](recurrence.md) for scheduled recurring tasks
- Understand [Task Types](task-types.md) for different execution patterns
