# Task Types

oncetask supports two types of task handlers, each designed for different execution patterns.

## Single Task Handler

Each task executes independently in its own handler call. This is the default and most common pattern.

### When to Use

Use single task handlers when:
- Each task represents a discrete unit of work
- Tasks can be processed independently
- You need fine-grained control over individual task execution

### Example

```go
manager.RegisterTaskHandler(TaskKindSendEmail, func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data SendEmailData
    task.ReadInto(&data)
    return sendEmail(data)
})
```

### Resource Key Serialization

Even with single task handlers, you can serialize execution per resource using the `ResourceKey` field. Tasks with the same resource key will execute one at a time, preventing concurrent modifications to the same resource.

```go
type SyncData struct {
    UserID string
}

func (d SyncData) GetResourceKey() string {
    return fmt.Sprintf("user:%s", d.UserID)
}
```

## Resource Key Handler

All pending tasks with the same resource key are grouped together and passed to the handler in a single batch.

### When to Use

Use resource key handlers when:
- You need to process multiple related tasks together
- Batch processing is more efficient than individual processing
- You want to apply atomic operations across multiple tasks
- You need to deduplicate or consolidate work

### Example

```go
manager.RegisterResourceKeyHandler(TaskKindSync, func(ctx context.Context, tasks []oncetask.OnceTask[MyTaskKind]) (any, error) {
    // All tasks for this resource key are processed together
    var allData []SyncData
    for _, task := range tasks {
        var data SyncData
        task.ReadInto(&data)
        allData = append(allData, data)
    }

    // Process all tasks in a single batch operation
    return nil, processBatch(allData)
})
```

### Behavior

- All pending tasks with the same resource key are collected
- The handler receives a slice of all tasks
- If the handler succeeds, all tasks are marked as completed
- If the handler fails, all tasks retry according to the retry policy
- Tasks are still identified by unique idempotent IDs

## Choosing Between Handler Types

| Scenario | Handler Type |
|----------|-------------|
| Send individual emails | Single Task Handler |
| Process payment transactions | Single Task Handler |
| Sync user data (batch API) | Resource Key Handler |
| Aggregate analytics events | Resource Key Handler |
| Independent operations | Single Task Handler |
| Related operations that benefit from batching | Resource Key Handler |

## Next Steps

- Learn about the [Task Data Interface](task-data.md) to structure your task data
- Explore [Configuration Options](configuration.md) for retries and concurrency
