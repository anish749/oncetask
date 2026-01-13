# Task Reset

oncetask provides the ability to reset tasks that have reached terminal states (COMPLETED, FAILED, or CANCELLED) back to pending state for re-execution. This is useful for retrying failed tasks after fixing root causes, reprocessing data after bug fixes, or resuming cancelled tasks.

## Reset Methods

### Reset a Single Task

```go
err := manager.ResetTask(ctx, taskID)
```

Resets a specific task by its ID. The task must be in a terminal state (COMPLETED, FAILED, or CANCELLED).

### Reset Multiple Tasks by IDs

```go
count, err := manager.ResetTasksByIds(ctx, []string{"id1", "id2", "id3"})
```

Resets multiple tasks by their IDs in a single operation. Returns the number of tasks reset.

## Reset Behavior

When a task is reset:

1. **Execution state is cleared:**
   - `Attempts` → 0
   - `Errors` → [] (empty array)
   - `DoneAt` → "" (cleared)
   - `LeasedUntil` → "" (cleared)
   - `Result` → nil (cleared)

2. **Cancellation state is cleared:**
   - `IsCancelled` → false
   - `CancelledAt` → "" (cleared)

3. **Task is queued for immediate execution:**
   - `WaitUntil` → NoWait (immediate execution)
   - Task evaluation is triggered to wake up workers

4. **Structural metadata is preserved:**
   - `Id`, `Type`, `Env` (identity fields)
   - `CreatedAt` (original creation time)
   - `ResourceKey` (resource association)
   - `ParentRecurrenceID` (recurrence relationship)
   - `Data` (task payload)

## Eligibility

Reset only affects tasks in **terminal states**:

- ✅ **COMPLETED** - Task finished successfully
- ✅ **FAILED** - Task exhausted all retry attempts
- ✅ **CANCELLED** - Task was cancelled and is done

Tasks in non-terminal states are **skipped (no-op)**:

- ❌ **PENDING** - Task waiting to execute
- ❌ **WAITING** - Task scheduled for future
- ❌ **LEASED** - Task currently executing
- ❌ **CANCELLATION_PENDING** - Task cancelled but not done

This makes reset operations **idempotent** - calling reset on an already-pending task has no effect.

## Common Use Cases

### 1. Retry Failed Tasks After Fixing Root Cause

```go
// Tasks failed due to API outage
// After service recovers, get all failed task IDs and reset them:
failedTaskIDs := []string{"task-1", "task-2", "task-3"} // Get from your query
count, err := manager.ResetTasksByIds(ctx, failedTaskIDs)
if err != nil {
    log.Printf("Failed to reset tasks: %v", err)
}
log.Printf("Reset %d failed tasks for retry", count)
```

### 2. Reprocess Data After Bug Fix

```go
// Bug in task handler caused incorrect data processing
// After deploying fix:
affectedTaskIDs := []string{"task-1", "task-2", "task-3"}
count, err := manager.ResetTasksByIds(ctx, affectedTaskIDs)
if err != nil {
    log.Printf("Partial reset: %v", err)
}
log.Printf("Reset %d tasks for reprocessing", count)
```

### 3. Resume Cancelled Tasks

```go
// Task was cancelled by mistake or cancellation reason is resolved
err := manager.ResetTask(ctx, taskID)
if err != nil {
    log.Printf("Failed to reset cancelled task: %v", err)
}
```

### 4. Bulk Retry After Transient Errors

```go
// Query failed tasks (example - requires custom query logic)
failedTasks := getFailedTasksFromLastHour()
taskIDs := make([]string, len(failedTasks))
for i, task := range failedTasks {
    taskIDs[i] = task.Id
}

// Reset all failed tasks
count, err := manager.ResetTasksByIds(ctx, taskIDs)
log.Printf("Reset %d/%d failed tasks", count, len(taskIDs))
```

## Reset vs. Delete vs. Cancellation

| Operation | Purpose | Effect | When to Use |
|-----------|---------|--------|-------------|
| **Reset** | Re-execute task | Clears state, queues for retry | Task failed or completed but needs re-execution |
| **Delete** | Remove task permanently | Deletes from database | Task no longer needed |
| **Cancel** | Stop task execution | Marks cancelled, runs cleanup handler | Task should not execute or finish |

## Idempotency

Reset operations are **idempotent**:

```go
// First reset: task transitions from FAILED → PENDING
count1, _ := manager.ResetTask(ctx, taskID) // count1 = 1

// Second reset (immediate): task is already PENDING, no-op
count2, _ := manager.ResetTask(ctx, taskID) // count2 = 0 (skipped)
```

