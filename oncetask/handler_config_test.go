package oncetask

import (
	"context"
	"testing"
	"time"
)

type testHandlerCfgKind string

func TestWithRetryPolicy(t *testing.T) {
	config := defaultHandlerConfig
	policy := FixedDelayPolicy{MaxAttempts: 7, Delay: 3 * time.Second}
	WithRetryPolicy(policy)(&config)
	if config.RetryPolicy != policy {
		t.Errorf("RetryPolicy: got %v, want %v", config.RetryPolicy, policy)
	}
}

func TestWithNoRetry(t *testing.T) {
	config := defaultHandlerConfig
	WithNoRetry()(&config)
	if _, ok := config.RetryPolicy.(NoRetryPolicy); !ok {
		t.Errorf("RetryPolicy: got %T, want NoRetryPolicy", config.RetryPolicy)
	}
}

func TestWithLeaseDuration(t *testing.T) {
	config := defaultHandlerConfig
	WithLeaseDuration(7 * time.Minute)(&config)
	if config.LeaseDuration != 7*time.Minute {
		t.Errorf("LeaseDuration: got %v, want 7m", config.LeaseDuration)
	}
}

func TestWithConcurrency(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want int
	}{
		{"positive", 5, 5},
		{"one", 1, 1},
		{"zero ignored", 0, 1},
		{"negative ignored", -3, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultHandlerConfig
			WithConcurrency(tt.n)(&config)
			if config.Concurrency != tt.want {
				t.Errorf("Concurrency: got %d, want %d", config.Concurrency, tt.want)
			}
		})
	}
}

func TestWithCancellationHandler(t *testing.T) {
	config := defaultHandlerConfig
	if config.cancellationTaskHandler != nil {
		t.Fatalf("default cancellationTaskHandler should be nil, got %v", config.cancellationTaskHandler)
	}
	handler := func(ctx context.Context, task *OnceTask[testHandlerCfgKind]) (any, error) {
		return nil, nil
	}
	WithCancellationHandler(handler)(&config)
	if config.cancellationTaskHandler == nil {
		t.Errorf("cancellationTaskHandler not set")
	}
}

func TestWithCancellationRetryPolicy(t *testing.T) {
	config := defaultHandlerConfig
	policy := FixedDelayPolicy{MaxAttempts: 4, Delay: 2 * time.Second}
	WithCancellationRetryPolicy(policy)(&config)
	if config.CancellationRetryPolicy != policy {
		t.Errorf("CancellationRetryPolicy: got %v, want %v", config.CancellationRetryPolicy, policy)
	}
}

func TestWithPollInterval(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want time.Duration
	}{
		{"positive", 30 * time.Second, 30 * time.Second},
		{"zero ignored", 0, 1 * time.Minute},
		{"negative ignored", -1 * time.Second, 1 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := defaultHandlerConfig
			WithPollInterval(tt.d)(&config)
			if config.PollInterval != tt.want {
				t.Errorf("PollInterval: got %v, want %v", config.PollInterval, tt.want)
			}
		})
	}
}

func TestDefaultHandlerConfig(t *testing.T) {
	if defaultHandlerConfig.LeaseDuration != 10*time.Minute {
		t.Errorf("LeaseDuration default: got %v, want 10m", defaultHandlerConfig.LeaseDuration)
	}
	if defaultHandlerConfig.Concurrency != 1 {
		t.Errorf("Concurrency default: got %d, want 1", defaultHandlerConfig.Concurrency)
	}
	if defaultHandlerConfig.PollInterval != 1*time.Minute {
		t.Errorf("PollInterval default: got %v, want 1m", defaultHandlerConfig.PollInterval)
	}
	if defaultHandlerConfig.cancellationTaskHandler != nil {
		t.Errorf("cancellationTaskHandler default: got %v, want nil", defaultHandlerConfig.cancellationTaskHandler)
	}
	if _, ok := defaultHandlerConfig.RetryPolicy.(ExponentialBackoffPolicy); !ok {
		t.Errorf("RetryPolicy default: got %T, want ExponentialBackoffPolicy", defaultHandlerConfig.RetryPolicy)
	}
	if _, ok := defaultHandlerConfig.CancellationRetryPolicy.(ExponentialBackoffPolicy); !ok {
		t.Errorf("CancellationRetryPolicy default: got %T, want ExponentialBackoffPolicy", defaultHandlerConfig.CancellationRetryPolicy)
	}
}
