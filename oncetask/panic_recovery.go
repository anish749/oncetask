package oncetask

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
)

// SafeExecute wraps a function execution with panic recovery.
// If the function panics, the panic is recovered and converted to an error.
// The stack trace is logged via slog.ErrorContext for debugging.
//
// Example usage:
//
//	result, err := SafeExecute(ctx, handler, task)
//
// Returns:
//   - (result, nil) if fn completes successfully
//   - (nil, error) if fn returns an error
//   - (nil, error) if fn panics (panic converted to error)
func SafeExecute[P any, R any](ctx context.Context, fn func(context.Context, P) (R, error), p P) (result R, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			slog.ErrorContext(ctx, "handler panicked", "panic", r, "stack", stack)
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	return fn(ctx, p)
}
