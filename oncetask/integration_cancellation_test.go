//go:build integration

package oncetask

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Cancellation covers every cancellation entry point —
// CancelTask, CancelTasksByIds, CancelTasksByResourceKey — and the
// cancellation handler lifecycle in a single table.
func TestIntegration_Cancellation(t *testing.T) {
	type setup struct {
		// number of tasks to create with the same resource key
		withRk int
		// number of standalone tasks (no resource key)
		standalone int
	}
	type cancelOp string
	const (
		cancelByID          cancelOp = "byId"
		cancelByIds         cancelOp = "byIds"
		cancelByResourceKey cancelOp = "byResourceKey"
	)

	type expect struct {
		// expected number of tasks reported as cancelled by the call
		count int
		// every task in the resource-key group should observe IsCancelled=true
		rkCancelled bool
		// every standalone task should observe IsCancelled=true
		standaloneCancelled bool
	}

	type tcase struct {
		name   string
		op     cancelOp
		setup  setup
		expect expect
	}

	cases := []tcase{
		{
			name:   "cancel_single_by_id",
			setup:  setup{standalone: 1},
			op:     cancelByID,
			expect: expect{count: 1, standaloneCancelled: true},
		},
		{
			name:   "cancel_bulk_by_ids",
			setup:  setup{standalone: 3},
			op:     cancelByIds,
			expect: expect{count: 3, standaloneCancelled: true},
		},
		{
			name:   "cancel_by_resource_key_cancels_only_matching",
			setup:  setup{withRk: 4, standalone: 2},
			op:     cancelByResourceKey,
			expect: expect{count: 4, rkCancelled: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			manager, _, cleanup := newTestManager[testKind](ctx, t)
			defer cleanup()

			kind := makeKind("cancel")
			rk := fmt.Sprintf("rk_%d", uniqueSuffix())

			var rkIDs, standaloneIDs []string
			for i := 0; i < tc.setup.withRk; i++ {
				id := fmt.Sprintf("rk_task_%d_%d", uniqueSuffix(), i)
				rkIDs = append(rkIDs, id)
				_, err := manager.CreateTask(ctx, testTaskData{
					Kind: kind, IDValue: id, Payload: "p", ResourceKey: rk,
				})
				require.NoError(t, err)
			}
			for i := 0; i < tc.setup.standalone; i++ {
				id := fmt.Sprintf("std_task_%d_%d", uniqueSuffix(), i)
				standaloneIDs = append(standaloneIDs, id)
				_, err := manager.CreateTask(ctx, testTaskData{
					Kind: kind, IDValue: id, Payload: "p",
				})
				require.NoError(t, err)
			}

			switch tc.op {
			case cancelByID:
				require.Len(t, standaloneIDs, 1)
				err := manager.CancelTask(ctx, standaloneIDs[0])
				require.NoError(t, err)
			case cancelByIds:
				count, err := manager.CancelTasksByIds(ctx, standaloneIDs)
				require.NoError(t, err)
				assert.Equal(t, tc.expect.count, count)
			case cancelByResourceKey:
				count, err := manager.CancelTasksByResourceKey(ctx, kind, rk)
				require.NoError(t, err)
				assert.Equal(t, tc.expect.count, count)
			}

			if tc.expect.rkCancelled {
				tasks, err := manager.GetTasksByIds(ctx, rkIDs)
				require.NoError(t, err)
				require.Len(t, tasks, len(rkIDs))
				for _, task := range tasks {
					assert.True(t, task.IsCancelled, "task %s should be cancelled", task.Id)
					assert.NotEmpty(t, task.CancelledAt)
					assert.Equal(t, NoWait, task.WaitUntil, "cancelled task should have waitUntil=NoWait")
				}
			}
			if tc.expect.standaloneCancelled {
				tasks, err := manager.GetTasksByIds(ctx, standaloneIDs)
				require.NoError(t, err)
				require.Len(t, tasks, len(standaloneIDs))
				for _, task := range tasks {
					assert.True(t, task.IsCancelled)
					assert.NotEmpty(t, task.CancelledAt)
				}
			}

			// Standalone tasks should NOT be cancelled when we cancelled by resource key.
			if tc.op == cancelByResourceKey && len(standaloneIDs) > 0 {
				tasks, err := manager.GetTasksByIds(ctx, standaloneIDs)
				require.NoError(t, err)
				for _, task := range tasks {
					assert.False(t, task.IsCancelled,
						"standalone task %s should NOT be cancelled by resource key cancel", task.Id)
				}
			}
		})
	}
}

