package oncetask

import (
	"context"
	"fmt"
	"runtime/debug"
)

// PanicError represents an error that occurred due to a panic in a handler.
// It captures the panic value and stack trace for debugging.
type PanicError struct {
	// Value is the value that was passed to panic()
	Value any
	// Stack is the stack trace at the point of panic
	Stack string
}

// Error implements the error interface.
func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// FullError returns the error message with the full stack trace.
func (e *PanicError) FullError() string {
	return fmt.Sprintf("panic: %v\n\nstack trace:\n%s", e.Value, e.Stack)
}

// IsPanicError checks if an error is a PanicError.
func IsPanicError(err error) bool {
	_, ok := err.(*PanicError)
	return ok
}

// AsPanicError returns the PanicError if err is one, otherwise nil.
func AsPanicError(err error) *PanicError {
	if pe, ok := err.(*PanicError); ok {
		return pe
	}
	return nil
}

// SafeExecute wraps a function execution with panic recovery.
// If the function panics, the panic is recovered and converted to a PanicError.
// This allows the caller to handle panics as regular errors.
//
// Returns:
//   - (result, nil) if fn completes successfully
//   - (nil, error) if fn returns an error
//   - (nil, *PanicError) if fn panics
func SafeExecute[T any](fn func() (T, error)) (result T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{
				Value: r,
				Stack: string(debug.Stack()),
			}
		}
	}()

	return fn()
}

// SafeHandler wraps a Handler with panic recovery.
// Returns a new Handler that catches panics and converts them to errors.
func SafeHandler[TaskKind ~string](handler Handler[TaskKind]) Handler[TaskKind] {
	return func(ctx context.Context, task *OnceTask[TaskKind]) (any, error) {
		return SafeExecute(func() (any, error) {
			return handler(ctx, task)
		})
	}
}

// SafeResourceKeyHandler wraps a ResourceKeyHandler with panic recovery.
// Returns a new ResourceKeyHandler that catches panics and converts them to errors.
func SafeResourceKeyHandler[TaskKind ~string](handler ResourceKeyHandler[TaskKind]) ResourceKeyHandler[TaskKind] {
	return func(ctx context.Context, tasks []OnceTask[TaskKind]) (any, error) {
		return SafeExecute(func() (any, error) {
			return handler(ctx, tasks)
		})
	}
}
