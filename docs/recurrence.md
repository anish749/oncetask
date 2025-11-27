# Recurrence

oncetask supports recurring tasks using RFC 5545 RRULE syntax, allowing you to schedule tasks that repeat on a regular basis.

## How Recurrence Works

Recurring tasks use a generator pattern:

1. **Recurrence Task (Generator)**: The main task that defines the recurrence schedule
2. **Occurrence Tasks**: Individual task instances spawned for each recurrence

The recurrence task itself doesn't execute your handler - it spawns occurrence tasks that do.

## Implementing Recurring Tasks

### Define the Recurrence

Implement the `RecurrenceProvider` interface on your task data:

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

### Register a Handler

Register your handler normally - it will process each occurrence:

```go
manager.RegisterTaskHandler(TaskKindWeeklyReport, func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data WeeklyReportData
    task.ReadInto(&data)
    return generateReport(data.UserID)
})
```

### Create the Recurring Task

```go
manager.CreateTask(ctx, WeeklyReportData{UserID: "user123"})
```

This creates:
- One recurrence task (the generator)
- Occurrence tasks will be automatically spawned based on the schedule

## RRULE Syntax

oncetask uses RFC 5545 RRULE syntax. Here are common patterns:

### Daily

```go
RRule: "FREQ=DAILY"
// Every day

RRule: "FREQ=DAILY;INTERVAL=2"
// Every 2 days

RRule: "FREQ=DAILY;BYHOUR=9;BYMINUTE=0"
// Every day at 9:00 AM
```

### Weekly

```go
RRule: "FREQ=WEEKLY;BYDAY=MO"
// Every Monday

RRule: "FREQ=WEEKLY;BYDAY=MO,WE,FR"
// Every Monday, Wednesday, and Friday

RRule: "FREQ=WEEKLY;INTERVAL=2;BYDAY=TU"
// Every other Tuesday
```

### Monthly

```go
RRule: "FREQ=MONTHLY;BYMONTHDAY=1"
// First day of every month

RRule: "FREQ=MONTHLY;BYDAY=1MO"
// First Monday of every month

RRule: "FREQ=MONTHLY;BYDAY=-1FR"
// Last Friday of every month
```

### Yearly

```go
RRule: "FREQ=YEARLY;BYMONTH=1;BYMONTHDAY=1"
// January 1st every year

RRule: "FREQ=YEARLY;BYMONTH=12;BYDAY=-1MO"
// Last Monday of December every year
```

### With End Date

```go
RRule: "FREQ=DAILY;UNTIL=20251231T235959Z"
// Daily until December 31, 2025

RRule: "FREQ=WEEKLY;COUNT=10"
// Weekly for 10 occurrences
```

## DTStart Parameter

The `DTStart` field specifies when the recurrence starts:

```go
&oncetask.Recurrence{
    RRule:   "FREQ=WEEKLY;BYDAY=MO",
    DTStart: "2025-01-01T09:00:00Z",  // RFC3339 format
}
```

**Important:** Use RFC3339 format for `DTStart`.

## Recurrence Task Lifecycle

```
┌─────────────────────┐
│ Recurrence Task     │ (Generator - never executes handler)
│ (State: Running)    │
└──────────┬──────────┘
           │
           ├─> Occurrence 1 (Executes handler)
           ├─> Occurrence 2 (Executes handler)
           ├─> Occurrence 3 (Executes handler)
           └─> ...
```

- The recurrence task remains in "running" state
- Occurrence tasks are spawned automatically
- Each occurrence is a normal task with its own lifecycle

## Cancelling Recurring Tasks

### Cancel the Recurrence (Generator)

```go
manager.CancelTask(ctx, recurrenceTaskID)
```

**Result:**
- The recurrence task is marked as done
- No new occurrence tasks are spawned
- Already-spawned occurrences continue normally

### Cancel Individual Occurrences

Each occurrence is a separate task and can be cancelled independently:

```go
manager.CancelTask(ctx, occurrenceTaskID)
```

See [Cancellation documentation](cancellation.md) for more details.

## Complete Example

```go
// Define recurring task data
type DailyBackupData struct {
    DatabaseID string
    StartTime  time.Time
}

// Required: OnceTaskData
func (d DailyBackupData) GetType() MyTaskKind {
    return TaskKindDailyBackup
}

func (d DailyBackupData) GenerateIdempotentID() string {
    return fmt.Sprintf("backup:%s:%d", d.DatabaseID, d.StartTime.Unix())
}

// Optional: RecurrenceProvider
func (d DailyBackupData) GetRecurrence() *oncetask.Recurrence {
    return &oncetask.Recurrence{
        RRule:   "FREQ=DAILY;BYHOUR=2;BYMINUTE=0", // 2 AM daily
        DTStart: d.StartTime.Format(time.RFC3339),
    }
}

// Register handler
manager.RegisterTaskHandler(TaskKindDailyBackup, func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data DailyBackupData
    task.ReadInto(&data)
    return performBackup(ctx, data.DatabaseID)
})

// Create recurring backup task
manager.CreateTask(ctx, DailyBackupData{
    DatabaseID: "prod-db-1",
    StartTime:  time.Now(),
})
```

## Best Practices

1. **Idempotent IDs**: Ensure occurrence tasks have unique IDs
   - Include timestamp or occurrence number in the ID
   - Don't use the same ID for all occurrences

2. **Time Zones**: Use UTC for `DTStart` to avoid timezone issues
   ```go
   DTStart: time.Now().UTC().Format(time.RFC3339)
   ```

3. **End Conditions**: Consider adding `UNTIL` or `COUNT` to prevent infinite recurrence
   ```go
   RRule: "FREQ=DAILY;COUNT=30"  // Run for 30 days then stop
   ```

4. **Monitoring**: Track recurrence task status to ensure it's generating occurrences

5. **Cancellation**: Always cancel recurrence tasks when they're no longer needed

## Troubleshooting

**Occurrences not spawning:**
- Check that the recurrence task is not marked as done
- Verify RRULE syntax is valid
- Ensure DTStart is in RFC3339 format
- Check that the next occurrence time hasn't passed

**Duplicate occurrences:**
- Verify `GenerateIdempotentID()` includes unique data per occurrence
- Each occurrence should have a distinct idempotent ID

## RRULE Resources

For more complex RRULE patterns, see:
- [RFC 5545 Specification](https://tools.ietf.org/html/rfc5545)
- [RRULE Tool](https://icalendar.org/rrule-tool.html) - Interactive RRULE builder

## Next Steps

- Learn about [Cancellation](cancellation.md) to manage task lifecycle
- Explore [Configuration](configuration.md) for retry policies and concurrency
- Understand [Task Data](task-data.md) interface requirements