// TestIntegration_CancellationIdempotency: cancelling already-cancelled
// or already-done tasks is a no-op and doesn't return errors.
func TestIntegration_CancellationIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("cancel_idem")

	// Set up: one task we'll cancel twice; one task we'll let complete first.
	cancelMeID := makeTaskID("cancelme")
	completeMeID := makeTaskID("completeme")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: cancelMeID, Payload: "p"})
	require.NoError(t, err)
	_, err = manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: completeMeID, Payload: "p"})
	require.NoError(t, err)

	// First cancel — should mark cancelMe as cancelled.
	require.NoError(t, manager.CancelTask(ctx, cancelMeID))

	// Second cancel — should be a no-op (counts 0 cancelled).
	count, err := manager.CancelTasksByIds(ctx, []string{cancelMeID})
	require.NoError(t, err)
	assert.Equal(t, 0, count, "re-cancelling already-cancelled task should be 0")

	// Now complete the second task by registering a quick handler.
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		// completeMe will run; cancelMe is already cancelled with NoWait,
		// so it'll also fire its (no-op) cancellation handler — that's fine.
		return nil
	})))

	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{completeMeID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "completeMe should finish")

	// Cancelling a done task should now also be a no-op.
	count, err = manager.CancelTasksByIds(ctx, []string{completeMeID})
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

// TestIntegration_CancellationHandlerRuns: when WithCancellationHandler is
// configured and a task gets cancelled, the cancellation handler executes
// (instead of the normal handler) and the task is marked done.
func TestIntegration_CancellationHandlerRuns(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("cancel_handler")

	var (
		normalCount, cancelCount int32
	)
	cancellationHandler := Handler[testKind](func(ctx context.Context, task *OnceTask[testKind]) (any, error) {
		atomic.AddInt32(&cancelCount, 1)
		assert.True(t, task.IsCancelled, "cancellation handler should see IsCancelled=true")
		return nil, nil
	})

	// Create the task FIRST, mark it cancelled, then register the
	// handler. That way the worker's first poll sees the cancelled task
	// and routes it through the cancellation handler.
	taskID := makeTaskID("cancel_h")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
	require.NoError(t, err)
	require.NoError(t, manager.CancelTask(ctx, taskID))

	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&normalCount, 1)
		return nil
	}),
		WithCancellationHandler(cancellationHandler),
		WithLeaseDuration(10*time.Second),
	))

	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "cancelled task should reach done state via cancellation handler")

	assert.Equal(t, int32(1), atomic.LoadInt32(&cancelCount), "cancellation handler should run once")
	assert.Equal(t, int32(0), atomic.LoadInt32(&normalCount), "normal handler should NOT run for cancelled task")

	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	assert.True(t, tasks[0].IsCancelled)
	assert.NotEmpty(t, tasks[0].DoneAt)
}

// TestIntegration_CancellationRetryPolicy: WithCancellationRetryPolicy is
// honoured separately from the normal RetryPolicy, and cancellation
// handler failures are retried per that policy.
func TestIntegration_CancellationRetryPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("cancel_retry")
	var attempts int32
	cancellationHandler := Handler[testKind](func(ctx context.Context, task *OnceTask[testKind]) (any, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			return nil, errors.New("transient cleanup failure")
		}
		return nil, nil
	})

	taskID := makeTaskID("cancel_retry")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
	require.NoError(t, err)
	require.NoError(t, manager.CancelTask(ctx, taskID))

	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		return nil
	}),
		WithCancellationHandler(cancellationHandler),
		WithCancellationRetryPolicy(FixedDelayPolicy{
			MaxAttempts: 5,
			Delay:       100 * time.Millisecond,
		}),
		WithLeaseDuration(5*time.Second),
	))

	requireWait(t, 20*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "cancelled task should eventually reach done via retried cancellation handler")

	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts),
		"cancellation handler should retry per CancellationRetryPolicy")
	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	assert.True(t, tasks[0].IsCancelled)
	assert.NotEmpty(t, tasks[0].DoneAt)
}

// TestIntegration_CancellationCrossEnv: cancelling a task that lives in a
// different environment must error.
//
// We can't simply spin up two managers in the same test function because
// the ONCE_TASK_ENV that the library reads is process-global; t.Setenv
// would just leave whichever value was set last. Instead we plant the
// "other env" task directly on Firestore and then use a normally-scoped
// manager to attempt cancellation of it.
func TestIntegration_CancellationCrossEnv(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, ourEnv, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("cancel_xenv")
	taskID := makeTaskID("xenv")

	// Plant a task in a different env, directly on Firestore.
	otherEnv := ourEnv + "_other"
	otherTask := OnceTask[testKind]{
		Id:          taskID,
		Type:        kind,
		Env:         otherEnv,
		Data:        map[string]any{"payload": "p"},
		WaitUntil:   NoWait,
		LeasedUntil: "",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	client := rawTestClient(ctx, t)
	_, err := client.Collection(CollectionOnceTasks).Doc(taskID).Create(ctx, otherTask)
	require.NoError(t, err)

	count, err := manager.CancelTasksByIds(ctx, []string{taskID})
	require.Error(t, err, "cross-env cancellation should error")
	assert.Equal(t, 0, count)
	assert.Contains(t, err.Error(), "different environment")

	// The other-env task should not have been mutated.
	doc, err := client.Collection(CollectionOnceTasks).Doc(taskID).Get(ctx)
	require.NoError(t, err)
	var got OnceTask[testKind]
	require.NoError(t, doc.DataTo(&got))
	assert.False(t, got.IsCancelled, "cross-env task must remain non-cancelled")
	assert.Equal(t, otherEnv, got.Env)
}
