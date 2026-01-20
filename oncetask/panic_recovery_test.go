package oncetask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSafeExecute_Success(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, input string) (string, error) {
		return "got: " + input, nil
	}

	result, err := SafeExecute(ctx, handler, "test")

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "got: test" {
		t.Errorf("expected 'got: test', got %q", result)
	}
}

func TestSafeExecute_ReturnsError(t *testing.T) {
	ctx := context.Background()
	expectedErr := errors.New("handler error")
	handler := func(ctx context.Context, input string) (string, error) {
		return "", expectedErr
	}

	result, err := SafeExecute(ctx, handler, "test")

	if err != expectedErr {
		t.Errorf("expected %v, got %v", expectedErr, err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestSafeExecute_RecoversPanic_String(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, input string) (string, error) {
		panic("something went wrong")
	}

	result, err := SafeExecute(ctx, handler, "test")

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
	handler := func(ctx context.Context, input int) (int, error) {
		panic(panicValue)
	}

	_, err := SafeExecute(ctx, handler, 42)

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "panic with error") {
		t.Errorf("expected error to contain panic message, got %v", err)
	}
}

func TestSafeExecute_RecoversPanic_Int(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, input any) (any, error) {
		panic(42)
	}

	_, err := SafeExecute(ctx, handler, nil)

	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("expected error to contain '42', got %v", err)
	}
}

func TestSafeExecute_RecoversPanic_Nil(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, input any) (any, error) {
		panic(nil) //nolint:govet // Intentionally testing panic(nil) recovery
	}

	_, err := SafeExecute(ctx, handler, nil)

	// Go 1.21+ returns *runtime.PanicNilError for panic(nil)
	// We just verify panic is recovered and converted to an error
	if err == nil {
		t.Fatal("expected error from panic(nil)")
	}
	if !strings.Contains(err.Error(), "panic:") {
		t.Errorf("expected error to contain 'panic:', got %v", err)
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
		t.Errorf("expected context value 'test-value', got %v", capturedValue)
	}
}

func TestSafeExecute_PassesParameter(t *testing.T) {
	ctx := context.Background()
	var capturedParam int
	handler := func(ctx context.Context, input int) (int, error) {
		capturedParam = input
		return input * 2, nil
	}

	result, err := SafeExecute(ctx, handler, 21)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if capturedParam != 21 {
		t.Errorf("expected captured param 21, got %d", capturedParam)
	}
	if result != 42 {
		t.Errorf("expected result 42, got %d", result)
	}
}

func TestSafeExecute_ComplexTypes(t *testing.T) {
	type request struct {
		Name  string
		Count int
	}
	type response struct {
		Message string
		Items   []string
	}

	ctx := context.Background()
	handler := func(ctx context.Context, req request) (response, error) {
		items := make([]string, req.Count)
		for i := range items {
			items[i] = req.Name
		}
		return response{Message: "done", Items: items}, nil
	}

	result, err := SafeExecute(ctx, handler, request{Name: "test", Count: 3})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result.Message != "done" {
		t.Errorf("expected message 'done', got %q", result.Message)
	}
	if len(result.Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(result.Items))
	}
}

// TaskKind for testing with actual handler types
type testTaskKind string

func TestSafeExecute_WithTaskHandler(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		return "processed: " + task.Id, nil
	}

	task := &OnceTask[testTaskKind]{Id: "task-123"}
	result, err := SafeExecute(ctx, handler, task)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "processed: task-123" {
		t.Errorf("expected 'processed: task-123', got %v", result)
	}
}

func TestSafeExecute_WithTaskHandler_Panic(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, task *OnceTask[testTaskKind]) (any, error) {
		panic("handler panic")
	}

	task := &OnceTask[testTaskKind]{Id: "task-123"}
	_, err := SafeExecute(ctx, handler, task)

	if err == nil {
		t.Fatal("expected error from panic")
	}
	if !strings.Contains(err.Error(), "handler panic") {
		t.Errorf("expected error to contain 'handler panic', got %v", err)
	}
}

func TestSafeExecute_WithResourceKeyHandler(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
		return len(tasks), nil
	}

	tasks := []OnceTask[testTaskKind]{{Id: "1"}, {Id: "2"}, {Id: "3"}}
	result, err := SafeExecute(ctx, handler, tasks)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 3 {
		t.Errorf("expected result 3, got %v", result)
	}
}

func TestSafeExecute_WithResourceKeyHandler_Panic(t *testing.T) {
	ctx := context.Background()
	handler := func(ctx context.Context, tasks []OnceTask[testTaskKind]) (any, error) {
		panic("resource handler panic")
	}

	tasks := []OnceTask[testTaskKind]{{Id: "1"}}
	_, err := SafeExecute(ctx, handler, tasks)

	if err == nil {
		t.Fatal("expected error from panic")
	}
	if !strings.Contains(err.Error(), "resource handler panic") {
		t.Errorf("expected error to contain 'resource handler panic', got %v", err)
	}
}
