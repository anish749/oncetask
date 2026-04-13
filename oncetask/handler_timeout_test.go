package oncetask

import (
	"context"
	"testing"
	"time"
)

func TestHandlerTimeout_SlowHandlerGetsCancelled(t *testing.T) {
	// Simulate the timeout runLoop applies: leaseDuration - 1s
	leaseDuration := 100 * time.Millisecond
	handlerTimeout := leaseDuration - 10*time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	slowHandler := func(ctx context.Context, task *OnceTask[string]) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return "should not reach", nil
		}
	}

	task := OnceTask[string]{Id: "test-timeout"}
	_, err := SafeExecute(ctx, slowHandler, &task)
	if !isContextDeadlineExceeded(err) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestHandlerTimeout_FastHandlerCompletesNormally(t *testing.T) {
	leaseDuration := 5 * time.Second
	handlerTimeout := leaseDuration - 1*time.Second

	ctx, cancel := context.WithTimeout(context.Background(), handlerTimeout)
	defer cancel()

	fastHandler := func(ctx context.Context, task *OnceTask[string]) (any, error) {
		return "done", nil
	}

	task := OnceTask[string]{Id: "test-fast"}
	result, err := SafeExecute(ctx, fastHandler, &task)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "done" {
		t.Errorf("expected result 'done', got %v", result)
	}
}

func isContextDeadlineExceeded(err error) bool {
	return err == context.DeadlineExceeded
}
