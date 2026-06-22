//go:build integration

package oncetask

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_HandlerPanicRecovery: a panicking handler is converted
// into an error, the task is retried per the retry policy, and a
// successful retry marks the task done.
func TestIntegration_HandlerPanicRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("panic_rec")
	var attempts int32

	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			panic("first attempt panics")
		}
		return nil
	}),
		WithLeaseDuration(5*time.Second),
		WithRetryPolicy(FixedDelayPolicy{MaxAttempts: 3, Delay: 100 * time.Millisecond}),
	))

	taskID := makeTaskID("panic")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
	require.NoError(t, err)

	requireWait(t, 20*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task should reach terminal state")

	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	task := tasks[0]
	assert.NotEmpty(t, task.DoneAt)
	assert.Equal(t, 2, task.Attempts, "expect attempt 1 panicked, attempt 2 succeeded")
	require.Len(t, task.Errors, 1, "panic should be captured as one task error")
	assert.Contains(t, task.Errors[0].Error, "panic", "error should mention panic")
}

// TestIntegration_HandlerPanic_PermanentFailure: a panicking handler with
// NoRetry results in a single attempt that ends in a permanent terminal
// state with the panic captured in the errors list.
func TestIntegration_HandlerPanic_PermanentFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("panic_perm")
	var attempts int32

	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&attempts, 1)
		panic("always panics")
	}),
		WithLeaseDuration(5*time.Second),
		WithNoRetry(),
	))

	taskID := makeTaskID("panic_perm")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
	require.NoError(t, err)

	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task should reach terminal state without retry")

	assert.Equal(t, int32(1), atomic.LoadInt32(&attempts), "WithNoRetry should not retry panicked task")

	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	require.Len(t, tasks[0].Errors, 1)
	assert.Contains(t, tasks[0].Errors[0].Error, "panic")
}

// TestIntegration_ContextHelpers: GetCurrentTaskID and
// GetCurrentTaskResourceKey return the current task's identifiers when
// invoked from inside a handler.
func TestIntegration_ContextHelpers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("ctx_helpers")
	rk := makeTaskID("ctx_rk")
	taskID := makeTaskID("ctx_task")

	type seenCtx struct {
		taskID      string
		resourceKey string
	}
	var seen atomic.Value // holds seenCtx

	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		seen.Store(seenCtx{
			taskID:      GetCurrentTaskID(ctx),
			resourceKey: GetCurrentTaskResourceKey(ctx),
		})
		return nil
	}), WithLeaseDuration(5*time.Second)))

	_, err := manager.CreateTask(ctx, testTaskData{
		Kind: kind, IDValue: taskID, Payload: "p", ResourceKey: rk,
	})
	require.NoError(t, err)

	requireWait(t, 15*time.Second, func() bool {
		return seen.Load() != nil
	}, "handler should run and record context")

	got := seen.Load().(seenCtx)
	assert.Equal(t, taskID, got.taskID, "GetCurrentTaskID should return the task id")
	assert.Equal(t, rk, got.resourceKey, "GetCurrentTaskResourceKey should return the resource key")
}

// TestIntegration_ContextHelpers_NoTaskInScope: outside a handler, the
// helpers return empty strings (don't panic).
func TestIntegration_ContextHelpers_NoTaskInScope(t *testing.T) {
	bg := context.Background()
	assert.Empty(t, GetCurrentTaskID(bg))
	assert.Empty(t, GetCurrentTaskResourceKey(bg))
}

// TestIntegration_ResourceKeyHandler_ContextScopes: the resource-key
// handler context always exposes the resource key. The task ID is
// populated only when the batch contains exactly one task; with multiple
// tasks the lib intentionally drops it (it would be ambiguous).
func TestIntegration_ResourceKeyHandler_ContextScopes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	type seen struct {
		taskID      string
		resourceKey string
		batchSize   int
	}

	t.Run("single_task_batch_includes_taskId", func(t *testing.T) {
		manager, _, cleanup := newTestManager[testKind](ctx, t)
		defer cleanup()

		kind := makeKind("ctx_rk_one")
		rk := makeTaskID("ctx_rk_key")
		var got atomic.Value

		require.NoError(t, manager.RegisterResourceKeyHandler(kind, NoResultResourceKey(func(ctx context.Context, tasks []OnceTask[testKind]) error {
			got.Store(seen{
				taskID:      GetCurrentTaskID(ctx),
				resourceKey: GetCurrentTaskResourceKey(ctx),
				batchSize:   len(tasks),
			})
			return nil
		}), WithLeaseDuration(5*time.Second)))

		taskID := makeTaskID("ctx_rk_task")
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind: kind, IDValue: taskID, Payload: "p", ResourceKey: rk,
		})
		require.NoError(t, err)

		requireWait(t, 15*time.Second, func() bool { return got.Load() != nil }, "handler should run")

		v := got.Load().(seen)
		assert.Equal(t, 1, v.batchSize)
		assert.Equal(t, taskID, v.taskID, "single-task batch should expose task id")
		assert.Equal(t, rk, v.resourceKey)
	})

	t.Run("multi_task_batch_drops_taskId", func(t *testing.T) {
		manager, _, cleanup := newTestManager[testKind](ctx, t)
		defer cleanup()

		kind := makeKind("ctx_rk_many")
		rk := makeTaskID("ctx_rk_key_many")
		var got atomic.Value

		// Create both tasks BEFORE registering so they land in the same
		// batch on the worker's first poll.
		taskA := makeTaskID("ctx_rk_a")
		taskB := makeTaskID("ctx_rk_b")
		_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskA, Payload: "p", ResourceKey: rk})
		require.NoError(t, err)
		_, err = manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskB, Payload: "p", ResourceKey: rk})
		require.NoError(t, err)

		require.NoError(t, manager.RegisterResourceKeyHandler(kind, NoResultResourceKey(func(ctx context.Context, tasks []OnceTask[testKind]) error {
			got.Store(seen{
				taskID:      GetCurrentTaskID(ctx),
				resourceKey: GetCurrentTaskResourceKey(ctx),
				batchSize:   len(tasks),
			})
			return nil
		}), WithLeaseDuration(5*time.Second)))

		requireWait(t, 15*time.Second, func() bool { return got.Load() != nil }, "handler should run")

		v := got.Load().(seen)
		assert.Equal(t, 2, v.batchSize, "batch should contain both tasks")
		assert.Empty(t, v.taskID, "multi-task batch should NOT expose a single task id")
		assert.Equal(t, rk, v.resourceKey)
	})
}

// touch time-import so this file remains buildable if every other usage
// is moved.
var _ = time.Second