This makes it safe to retry reset operations without side effects.

## Partial Failures

Batch reset operations can succeed partially:

```go
taskIDs := []string{"task-1", "task-2", "task-3"}
count, err := manager.ResetTasksByIds(ctx, taskIDs)

if err != nil {
    // Some tasks failed to reset, but count shows how many succeeded
    log.Printf("Partial reset: %d succeeded, error: %v", count, err)
} else {
    log.Printf("All %d tasks reset successfully", count)
}
```

Error handling:
- Non-existent tasks are silently skipped
- Tasks in different environments are silently skipped
- Parse failures and update failures are aggregated in the error

## Environment Isolation

Reset respects environment boundaries (via `ONCE_TASK_ENV`):

```go
// Tasks in env "production" cannot be reset from env "staging"
// Silently skipped, no error
```

This prevents accidental cross-environment operations.

## Resource Key Behavior

Tasks with resource keys can be reset individually or in batches:

```go
// Get all terminal tasks with a resource key
tasks, err := manager.GetTasksByResourceKey(ctx, "file-123")

// Filter for terminal tasks and collect IDs
var terminalTaskIDs []string
for _, task := range tasks {
    status := task.GetStatus()
    if status == oncetask.TaskStatusCompleted ||
       status == oncetask.TaskStatusFailed ||
       status == oncetask.TaskStatusCancelled {
        terminalTaskIDs = append(terminalTaskIDs, task.Id)
    }
}

// Reset them
count, err := manager.ResetTasksByIds(ctx, terminalTaskIDs)

// ResourceKey is preserved after reset
// Tasks will execute with same resource key mutual exclusion
```

**Note:** If using `OnePerResourceKey` execution pattern, reset tasks will execute sequentially as before.

## Recurrence Task Reset

Resetting recurrence tasks has special behavior:

### Resetting Recurrence Generator

```go
// Reset the recurrence generator task
err := manager.ResetTask(ctx, recurrenceTaskID)
```

**Result:**
- If generator was exhausted (doneAt set), it becomes active again
- Generator will resume spawning occurrences from current time
- Useful if you want to extend a recurrence that has finished

### Resetting Occurrence Tasks

```go
// Get all occurrence tasks for a resource key
tasks, err := manager.GetTasksByResourceKey(ctx, "user-123")

// Filter for terminal occurrences and reset
var occurrenceIDs []string
for _, task := range tasks {
    if task.ParentRecurrenceID != "" && task.DoneAt != "" {
        occurrenceIDs = append(occurrenceIDs, task.Id)
    }
}

count, err := manager.ResetTasksByIds(ctx, occurrenceIDs)
```

**Result:**
- Individual occurrence tasks are reset independently
- Each will re-execute with same occurrence timestamp
- Parent recurrence generator is unaffected

## Best Practices

### 1. Fix Root Cause Before Reset

Always fix the underlying issue before resetting failed tasks:

```go
// ❌ Bad: Reset without fixing issue
manager.ResetTasksByResourceKey(ctx, taskType, resourceKey)

// ✅ Good: Fix issue first
if err := fixAPICredentials(); err != nil {
    log.Fatal("Cannot fix credentials:", err)
}
// Now reset tasks
manager.ResetTasksByResourceKey(ctx, taskType, resourceKey)
```

### 2. Monitor Reset Operations

Track reset operations for visibility:

```go
count, err := manager.ResetTasksByIds(ctx, taskIDs)
if err != nil {
    metrics.IncrementCounter("task_reset_failures", 1)
    log.Printf("Reset failed: %v", err)
} else {
    metrics.IncrementCounter("task_reset_success", count)
    log.Printf("Reset %d tasks successfully", count)
}
```

### 3. Use Batch Operations for Scale

For large numbers of tasks, use batch operations:

```go
// ✅ Good: Single batch operation
count, err := manager.ResetTasksByIds(ctx, taskIDs)

// ❌ Bad: Loop with individual resets
for _, id := range taskIDs {
    manager.ResetTask(ctx, id) // Multiple database operations
}
```

### 4. Handle Partial Failures Gracefully

```go
count, err := manager.ResetTasksByIds(ctx, taskIDs)
if err != nil {
    // Log which tasks might have failed
    log.Printf("Partial reset: %d/%d succeeded. Error: %v",
        count, len(taskIDs), err)

    // Optionally retry failed tasks
    // (implementation depends on error details)
}
```

