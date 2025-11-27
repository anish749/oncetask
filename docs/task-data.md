# Task Data Interface

Task data in oncetask is flexible and type-safe. You define your own data structures and implement the required interfaces.

## Required Interface: OnceTaskData

Every task data type must implement the `OnceTaskData` interface:

```go
type OnceTaskData interface {
    GetType() TaskKind
    GenerateIdempotentID() string
}
```

### Example

```go
type SendEmailData struct {
    To      string
    Subject string
    Body    string
}

func (d SendEmailData) GetType() MyTaskKind {
    return TaskKindSendEmail
}

func (d SendEmailData) GenerateIdempotentID() string {
    // Generate a deterministic ID based on task data
    // Same input data should always produce the same ID
    return fmt.Sprintf("email:%s:%s", d.To, hashContent(d.Subject))
}
```

### Idempotent ID Guidelines

The `GenerateIdempotentID()` method should:
- Return a deterministic ID based on task data
- Produce the same ID for identical task data
- Be unique enough to distinguish different tasks
- Consider what makes a task truly unique for your use case

## Optional Interfaces

### ResourceKeyProvider

Implement this interface to enable resource-level serialization:

```go
type ResourceKeyProvider interface {
    GetResourceKey() string
}
```

**Example:**

```go
type SyncUserData struct {
    UserID string
    Data   map[string]interface{}
}

func (d SyncUserData) GetResourceKey() string {
    return fmt.Sprintf("user:%s", d.UserID)
}
```

Tasks with the same resource key will execute one at a time, preventing concurrent operations on the same resource.

### ScheduledTask

Implement this interface to delay task execution until a specific time:

```go
type ScheduledTask interface {
    GetScheduledTime() time.Time
}
```

**Example:**

```go
type SendReminderData struct {
    UserID     string
    Message    string
    RemindAt   time.Time
}

func (d SendReminderData) GetScheduledTime() time.Time {
    return d.RemindAt
}
```

The task will not execute until the scheduled time has passed.

### RecurrenceProvider

Implement this interface to define recurring task schedules:

```go
type RecurrenceProvider interface {
    GetRecurrence() *Recurrence
}
```

**Example:**

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

See [Recurrence documentation](recurrence.md) for more details.

## Complete Example

Here's a complete example combining multiple interfaces:

```go
type ScheduledSyncData struct {
    UserID      string
    SyncType    string
    ScheduledAt time.Time
}

// Required: OnceTaskData
func (d ScheduledSyncData) GetType() MyTaskKind {
    return TaskKindSync
}

func (d ScheduledSyncData) GenerateIdempotentID() string {
    return fmt.Sprintf("sync:%s:%s:%d", d.UserID, d.SyncType, d.ScheduledAt.Unix())
}

// Optional: ResourceKeyProvider
func (d ScheduledSyncData) GetResourceKey() string {
    return fmt.Sprintf("user:%s", d.UserID)
}

// Optional: ScheduledTask
func (d ScheduledSyncData) GetScheduledTime() time.Time {
    return d.ScheduledAt
}
```

## Best Practices

1. **Idempotent IDs**: Make them deterministic but unique enough for your use case
2. **Resource Keys**: Use them to prevent concurrent operations on the same resource
3. **Serialization**: Task data is JSON-serialized, so use JSON-compatible types
4. **Type Safety**: Use strongly-typed data structures instead of `map[string]interface{}`
5. **Validation**: Validate data before creating tasks to catch errors early

## Next Steps

- Learn about [Configuration Options](configuration.md) for retries and concurrency
- Explore [Recurrence](recurrence.md) for scheduled recurring tasks
- Understand [Task Types](task-types.md) for different execution patterns
