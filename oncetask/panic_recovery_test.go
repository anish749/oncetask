package oncetask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSafeExecute_Success(t *testing.T) {
	ctx := context.Background()
	result, err := SafeExecute(ctx, func() (string, error) {
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
	ctx := context.Background()
	expectedErr := errors.New("handler error")

	result, err := SafeExecute(ctx, func() (string, error) {
		return "", expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestSafeExecute_RecoversPanic_String(t *testing.T) {
	ctx := context.Background()
	result, err := SafeExecute(ctx, func() (string, error) {
		panic("something went wrong")
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	if !strings.Contains(err.Error(), "panic:") {
		t.Errorf("expected error to contain 'panic:', got %v", err)
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("expected error to contain panic message, got %v", err)
	}
}

func TestSafeExecute_RecoversPanic_Error(t *testing.T) {
	ctx := context.Background()
	panicValue := errors.New("panic with error")

	_, err := SafeExecute(ctx, func() (int, error) {
		panic(panicValue)
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "panic with error") {
		t.Errorf("expected error to contain panic message, got %v", err)
	}
}

func TestSafeExecute_RecoversPanic_Int(t *testing.T) {
	ctx := context.Background()
	_, err := SafeExecute(ctx, func() (any, error) {
		panic(42)
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("expected error to contain '42', got %v", err)
	}
}

func TestSafeExecute_RecoversPanic_Nil(t *testing.T) {
	ctx := context.Background()
	_, err := SafeExecute(ctx, func() (any, error) {
		panic(nil)
	})

	// Go 1.21+ returns *runtime.PanicNilError for panic(nil)
	// We just verify panic is recovered and converted to an error
	if err == nil {
		t.Fatal("expected error from panic(nil)")
	}
	if !strings.Contains(err.Error(), "panic:") {
		t.Errorf("expected error to contain 'panic:', got %v", err)
	}
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
	if !strings.Contains(err.Error(), "handler panic") {
		t.Errorf("expected error to contain 'handler panic', got %v", err)
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
	if !strings.Contains(err.Error(), "resource handler panic") {
		t.Errorf("expected error to contain 'resource handler panic', got %v", err)
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

	ctx := context.Background()
	result, err := SafeExecute(ctx, func() (complexResult, error) {
		return expected, nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.Name != expected.Name || result.Count != expected.Count {
		t.Errorf("expected %+v, got %+v", expected, result)
	}
}
