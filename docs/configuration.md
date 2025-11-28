# Configuration

oncetask provides flexible configuration options to customize task execution behavior.

## Handler Configuration Options

All configuration options are applied when registering a task handler:

```go
manager.RegisterTaskHandler(taskKind, handler,
    oncetask.WithRetryPolicy(policy),
    oncetask.WithLeaseDuration(duration),
    oncetask.WithConcurrency(n),
)
```

## Retry Policies

Control how tasks are retried when they fail.

### Exponential Backoff (Default)

Retries with exponentially increasing delays:

```go
oncetask.WithRetryPolicy(oncetask.ExponentialBackoffPolicy{
    MaxAttempts: 5,
    BaseDelay:   time.Second,
    MaxDelay:    time.Minute,
})
```

**Parameters:**
- `MaxAttempts`: Maximum number of retry attempts
- `BaseDelay`: Initial delay before the first retry
- `MaxDelay`: Maximum delay between retries (backoff is capped at this value)

**Delay calculation:**
```
delay = min(BaseDelay * 2^(attempt-1), MaxDelay)
```

### Fixed Delay

Retries with a constant delay between attempts:

```go
oncetask.WithRetryPolicy(oncetask.FixedDelayPolicy{
    MaxAttempts: 3,
    Delay:       5 * time.Second,
})
```

**Parameters:**
- `MaxAttempts`: Maximum number of retry attempts
- `Delay`: Fixed delay between each retry

### No Retry

Disable retries completely - tasks fail permanently on the first error:

```go
oncetask.WithNoRetry()
```

**Use cases:**
- Critical tasks that should not be retried
- Tasks with side effects that cannot be safely retried
- Operations where immediate failure feedback is required

## Lease Duration

Controls how long a task is leased during execution. This acts as a timeout for task execution:

```go
oncetask.WithLeaseDuration(15 * time.Minute)
```

**Default:** 10 minutes

**Purpose:**
- Acts as a timeout for task execution
- Prevents other workers from processing the same task during the lease period
- Automatically expires if the worker crashes or the task exceeds the lease duration
- Should be longer than your expected task execution time

**Behavior:**
- If a task exceeds the lease duration, the lease expires and the task may be picked up by another worker
- This means tasks running longer than the lease duration can be interrupted and retried

**Recommendations:**
- Set it to 2-3x your typical task execution time to account for variability
- For long-running tasks, increase accordingly to prevent premature timeouts
- For quick tasks, you can reduce it to minimize recovery time after crashes

## Concurrency

Controls the number of concurrent workers processing tasks:

```go
oncetask.WithConcurrency(3)
```

**Default:** 1

**Behavior:**
- Each worker can process one task at a time
- Multiple workers run in parallel goroutines
- Respects resource key serialization (tasks with the same resource key still execute one at a time)

**Recommendations:**
- Increase for I/O-bound tasks (network calls, database operations)
- Keep low for CPU-intensive tasks
- Consider your downstream service rate limits

## Cancellation Handler

Register a cleanup handler for cancelled tasks:

```go
oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
    var data MyTaskData
    task.ReadInto(&data)
    return nil, cleanupResources(data)
})
```

**Use cases:**
- Release acquired resources
- Clean up partial work
- Notify external systems
- Rollback operations

See [Cancellation documentation](cancellation.md) for more details.

## Cancellation Retry Policy

Configure retry behavior specifically for cancellation handlers:

```go
oncetask.WithCancellationRetryPolicy(oncetask.ExponentialBackoffPolicy{
    MaxAttempts: 5,
    BaseDelay:   time.Second,
    MaxDelay:    10 * time.Minute,
})
```

**Note:** This is separate from the main task retry policy.

## Complete Example

```go
manager.RegisterTaskHandler(
    TaskKindProcessPayment,
    func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data PaymentData
        task.ReadInto(&data)
        return processPayment(ctx, data)
    },
    // Retry up to 5 times with exponential backoff
    oncetask.WithRetryPolicy(oncetask.ExponentialBackoffPolicy{
        MaxAttempts: 5,
        BaseDelay:   2 * time.Second,
        MaxDelay:    5 * time.Minute,
    }),
    // Lease for 5 minutes (payments should complete quickly)
    oncetask.WithLeaseDuration(5 * time.Minute),
    // Process 3 payments concurrently
    oncetask.WithConcurrency(3),
    // Handle cancellations (refund if needed)
    oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[MyTaskKind]) (any, error) {
        var data PaymentData
        task.ReadInto(&data)
        return refundPayment(ctx, data)
    }),
    // Retry cancellation handler with different policy
    oncetask.WithCancellationRetryPolicy(oncetask.ExponentialBackoffPolicy{
        MaxAttempts: 10,
        BaseDelay:   time.Second,
        MaxDelay:    1 * time.Minute,
    }),
)
```

## Configuration Best Practices

1. **Retry Policies**:
   - Use exponential backoff for transient failures
   - Use fixed delay for rate-limited APIs
   - Use no retry for non-idempotent operations

2. **Lease Duration**:
   - Set to 2-3x typical execution time to avoid timeouts
   - Account for retries within the lease period
   - Balance recovery speed vs unnecessary retries
   - Remember it acts as a timeout - tasks exceeding it will be interrupted

3. **Concurrency**:
   - Start with 1 and increase based on monitoring
   - Consider downstream system capacity
   - Monitor resource usage (CPU, memory, connections)

4. **Cancellation**:
   - Always implement cancellation handlers for resource-acquiring tasks
   - Make cancellation handlers idempotent
   - Use aggressive retry policies for cancellation handlers

## Next Steps

- Learn about [Recurrence](recurrence.md) for scheduled recurring tasks
- Understand [Cancellation](cancellation.md) in detail
- Explore [Task Types](task-types.md) for different execution patterns
