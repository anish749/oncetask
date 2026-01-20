package oncetask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSafeExecute_Success(t *testing.T) {
	result, err := SafeExecute(func() (string, error) {
		return "success", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %q", result)
	}
}

func TestSafeExecute_ReturnsError(t *testing.T) {
	expectedErr := errors.New("handler error")

	result, err := SafeExecute(func() (string, error) {
		return "", expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	if IsPanicError(err) {
		t.Error("expected regular error, not PanicError")
	}
}

func TestSafeExecute_RecoversPanic_String(t *testing.T) {
	result, err := SafeExecute(func() (string, error) {
		panic("something went wrong")
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}

	panicErr := AsPanicError(err)
	if panicErr == nil {
		t.Fatal("expected PanicError, got different error type")
	}

	if panicErr.Value != "something went wrong" {
		t.Errorf("expected panic value 'something went wrong', got %v", panicErr.Value)
	}

	if !strings.Contains(panicErr.Stack, "panic_recovery_test.go") {
		t.Errorf("expected stack trace to contain test file, got:\n%s", panicErr.Stack)
	}
}

func TestSafeExecute_RecoversPanic_Error(t *testing.T) {
	panicValue := errors.New("panic with error")

	_, err := SafeExecute(func() (int, error) {
		panic(panicValue)
	})

	panicErr := AsPanicError(err)
	if panicErr == nil {
		t.Fatal("expected PanicError")
	}

	if panicErr.Value != panicValue {
		t.Errorf("expected panic value to be the error, got %v", panicErr.Value)
	}
}

func TestSafeExecute_RecoversPanic_Int(t *testing.T) {
	_, err := SafeExecute(func() (any, error) {
		panic(42)
	})

	panicErr := AsPanicError(err)
	if panicErr == nil {
		t.Fatal("expected PanicError")
	}

	if panicErr.Value != 42 {
		t.Errorf("expected panic value 42, got %v", panicErr.Value)
	}
}

func TestSafeExecute_RecoversPanic_Nil(t *testing.T) {
	_, err := SafeExecute(func() (any, error) {
		panic(nil)
	})

	panicErr := AsPanicError(err)
	if panicErr == nil {
		t.Fatal("expected PanicError")
	}

	// Go 1.21+ returns *runtime.PanicNilError for panic(nil)
	// Earlier versions returned nil. We just verify it's captured.
	if err == nil {
		t.Error("expected error to be non-nil")
	}
}

func TestPanicError_Error(t *testing.T) {
	pe := &PanicError{
		Value: "test panic",
		Stack: "fake stack trace",
	}

	expected := "panic: test panic"
	if pe.Error() != expected {
		t.Errorf("expected %q, got %q", expected, pe.Error())
	}
}

func TestPanicError_FullError(t *testing.T) {
	pe := &PanicError{
		Value: "test panic",
		Stack: "line 1\nline 2",
	}

	fullErr := pe.FullError()

	if !strings.Contains(fullErr, "panic: test panic") {
		t.Errorf("expected full error to contain panic message")
	}
	if !strings.Contains(fullErr, "stack trace:") {
		t.Errorf("expected full error to contain stack trace header")
	}
	if !strings.Contains(fullErr, "line 1\nline 2") {
		t.Errorf("expected full error to contain stack trace content")
	}
}

func TestIsPanicError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "regular error",
			err:      errors.New("regular error"),
			expected: false,
		},
		{
			name:     "panic error",
			err:      &PanicError{Value: "panic"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPanicError(tt.err)
			if result != tt.expected {
				t.Errorf("IsPanicError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestAsPanicError(t *testing.T) {
	t.Run("returns nil for regular error", func(t *testing.T) {
		err := errors.New("regular error")
		if AsPanicError(err) != nil {
			t.Error("expected nil for regular error")
		}
	})

	t.Run("returns nil for nil error", func(t *testing.T) {
		if AsPanicError(nil) != nil {
			t.Error("expected nil for nil error")
		}
	})

	t.Run("returns PanicError for PanicError", func(t *testing.T) {
		pe := &PanicError{Value: "test"}
		result := AsPanicError(pe)
		if result != pe {
			t.Errorf("expected same PanicError instance")
		}
	})
}

// TaskKind for testing
type testTaskKind string

func TestSafeHandler_Success(t *testing.T) {
	original := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		return "result", nil
	}

	safe := SafeHandler(original)
	result, err := safe(context.Background(), &OnceTask[testTaskKind]{})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "result" {
		t.Errorf("expected 'result', got %v", result)
	}
}

func TestSafeHandler_ReturnsError(t *testing.T) {
	expectedErr := errors.New("handler failed")
	original := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		return nil, expectedErr
	}

	safe := SafeHandler(original)
	_, err := safe(context.Background(), &OnceTask[testTaskKind]{})

	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestSafeHandler_RecoversPanic(t *testing.T) {
	original := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		panic("handler panic")
	}

	safe := SafeHandler(original)
	_, err := safe(context.Background(), &OnceTask[testTaskKind]{})

	if err == nil {
		t.Fatal("expected error from panic")
	}

	panicErr := AsPanicError(err)
	if panicErr == nil {
		t.Fatal("expected PanicError")
	}
	if panicErr.Value != "handler panic" {
		t.Errorf("expected panic value 'handler panic', got %v", panicErr.Value)
	}
}

func TestSafeHandler_PreservesContext(t *testing.T) {
	type ctxKey string
	key := ctxKey("test-key")

	var capturedValue any
	original := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		capturedValue = ctx.Value(key)
		return nil, nil
	}

	safe := SafeHandler(original)
	ctx := context.WithValue(context.Background(), key, "test-value")
	_, _ = safe(ctx, &OnceTask[testTaskKind]{})

	if capturedValue != "test-value" {
		t.Errorf("expected context value 'test-value', got %v", capturedValue)
	}
}

func TestSafeHandler_PreservesTaskData(t *testing.T) {
	var capturedTask *OnceTask[testTaskKind]
	original := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		capturedTask = task
		return nil, nil
	}

	safe := SafeHandler(original)
	inputTask := &OnceTask[testTaskKind]{Id: "test-id-123"}
	_, _ = safe(context.Background(), inputTask)

	if capturedTask != inputTask {
		t.Error("expected same task instance to be passed")
	}
	if capturedTask.Id != "test-id-123" {
		t.Errorf("expected task ID 'test-id-123', got %q", capturedTask.Id)
	}
}

func TestSafeResourceKeyHandler_Success(t *testing.T) {
	original := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
		return len(tasks), nil
	}

	safe := SafeResourceKeyHandler(original)
	tasks := []OnceTask[testTaskKind]{{Id: "1"}, {Id: "2"}, {Id: "3"}}
	result, err := safe(context.Background(), tasks)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 3 {
		t.Errorf("expected result 3, got %v", result)
	}
}

func TestSafeResourceKeyHandler_ReturnsError(t *testing.T) {
	expectedErr := errors.New("resource handler failed")
	original := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
		return nil, expectedErr
	}

	safe := SafeResourceKeyHandler(original)
	_, err := safe(context.Background(), nil)

	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
}

