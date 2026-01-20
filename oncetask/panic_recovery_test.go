package oncetask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSafeExecute(t *testing.T) {
	tests := []struct {
		handler        func(context.Context, string) (string, error)
		name           string
		input          string
		wantResult     string
		wantErrContain string
		wantErr        bool
	}{
		{
			name: "success returns result",
			handler: func(ctx context.Context, input string) (string, error) {
				return "got: " + input, nil
			},
			input:      "test",
			wantResult: "got: test",
			wantErr:    false,
		},
		{
			name: "error is passed through",
			handler: func(ctx context.Context, input string) (string, error) {
				return "", errors.New("handler error")
			},
			input:          "test",
			wantResult:     "",
			wantErr:        true,
			wantErrContain: "handler error",
		},
		{
			name: "panic with string is recovered",
			handler: func(ctx context.Context, input string) (string, error) {
				panic("something went wrong")
			},
			input:          "test",
			wantResult:     "",
			wantErr:        true,
			wantErrContain: "something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeExecute(context.Background(), tt.handler, tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("SafeExecute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Errorf("SafeExecute() error = %v, want containing %q", err, tt.wantErrContain)
			}
			if result != tt.wantResult {
				t.Errorf("SafeExecute() result = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestSafeExecute_PanicRecovery(t *testing.T) {
	tests := []struct {
		name           string
		panicValue     any
		wantErrContain string
	}{
		{
			name:           "string panic",
			panicValue:     "panic message",
			wantErrContain: "panic message",
		},
		{
			name:           "error panic",
			panicValue:     errors.New("error as panic"),
			wantErrContain: "error as panic",
		},
		{
			name:           "int panic",
			panicValue:     42,
			wantErrContain: "42",
		},
		{
			name:           "nil panic",
			panicValue:     nil,
			wantErrContain: "panic:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(ctx context.Context, input any) (any, error) {
				if tt.panicValue == nil {
					panic(nil) //nolint:govet // Intentionally testing panic(nil) recovery
				}
				panic(tt.panicValue)
			}

			_, err := SafeExecute(context.Background(), handler, nil)

			if err == nil {
				t.Fatal("expected error from panic, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrContain)
			}
		})
	}
}

func TestSafeExecute_PreservesContext(t *testing.T) {
	type ctxKey string
	key := ctxKey("test-key")

	var capturedValue any
	handler := func(ctx context.Context, input string) (string, error) {
		capturedValue = ctx.Value(key)
		return "", nil
	}

	ctx := context.WithValue(context.Background(), key, "test-value")
	_, _ = SafeExecute(ctx, handler, "input")

	if capturedValue != "test-value" {
		t.Errorf("context value = %v, want %v", capturedValue, "test-value")
	}
}

func TestSafeExecute_PassesParameter(t *testing.T) {
	var capturedParam int
	handler := func(ctx context.Context, input int) (int, error) {
		capturedParam = input
		return input * 2, nil
	}

	result, err := SafeExecute(context.Background(), handler, 21)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if capturedParam != 21 {
		t.Errorf("captured param = %d, want 21", capturedParam)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42", result)
	}
}

// testTaskKind for testing with actual handler types
type testTaskKind string

func TestSafeExecute_WithHandlerTypes(t *testing.T) {
	t.Run("task handler success", func(t *testing.T) {
		handler := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
			return "processed: " + task.Id, nil
		}

		task := &OnceTask[testTaskKind]{Id: "task-123"}
		result, err := SafeExecute(context.Background(), handler, task)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "processed: task-123" {
			t.Errorf("result = %v, want %v", result, "processed: task-123")
		}
	})

	t.Run("task handler panic", func(t *testing.T) {
		handler := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
			panic("handler panic")
		}

		task := &OnceTask[testTaskKind]{Id: "task-123"}
		_, err := SafeExecute(context.Background(), handler, task)

		if err == nil {
			t.Fatal("expected error from panic")
		}
		if !strings.Contains(err.Error(), "handler panic") {
			t.Errorf("error = %v, want containing %q", err, "handler panic")
		}
	})

	t.Run("resource key handler success", func(t *testing.T) {
		handler := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
			return len(tasks), nil
		}

		tasks := []OnceTask[testTaskKind]{{Id: "1"}, {Id: "2"}, {Id: "3"}}
		result, err := SafeExecute(context.Background(), handler, tasks)

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != 3 {
			t.Errorf("result = %v, want 3", result)
		}
	})

	t.Run("resource key handler panic", func(t *testing.T) {
		handler := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
			panic("resource handler panic")
		}

		tasks := []OnceTask[testTaskKind]{{Id: "1"}}
		_, err := SafeExecute(context.Background(), handler, tasks)

		if err == nil {
			t.Fatal("expected error from panic")
		}
		if !strings.Contains(err.Error(), "resource handler panic") {
			t.Errorf("error = %v, want containing %q", err, "resource handler panic")
		}
	})
}
