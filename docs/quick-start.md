# Quick Start

This guide will help you get started with oncetask in just a few minutes.

## Installation

```bash
go get github.com/anish749/oncetask
```

## Basic Usage

### Initialize the Manager

```go
import "oncetask"

// Initialize with your Firestore client
manager, cancel := oncetask.NewFirestoreOnceTaskManager[MyTaskKind](firestoreClient)
defer cancel()
```

### Register a Task Handler

```go
manager.RegisterTaskHandler(TaskKindSendEmail, func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data SendEmailData
    task.ReadInto(&data)
    return sendEmail(data)
})
```

### Create a Task

```go
manager.CreateTask(ctx, SendEmailData{To: "user@example.com", Subject: "Hello"})
```

That's it! The task will be automatically processed by your handler with built-in retries, leasing, and idempotency guarantees.

## Next Steps

- Learn about [Task Types](task-types.md) - single task handlers vs resource key handlers
- Understand the [Task Data Interface](task-data.md) - how to structure your task data
- Explore [Configuration Options](configuration.md) - customize retry policies, concurrency, and more
- Set up [Recurring Tasks](recurrence.md) - schedule tasks to run periodically
- Handle [Task Cancellation](cancellation.md) - cancel tasks and clean up resources