func TestSafeResourceKeyHandler_RecoversPanic(t *testing.T) {
	original := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
		panic("resource handler panic")
	}

	safe := SafeResourceKeyHandler(original)
	_, err := safe(context.Background(), nil)

	if err == nil {
		t.Fatal("expected error from panic")
	}

	panicErr := AsPanicError(err)
	if panicErr == nil {
		t.Fatal("expected PanicError")
	}
	if panicErr.Value != "resource handler panic" {
		t.Errorf("expected panic value 'resource handler panic', got %v", panicErr.Value)
	}
}

func TestSafeResourceKeyHandler_PreservesTasks(t *testing.T) {
	var capturedTasks []OnceTask[testTaskKind]
	original := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
		capturedTasks = tasks
		return nil, nil
	}

	safe := SafeResourceKeyHandler(original)
	inputTasks := []OnceTask[testTaskKind]{{Id: "a"}, {Id: "b"}}
	_, _ = safe(context.Background(), inputTasks)

	if len(capturedTasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(capturedTasks))
	}
	if capturedTasks[0].Id != "a" || capturedTasks[1].Id != "b" {
		t.Error("task data was not preserved")
	}
}

// TestSafeExecute_StackTraceQuality verifies that the stack trace is useful for debugging
func TestSafeExecute_StackTraceQuality(t *testing.T) {
	_, err := SafeExecute(func() (any, error) {
		nestedPanic()
		return nil, nil
	})

	panicErr := AsPanicError(err)
	if panicErr == nil {
		t.Fatal("expected PanicError")
	}

	// Stack should contain the function that panicked
	if !strings.Contains(panicErr.Stack, "nestedPanic") {
		t.Errorf("expected stack to contain 'nestedPanic', got:\n%s", panicErr.Stack)
	}

	// Stack should contain runtime.gopanic (standard Go panic marker)
	if !strings.Contains(panicErr.Stack, "panic") {
		t.Errorf("expected stack to contain panic marker")
	}
}

func nestedPanic() {
	panic("nested panic")
}

// TestSafeExecute_ComplexResult ensures complex types work correctly
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

	result, err := SafeExecute(func() (complexResult, error) {
		return expected, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.Name != expected.Name || result.Count != expected.Count {
		t.Errorf("expected %+v, got %+v", expected, result)
	}
}
