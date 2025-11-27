package oncetask

import (
	"context"
	"testing"
)

func TestGetCancellationHandler(t *testing.T) {
	t.Run("returns no-op handler when not set", func(t *testing.T) {
		config := HandlerConfig{
			cancellationTaskHandler: nil,
		}

		cancellationHandler := getCancellationHandler[string](config)
		result, err := cancellationHandler(context.Background(), &OnceTask[string]{Id: "test-task"})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("no-op handler result = %v, want nil", result)
		}
	})
}
