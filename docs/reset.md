# Task Reset

Reset tasks in terminal states (COMPLETED, FAILED, or CANCELLED) back to pending for re-execution.

## Methods

### Reset a Single Task

```go
err := manager.ResetTask(ctx, taskID)
```

### Reset Multiple Tasks

```go
count, err := manager.ResetTasksByIds(ctx, []string{"id1", "id2", "id3"})
```

Returns the number of tasks reset. Partial failures return both count and aggregated error.

## Reset Behavior

**Cleared:**
- Execution state: `Attempts`, `Errors`, `DoneAt`, `LeasedUntil`, `Result`
- Cancellation state: `IsCancelled`, `CancelledAt`

**Set:**
- `WaitUntil` → NoWait (immediate execution)
- Triggers worker evaluation

**Preserved:**
- Identity: `Id`, `Type`, `Env`
- Metadata: `CreatedAt`, `ResourceKey`, `ParentRecurrenceID`, `Data`

## Eligibility

**Resettable (terminal states):**
- COMPLETED, FAILED, CANCELLED

**Skipped (no-op):**
- PENDING, WAITING, LEASED, CANCELLATION_PENDING

Reset is **idempotent** - calling on non-terminal tasks has no effect.

## Common Use Cases

### Retry After Bug Fix

```go
// After deploying fix
affectedTaskIDs := []string{"task-1", "task-2", "task-3"}
count, err := manager.ResetTasksByIds(ctx, affectedTaskIDs)
```

### Reset Tasks by Resource Key

```go
// Get terminal tasks for a resource
tasks, _ := manager.GetTasksByResourceKey(ctx, "resource-123")

var terminalIDs []string
for _, task := range tasks {
    if task.DoneAt != "" {
        terminalIDs = append(terminalIDs, task.Id)
    }
}

count, _ := manager.ResetTasksByIds(ctx, terminalIDs)
```

### Reset Exhausted Recurrence Generator

```go
// Resume a finished recurring task
err := manager.ResetTask(ctx, recurrenceTaskID)
// Generator will resume spawning occurrences from now
```

## Environment Isolation

Reset respects environment boundaries set by `ONCE_TASK_ENV`:

```go
// Only resets tasks in current environment
os.Setenv("ONCE_TASK_ENV", "production")
manager.ResetTask(ctx, taskID) // Resets if task.Env == "production"
```

**Important:** Attempting to reset a task from a different environment will return an error:

```go
os.Setenv("ONCE_TASK_ENV", "production")
// If task belongs to "staging" environment
err := manager.ResetTask(ctx, taskID) // Returns error: "task XYZ is in different environment"
```

## Best Practices

**1. Fix root cause first**

```go
// ✅ Fix issue before reset
fixAPICredentials()
manager.ResetTasksByIds(ctx, taskIDs)
```

**2. Use batch operations**

```go
// ✅ Single batch
count, _ := manager.ResetTasksByIds(ctx, taskIDs)

// ❌ Loop over individual resets
for _, id := range taskIDs {
    manager.ResetTask(ctx, id)
}
```

**3. Handle partial failures**

```go
count, err := manager.ResetTasksByIds(ctx, taskIDs)
if err != nil {
    log.Printf("Partial: %d/%d succeeded: %v", count, len(taskIDs), err)
}
```

**4. Consider system load**

```go
// Reset large batches in chunks to avoid overwhelming workers
batches := chunk(taskIDs, 100)
for _, batch := range batches {
    manager.ResetTasksByIds(ctx, batch)
    time.Sleep(time.Second)
}
```

## Next Steps

- [Cancellation](cancellation.md) - Stop task execution
- [Deletion](deletion.md) - Permanently remove tasks
- [Configuration](configuration.md) - Retry policies and concurrency
