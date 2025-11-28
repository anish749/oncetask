# oncetask

Lease-based idempotent task execution with Firestore. Ensures tasks complete exactly once with automatic retries on failure, lease-based concurrency control, and recurrence support.

## Features

- **Lease-based execution** - Tasks use leases to prevent concurrent execution; auto-expires on crashes
- **Exactly-once completion** - Tasks are retried on failure until they succeed, ensuring exactly-once successful completion
- **Idempotent execution** - Tasks identified by deterministic IDs, safe to create multiple times
- **Configurable retry policies** - Exponential backoff, fixed delay, or no retry
- **Resource key serialization** - Tasks with the same resource key execute one at a time
- **Grouped execution** - Process all pending tasks for a resource key together in a single handler call
- **Recurring tasks** - RFC 5545 RRULE support for scheduled recurring execution
- **Environment isolation** - Logical separation via `ONCE_TASK_ENV`

## Installation

```bash
go get github.com/anish749/oncetask
```

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

## Documentation

- **[Quick Start Guide](docs/quick-start.md)** - Get started in minutes
- **[Task Types](docs/task-types.md)** - Single task vs resource key handlers
- **[Task Data Interface](docs/task-data.md)** - Structuring your task data
- **[Configuration](docs/configuration.md)** - Retry policies, concurrency, and leasing
- **[Recurrence](docs/recurrence.md)** - Scheduled recurring tasks
- **[Cancellation](docs/cancellation.md)** - Task cancellation and cleanup

## License

See [LICENSE](LICENSE) file for details.
