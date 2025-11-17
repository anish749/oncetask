package oncetask

import (
	"context"
	"log/slog"
)

// contextKey is a private type for context keys to avoid collisions
type contextKey string

const (
	taskIDContextKey contextKey = "oncetask.taskID"
)

// withTaskID adds the task ID to the context for automatic logging
func withTaskID(ctx context.Context, taskID string) context.Context {
	return context.WithValue(ctx, taskIDContextKey, taskID)
}

// taskIDFromContext retrieves the task ID from the context, if present
func taskIDFromContext(ctx context.Context) (string, bool) {
	taskID, ok := ctx.Value(taskIDContextKey).(string)
	return taskID, ok
}

// ContextHandler is a slog.Handler that automatically extracts the task ID from context
// and adds it as an attribute to all log records.
//
// Usage:
//
//	handler := oncetask.NewContextHandler(slog.NewJSONHandler(os.Stdout, nil))
//	slog.SetDefault(slog.New(handler))
type ContextHandler struct {
	handler slog.Handler
}

var _ slog.Handler = (*ContextHandler)(nil)

// NewContextHandler creates a new ContextHandler that wraps another handler
func NewContextHandler(h slog.Handler) *ContextHandler {
	return &ContextHandler{handler: h}
}

// Enabled implements slog.Handler
func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle implements slog.Handler and automatically adds task ID from context
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract task ID from context and add it to the record
	if taskID, ok := taskIDFromContext(ctx); ok {
		r.AddAttrs(slog.String("taskId", taskID))
	}
	return h.handler.Handle(ctx, r)
}

// WithAttrs implements slog.Handler
func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewContextHandler(h.handler.WithAttrs(attrs))
}

// WithGroup implements slog.Handler
func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return NewContextHandler(h.handler.WithGroup(name))
}
