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
// Returns:
//   - (result, nil) if fn completes successfully
//   - (nil, error) if fn returns an error
//   - (nil, error) if fn panics (panic converted to error)
func SafeExecute[T any](ctx context.Context, fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			slog.ErrorContext(ctx, "handler panicked", "panic", r, "stack", stack)
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	return fn()
}

// SafeHandler wraps a Handler with panic recovery.
// Returns a new Handler that catches panics and converts them to errors.
func SafeHandler[TaskKind ~string](handler Handler[TaskKind]) Handler[TaskKind] {
	return func(ctx context.Context, task *OnceTask[TaskKind]) (any, error) {
		return SafeExecute(ctx, func() (any, error) {
			return handler(ctx, task)
		})
	}
}

// SafeResourceKeyHandler wraps a ResourceKeyHandler with panic recovery.
// Returns a new ResourceKeyHandler that catches panics and converts them to errors.
func SafeResourceKeyHandler[TaskKind ~string](handler ResourceKeyHandler[TaskKind]) ResourceKeyHandler[TaskKind] {
	return func(ctx context.Context, tasks []OnceTask[TaskKind]) (any, error) {
		return SafeExecute(ctx, func() (any, error) {
			return handler(ctx, tasks)
		})
	}
}
