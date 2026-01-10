package oncetask

import (
	"context"
	"log/slog"
)

// ContextHandler is a slog.Handler that automatically extracts the task ID and resource key from context
// and adds them as attributes to all log records.
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

// Handle implements slog.Handler and automatically adds task ID and resource key from context
//
//nolint:gocritic // slog.Handler interface requires Record by value
func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	// Extract task ID from context and add it to the record
	if taskID, ok := ctx.Value(taskIDContextKey).(string); ok {
		r.AddAttrs(slog.String("taskId", taskID))
	}
	// Extract resource key from context and add it to the record
	if resourceKey, ok := ctx.Value(resourceKeyContextKey).(string); ok {
		r.AddAttrs(slog.String("resourceKey", resourceKey))
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