### 5. Consider Impact on System Load

Resetting many tasks triggers immediate re-execution:

```go
// Resetting 1000 failed tasks will queue 1000 tasks immediately
// Ensure workers can handle the load

// Option 1: Reset in smaller batches
batches := chunk(taskIDs, 100)
for _, batch := range batches {
    count, err := manager.ResetTasksByIds(ctx, batch)
    log.Printf("Reset batch: %d tasks", count)
    time.Sleep(1 * time.Second) // Rate limit
}

// Option 2: Temporarily increase worker concurrency
// (configure via WithConcurrency)
```

## Complete Example

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/anish749/oncetask"
)

type DataProcessingTask struct {
    UserID    string
    DataFile  string
    ProcessAt time.Time
}

func (t DataProcessingTask) GetType() TaskKind              { return TaskKindDataProcessing }
func (t DataProcessingTask) GenerateIdempotentID() string   { return t.UserID + ":" + t.DataFile }
func (t DataProcessingTask) GetResourceKey() string         { return t.UserID }

func main() {
    ctx := context.Background()

    // Initialize manager
    manager, err := oncetask.NewManager[TaskKind](ctx, "project-id")
    if err != nil {
        log.Fatal(err)
    }

    // Register handler with retry policy
    manager.RegisterTaskHandler(
        TaskKindDataProcessing,
        func(ctx context.Context, task *oncetask.OnceTask[TaskKind]) (any, error) {
            var data DataProcessingTask
            task.ReadInto(&data)

            // Process data
            result, err := processData(ctx, data)
            if err != nil {
                return nil, err // Will retry based on policy
            }

            return result, nil
        },
        oncetask.WithRetryPolicy(oncetask.ExponentialBackoffPolicy{
            MaxAttempts: 3,
            BaseDelay:   time.Second,
            MaxDelay:    5 * time.Minute,
        }),
    )

    // Scenario: Bug in processData() caused tasks to fail
    // After deploying fix:

    // Get failed tasks for specific user
    tasks, err := manager.GetTasksByResourceKey(ctx, "user-123")
    if err != nil {
        log.Fatal(err)
    }

    // Filter for failed tasks
    var failedTaskIDs []string
    for _, task := range tasks {
        if task.GetStatus() == oncetask.TaskStatusFailed {
            failedTaskIDs = append(failedTaskIDs, task.Id)
        }
    }

    // Reset failed tasks
    count, err := manager.ResetTasksByIds(ctx, failedTaskIDs)
    if err != nil {
        log.Printf("Partial reset: %d succeeded, error: %v", count, err)
    } else {
        log.Printf("Successfully reset %d failed tasks for user-123", count)
    }

    // Alternative: Reset all failed tasks by resource key
    count, err = manager.ResetTasksByResourceKey(ctx, TaskKindDataProcessing, "user-456")
    log.Printf("Reset %d tasks for user-456", count)
}

func processData(ctx context.Context, data DataProcessingTask) (any, error) {
    // Implementation
    return nil, nil
}
```

## Monitoring and Observability

Track these metrics for reset operations:

- **Reset Count**: Number of tasks reset (per task type)
- **Reset Success Rate**: Successful resets vs. partial failures
- **Re-Execution Success Rate**: How many reset tasks succeed on retry
- **Time to Success**: Time from reset to successful completion

Example:

```go
count, err := manager.ResetTasksByIds(ctx, taskIDs)

// Track metrics
if err != nil {
    metrics.RecordResetOperation("partial_failure", count, len(taskIDs))
} else {
    metrics.RecordResetOperation("success", count, len(taskIDs))
}
```

## Common Errors

### Task Not Found

```go
err := manager.ResetTask(ctx, "non-existent-id")
// err == nil, operation is idempotent
// count == 0 (no tasks reset)
```

### Wrong Environment

```go
// Trying to reset task from different environment
err := manager.ResetTask(ctx, taskIDFromOtherEnv)
// err == nil, silently skipped
// count == 0
```

### Already Pending

```go
err := manager.ResetTask(ctx, pendingTaskID)
// err == nil, no-op (idempotent)
// count == 0 (task already pending)
```

## Next Steps

- Learn about [Cancellation](cancellation.md) for stopping task execution
- Learn about [Deletion](deletion.md) for permanently removing tasks
- Learn about [Configuration](configuration.md) for retry policies and concurrency
- Understand [Task Types](task-types.md) for different execution patterns
