package oncetask

import "time"

// handlerConfig holds configuration options for a task handler.
type handlerConfig struct {
	// cancellationTaskHandler is an optional cleanup handler for cancelled tasks.
	// Type: Handler[TaskKind].
	// Always processes tasks one at a time, regardless of whether the normal handler is single-task or resource-key.
	// Use WithCancellationHandler() to set this field - it should not be set directly.
	cancellationTaskHandler any
	RetryPolicy             RetryPolicy   // Retry policy for normal task execution
	CancellationRetryPolicy RetryPolicy   // Retry policy for cancellation handlers (separate)
	LeaseDuration           time.Duration // Duration for which a task is leased during execution
	Concurrency             int           // Number of concurrent workers processing tasks
}

// defaultHandlerConfig provides sensible defaults for all handlers.
var defaultHandlerConfig = handlerConfig{
	RetryPolicy: ExponentialBackoffPolicy{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    5 * time.Minute,
		Multiplier:  2.0,
	},
	CancellationRetryPolicy: ExponentialBackoffPolicy{
		MaxAttempts: 5,
		BaseDelay:   1 * time.Second,
		MaxDelay:    10 * time.Minute,
		Multiplier:  2.0,
	},
	LeaseDuration:           10 * time.Minute,
	Concurrency:             1,
	cancellationTaskHandler: nil, // Optional
}

// HandlerOption is a functional option for configuring handlers.
type HandlerOption func(*handlerConfig)

// WithRetryPolicy sets a custom retry policy for the handler.
func WithRetryPolicy(policy RetryPolicy) HandlerOption {
	return func(c *handlerConfig) {
		c.RetryPolicy = policy
	}
}

// WithNoRetry disables retries - tasks fail permanently on first error.
func WithNoRetry() HandlerOption {
	return func(c *handlerConfig) {
		c.RetryPolicy = NoRetryPolicy{}
	}
}

// WithLeaseDuration sets the lease duration for task execution.
func WithLeaseDuration(d time.Duration) HandlerOption {
	return func(c *handlerConfig) {
		c.LeaseDuration = d
	}
}

// WithConcurrency sets the number of concurrent workers processing tasks.
// Values less than 1 are ignored; default is 1 (serial execution).
func WithConcurrency(n int) HandlerOption {
	return func(c *handlerConfig) {
		if n > 0 {
			c.Concurrency = n
		}
	}
}

// WithCancellationHandler sets a cleanup handler for cancelled tasks.
// The handler uses the same signature as OnceTaskHandler[TaskKind].
// Cancelled tasks are always processed one at a time, even if the normal handler is a resource-key handler.
// When a task is cancelled:
//   - If handler is set: Handler is invoked to perform cleanup, retried per CancellationRetryPolicy.
//   - If handler is nil: Task is immediately marked as done-cancelled without executing any handler.
//
// Example:
//
//	oncetask.WithCancellationHandler(func(ctx context.Context, task *oncetask.OnceTask[TaskKind]) (any, error) {
//	    // Perform cleanup operations
//	    return nil, nil
//	})
func WithCancellationHandler[TaskKind ~string](handler Handler[TaskKind]) HandlerOption {
	return func(c *handlerConfig) {
		c.cancellationTaskHandler = handler
	}
}

// WithCancellationRetryPolicy sets the retry policy for cancellation handlers.
func WithCancellationRetryPolicy(policy RetryPolicy) HandlerOption {
	return func(c *handlerConfig) {
		c.CancellationRetryPolicy = policy
	}
}
