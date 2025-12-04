# Deletion

oncetask provides hard delete functionality to permanently remove tasks from Firestore. Unlike [cancellation](cancellation.md) which marks tasks as cancelled but keeps them in the database, deletion permanently removes task documents.

## Deletion Methods

### Delete a Single Task

```go
err := manager.DeleteTask(ctx, taskID)
```

Permanently removes a specific task by its ID.

### Delete Multiple Tasks by IDs

```go
count, err := manager.DeleteTasksByIds(ctx, []string{"id1", "id2", "id3"})
```

Permanently removes multiple tasks by their IDs in a single operation. Returns the number of tasks deleted.

## Deletion Behavior

When a task is deleted:

1. **Document is removed from Firestore** - The task is permanently deleted with no way to recover it
2. **No cleanup handlers run** - Unlike cancellation, deletion does not trigger any handlers
3. **No audit trail** - The task is gone with no record of its existence

## Deletion vs Cancellation

| Aspect | Deletion | Cancellation |
|--------|----------|--------------|
| Data retention | Permanently removed | Kept with `isCancelled=true` |
| Cleanup handlers | Not triggered | Triggered if registered |
| Audit trail | None | `cancelledAt` timestamp preserved |
| Recoverable | No | Task data still available |
| Use case | Cleanup old/test data | Graceful task termination |

**When to use deletion:**
- Cleaning up old completed/failed tasks
- Removing test data
- Purging tasks after data retention period
- Removing erroneously created tasks

**When to use cancellation:**
- Stopping a task that's no longer needed
- Graceful shutdown with cleanup
- When you need audit trail of what was cancelled

## Idempotency

Deletion operations are idempotent:

- Deleting a non-existent task returns no error
- Deleting a task from a different environment is silently skipped
- Re-deleting an already deleted task succeeds

```go
// Both calls succeed, second one is a no-op
manager.DeleteTask(ctx, "task-123")
manager.DeleteTask(ctx, "task-123") // No error
```

## Environment Isolation

Deletion respects environment boundaries set by `ONCE_TASK_ENV`:

```go
// Only deletes tasks in current environment
os.Setenv("ONCE_TASK_ENV", "production")
manager.DeleteTask(ctx, taskID) // Only deletes if task.Env == "production"
```

Tasks from other environments are silently skipped.

## Deleting Running Tasks

You can delete tasks that are currently being executed (leased):

```go
// This works, but has consequences
manager.DeleteTask(ctx, leasedTaskID)
```

**Warning:** If you delete a leased task:
- The handler continues running until completion
- When the handler tries to update the task status, it will fail (document not found)
- The handler's side effects (external API calls, etc.) still occur

**Recommendation:** Cancel tasks instead of deleting them if they might be running.

## Partial Failures

Bulk deletion can have partial failures:

```go
count, err := manager.DeleteTasksByIds(ctx, []string{"id1", "id2", "id3"})
if err != nil {
    // Some deletions may have succeeded
    log.Printf("Deleted %d tasks, but encountered errors: %v", count, err)
}
```

The returned count indicates how many tasks were successfully deleted, even if some failed.

## Examples

### Cleanup Old Completed Tasks

```go
// Get tasks by resource key, filter completed ones, delete
tasks, err := manager.GetTasksByResourceKey(ctx, "user-123")
if err != nil {
    return err
}

var oldTaskIDs []string
for _, task := range tasks {
    if task.GetStatus() == oncetask.TaskStatusCompleted {
        oldTaskIDs = append(oldTaskIDs, task.Id)
    }
}

count, err := manager.DeleteTasksByIds(ctx, oldTaskIDs)
log.Printf("Deleted %d old tasks", count)
```

### Delete Test Data

```go
// After integration tests, clean up test tasks
testTaskIDs := []string{
    "test-task-1",
    "test-task-2",
    "test-task-3",
}

count, err := manager.DeleteTasksByIds(ctx, testTaskIDs)
if err != nil {
    log.Printf("Warning: test cleanup had errors: %v", err)
}
```

### Delete Single Erroneously Created Task

```go
// Oops, created a task with wrong data
err := manager.DeleteTask(ctx, "wrong-task-id")
if err != nil {
    log.Printf("Failed to delete task: %v", err)
}
```

## Best Practices

### 1. Prefer Cancellation for Active Tasks

If a task might be running or needs cleanup:

```go
// Better: cancel first, then delete later if needed
manager.CancelTask(ctx, taskID)

// Later, after task is done-cancelled:
manager.DeleteTask(ctx, taskID)
```

### 2. Batch Deletions for Efficiency

Use `DeleteTasksByIds` for multiple tasks:

```go
// Efficient: single bulk operation
manager.DeleteTasksByIds(ctx, taskIDs)

// Inefficient: multiple round trips
for _, id := range taskIDs {
    manager.DeleteTask(ctx, id)
}
```

### 3. Handle Partial Failures

Always check both count and error:

```go
count, err := manager.DeleteTasksByIds(ctx, taskIDs)
if err != nil {
    log.Printf("Partial deletion: %d/%d succeeded, error: %v",
        count, len(taskIDs), err)
}
```

### 4. Consider Data Retention Requirements

Before deleting, ensure you don't need the task data for:
- Audit logs
- Debugging
- Analytics
- Compliance requirements

## Next Steps

- Learn about [Cancellation](cancellation.md) for graceful task termination
- Explore [Configuration](configuration.md) for retry policies
- Understand [Task Types](task-types.md) for different execution patterns
