package oncetask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSafeExecute(t *testing.T) {
	tests := []struct {
		name           string
		fn             func() (string, error)
		wantResult     string
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "success",
			fn: func() (string, error) {
				return "success", nil
			},
			wantResult: "success",
			wantErr:    false,
		},
		{
			name: "returns error",
			fn: func() (string, error) {
				return "", errors.New("handler error")
			},
			wantResult:     "",
			wantErr:        true,
			wantErrContain: "handler error",
		},
		{
			name: "recovers panic with string",
			fn: func() (string, error) {
				panic("something went wrong")
			},
			wantResult:     "",
			wantErr:        true,
			wantErrContain: "something went wrong",
		},
		{
			name: "recovers panic with error",
			fn: func() (string, error) {
				panic(errors.New("panic error"))
			},
			wantResult:     "",
			wantErr:        true,
			wantErrContain: "panic error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SafeExecute(context.Background(), tt.fn)

			if result != tt.wantResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}

			if tt.wantErrContain != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("err = %v, want containing %q", err, tt.wantErrContain)
				}
			}
		})
	}
}

func TestSafeExecute_PanicTypes(t *testing.T) {
	tests := []struct {
		name       string
		panicValue any
		wantContain string
	}{
		{
			name:        "panic with int",
			panicValue:  42,
			wantContain: "42",
		},
		{
			name:        "panic with struct",
			panicValue:  struct{ msg string }{"structured panic"},
			wantContain: "structured panic",
		},
		{
			name:        "panic with nil",
			panicValue:  nil,
			wantContain: "panic:", // Go 1.21+ wraps nil panics
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeExecute(context.Background(), func() (any, error) {
				panic(tt.panicValue)
			})

			if err == nil {
				t.Fatal("expected error from panic")
			}
			if !strings.Contains(err.Error(), tt.wantContain) {
				t.Errorf("err = %v, want containing %q", err, tt.wantContain)
			}
		})
	}
}

func TestSafeExecute_ComplexResult(t *testing.T) {
	type complexResult struct {
		Name  string
		Count int
		Items []string
	}

	expected := complexResult{
		Name:  "test",
		Count: 42,
		Items: []string{"a", "b", "c"},
	}

	result, err := SafeExecute(context.Background(), func() (complexResult, error) {
		return expected, nil
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Name != expected.Name || result.Count != expected.Count {
		t.Errorf("result = %+v, want %+v", result, expected)
	}
}

// testTaskKind for handler tests
type testTaskKind string

func TestSafeHandler(t *testing.T) {
	tests := []struct {
		name           string
		handler        Handler[testTaskKind]
		wantResult     any
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "success",
			handler: func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
				return "result", nil
			},
			wantResult: "result",
			wantErr:    false,
		},
		{
			name: "returns error",
			handler: func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
				return nil, errors.New("handler failed")
			},
			wantResult:     nil,
			wantErr:        true,
			wantErrContain: "handler failed",
		},
		{
			name: "recovers panic",
			handler: func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
				panic("handler panic")
			},
			wantResult:     nil,
			wantErr:        true,
			wantErrContain: "handler panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe := SafeHandler(tt.handler)
			result, err := safe(context.Background(), &OnceTask[testTaskKind]{})

			if result != tt.wantResult {
				t.Errorf("result = %v, want %v", result, tt.wantResult)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}

			if tt.wantErrContain != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("err = %v, want containing %q", err, tt.wantErrContain)
				}
			}
		})
	}
}

func TestSafeHandler_PreservesContext(t *testing.T) {
	type ctxKey string
	key := ctxKey("test-key")

	var capturedValue any
	handler := SafeHandler(func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		capturedValue = ctx.Value(key)
		return nil, nil
	})

	ctx := context.WithValue(context.Background(), key, "test-value")
	_, _ = handler(ctx, &OnceTask[testTaskKind]{})

	if capturedValue != "test-value" {
		t.Errorf("context value = %v, want %q", capturedValue, "test-value")
	}
}

func TestSafeHandler_PreservesTaskData(t *testing.T) {
	var capturedTask *OnceTask[testTaskKind]
	handler := SafeHandler(func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		capturedTask = task
		return nil, nil
	})

	inputTask := &OnceTask[testTaskKind]{Id: "test-id-123"}
	_, _ = handler(context.Background(), inputTask)

	if capturedTask != inputTask {
		t.Error("expected same task instance")
	}
	if capturedTask.Id != "test-id-123" {
		t.Errorf("task ID = %q, want %q", capturedTask.Id, "test-id-123")
	}
}

func TestSafeResourceKeyHandler(t *testing.T) {
	tests := []struct {
		name           string
		handler        ResourceKeyHandler[testTaskKind]
		tasks          []OnceTask[testTaskKind]
		wantResult     any
		wantErr        bool
		wantErrContain string
	}{
		{
			name: "success",
			handler: func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
				return len(tasks), nil
			},
			tasks:      []OnceTask[testTaskKind]{{Id: "1"}, {Id: "2"}, {Id: "3"}},
			wantResult: 3,
			wantErr:    false,
		},
		{
			name: "returns error",
			handler: func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
				return nil, errors.New("resource handler failed")
			},
			tasks:          nil,
			wantResult:     nil,
			wantErr:        true,
			wantErrContain: "resource handler failed",
		},
		{
			name: "recovers panic",
			handler: func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
				panic("resource handler panic")
			},
			tasks:          nil,
			wantResult:     nil,
			wantErr:        true,
			wantErrContain: "resource handler panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			safe := SafeResourceKeyHandler(tt.handler)
			result, err := safe(context.Background(), tt.tasks)

			if result != tt.wantResult {
				t.Errorf("result = %v, want %v", result, tt.wantResult)
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, tt.wantErr)
			}

			if tt.wantErrContain != "" && err != nil {
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("err = %v, want containing %q", err, tt.wantErrContain)
				}
			}
		})
	}
}

func TestSafeResourceKeyHandler_PreservesTasks(t *testing.T) {
	var capturedTasks []OnceTask[testTaskKind]
	handler := SafeResourceKeyHandler(func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
		capturedTasks = tasks
		return nil, nil
	})

	inputTasks := []OnceTask[testTaskKind]{{Id: "a"}, {Id: "b"}}
	_, _ = handler(context.Background(), inputTasks)

	if len(capturedTasks) != 2 {
		t.Errorf("captured %d tasks, want 2", len(capturedTasks))
	}
	if capturedTasks[0].Id != "a" || capturedTasks[1].Id != "b" {
		t.Error("task data was not preserved")
	}
}
