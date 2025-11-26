package oncetask

import "time"

// HandlerConfig holds configuration options for a task handler.
type HandlerConfig struct {
	RetryPolicy   RetryPolicy   // Retry policy for failed tasks
	LeaseDuration time.Duration // Duration for which a task is leased during execution
	Concurrency   int           // Number of concurrent workers processing tasks
}

// DefaultHandlerConfig provides sensible defaults for all handlers.
var DefaultHandlerConfig = HandlerConfig{
	RetryPolicy: ExponentialBackoffPolicy{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Second,
		MaxDelay:    5 * time.Minute,
		Multiplier:  2.0,
	},
	LeaseDuration: 10 * time.Minute,
	Concurrency:   1,
}

// HandlerOption is a functional option for configuring handlers.
type HandlerOption func(*HandlerConfig)

// WithRetryPolicy sets a custom retry policy for the handler.
func WithRetryPolicy(policy RetryPolicy) HandlerOption {
	return func(c *HandlerConfig) {
		c.RetryPolicy = policy
	}
}

// WithNoRetry disables retries - tasks fail permanently on first error.
func WithNoRetry() HandlerOption {
	return func(c *HandlerConfig) {
		c.RetryPolicy = NoRetryPolicy{}
	}
}

// WithLeaseDuration sets the lease duration for task execution.
func WithLeaseDuration(d time.Duration) HandlerOption {
	return func(c *HandlerConfig) {
		c.LeaseDuration = d
	}
}

// WithConcurrency sets the number of concurrent workers processing tasks.
// Values less than 1 are ignored; default is 1 (serial execution).
func WithConcurrency(n int) HandlerOption {
	return func(c *HandlerConfig) {
		if n > 0 {
			c.Concurrency = n
		}
	}
}
