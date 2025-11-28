package oncetask

import "time"

// RetryPolicy defines retry behavior for task execution failures.
// Users can implement this interface for custom retry logic.
type RetryPolicy interface {
	// ShouldRetry returns true if the task should be retried after failure.
	// attempts is the current attempt count (1 = first attempt, 2 = first retry, etc.)
	ShouldRetry(attempts int, err error) bool

	// NextRetryDelay returns the duration to wait before the next retry.
	// attempts is the current attempt count.
	// err is the error returned by the handler, which can be used to determine the delay.
	NextRetryDelay(attempts int, err error) time.Duration
}

// ExponentialBackoffPolicy retries with exponential backoff.
// Delay = BaseDelay * (Multiplier ^ (attempts - 1)), capped at MaxDelay.
type ExponentialBackoffPolicy struct {
	MaxAttempts int           // Maximum attempts (0 = unlimited)
	BaseDelay   time.Duration // Initial delay (default: 1 second)
	MaxDelay    time.Duration // Maximum delay cap (default: 5 minutes)
	Multiplier  float64       // Multiplier per attempt (default: 2.0)
}

// ShouldRetry returns true if the task should be retried based on attempt count.
func (p ExponentialBackoffPolicy) ShouldRetry(attempts int, err error) bool {
	if p.MaxAttempts == 0 {
		return true // Unlimited retries
	}
	return attempts < p.MaxAttempts
}

// NextRetryDelay calculates the exponential backoff delay for the next retry.
func (p ExponentialBackoffPolicy) NextRetryDelay(attempts int, err error) time.Duration {
	baseDelay := p.BaseDelay
	if baseDelay == 0 {
		baseDelay = 1 * time.Second
	}

	maxDelay := p.MaxDelay
	if maxDelay == 0 {
		maxDelay = 5 * time.Minute
	}

	multiplier := p.Multiplier
	if multiplier == 0 {
		multiplier = 2.0
	}

	// Calculate delay: baseDelay * (multiplier ^ (attempts - 1))
	delay := float64(baseDelay)
	for i := 1; i < attempts; i++ {
		delay *= multiplier
		if time.Duration(delay) > maxDelay {
			return maxDelay
		}
	}

	return time.Duration(delay)
}

// FixedDelayPolicy retries with a constant delay between attempts.
type FixedDelayPolicy struct {
	MaxAttempts int           // Maximum attempts (0 = unlimited)
	Delay       time.Duration // Delay between retries
}

// ShouldRetry returns true if the task should be retried based on attempt count.
func (p FixedDelayPolicy) ShouldRetry(attempts int, err error) bool {
	if p.MaxAttempts == 0 {
		return true
	}
	return attempts < p.MaxAttempts
}

// NextRetryDelay returns the fixed delay for the next retry.
func (p FixedDelayPolicy) NextRetryDelay(attempts int, err error) time.Duration {
	return p.Delay
}

// NoRetryPolicy never retries - tasks fail permanently on first error.
type NoRetryPolicy struct{}

// ShouldRetry always returns false - no retries are performed.
func (p NoRetryPolicy) ShouldRetry(attempts int, err error) bool {
	return false
}

// NextRetryDelay always returns 0 - no retry delay is needed.
func (p NoRetryPolicy) NextRetryDelay(attempts int, err error) time.Duration {
	return 0
}
