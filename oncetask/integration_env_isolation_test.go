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

// TestIntegration_EnvIsolation_QueryReadFiltering: a manager scoped to env
// X cannot see tasks created in env Y, even though they're in the same
// Firestore collection.
func TestIntegration_EnvIsolation_QueryReadFiltering(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, ourEnv, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("env_iso")
	ourTaskID := makeTaskID("ours")
	otherTaskID := makeTaskID("other")

	// Plant a task in a different env directly.
	otherEnv := ourEnv + "_other"
	client := rawTestClient(ctx, t)
	otherTask := OnceTask[testKind]{
		Id: otherTaskID, Type: kind, Env: otherEnv,
		Data: map[string]any{"payload": "p"}, WaitUntil: NoWait,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_, err := client.Collection(CollectionOnceTasks).Doc(otherTaskID).Create(ctx, otherTask)
	require.NoError(t, err)

	// Plant a task in our env via the manager.
	_, err = manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: ourTaskID, Payload: "p"})
	require.NoError(t, err)

	t.Run("GetTasksByIds_filters_by_env", func(t *testing.T) {
		tasks, err := manager.GetTasksByIds(ctx, []string{ourTaskID, otherTaskID})
		require.NoError(t, err)
		require.Len(t, tasks, 1, "manager should only see its own env's task")
		assert.Equal(t, ourTaskID, tasks[0].Id)
	})

	t.Run("GetTasksByResourceKey_filters_by_env", func(t *testing.T) {
		// Re-plant cross-env tasks with same resource key
		rk := makeTaskID("rk_iso")

		ourRkID := makeTaskID("our_rk")
		otherRkID := makeTaskID("other_rk")

		_, err := manager.CreateTask(ctx, testTaskData{
			Kind: kind, IDValue: ourRkID, Payload: "p", ResourceKey: rk,
		})
		require.NoError(t, err)

		otherRkTask := OnceTask[testKind]{
			Id: otherRkID, Type: kind, Env: otherEnv, ResourceKey: rk,
			Data:      map[string]any{"payload": "p"},
			WaitUntil: NoWait,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		_, err = client.Collection(CollectionOnceTasks).Doc(otherRkID).Create(ctx, otherRkTask)
		require.NoError(t, err)

		tasks, err := manager.GetTasksByResourceKey(ctx, rk)
		require.NoError(t, err)
		require.Len(t, tasks, 1)
		assert.Equal(t, ourRkID, tasks[0].Id)
	})

	t.Run("GetPendingCount_only_counts_our_env", func(t *testing.T) {
		count, err := manager.GetPendingCount(ctx, kind)
		require.NoError(t, err)
		// Should not include the cross-env tasks. The exact value depends
		// on what's been created; we just assert it's not affected by
		// the planted other-env task.
		assert.GreaterOrEqual(t, count, 1, "should see our pending task at minimum")

		// Plant another other-env task and confirm count doesn't change.
		extra := OnceTask[testKind]{
			Id: makeTaskID("other_extra"), Type: kind, Env: otherEnv,
			Data:      map[string]any{"payload": "p"},
			WaitUntil: NoWait,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		_, err = client.Collection(CollectionOnceTasks).Doc(extra.Id).Create(ctx, extra)
		require.NoError(t, err)

		count2, err := manager.GetPendingCount(ctx, kind)
		require.NoError(t, err)
		assert.Equal(t, count, count2, "other-env tasks must not affect our pending count")
	})
}

// TestIntegration_EnvIsolation_HandlerSkipsCrossEnv: a worker subscribed
// to kind X in env A must not execute a kind X task that lives in env B.
func TestIntegration_EnvIsolation_HandlerSkipsCrossEnv(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, ourEnv, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("env_handler")

	var ran int32
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&ran, 1)
		return nil
	}), WithLeaseDuration(5*time.Second)))

	// Plant a task in a DIFFERENT env that's already due.
	client := rawTestClient(ctx, t)
	otherTaskID := makeTaskID("xenv_handler")
	otherTask := OnceTask[testKind]{
		Id: otherTaskID, Type: kind, Env: ourEnv + "_alien",
		Data: map[string]any{"payload": "p"}, WaitUntil: NoWait,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_, err := client.Collection(CollectionOnceTasks).Doc(otherTaskID).Create(ctx, otherTask)
	require.NoError(t, err)

	// Plant a task in OUR env to confirm the handler is alive.
	ourTaskID := makeTaskID("our_handler")
	_, err = manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: ourTaskID, Payload: "p"})
	require.NoError(t, err)

	requireWait(t, 10*time.Second, func() bool {
		return atomic.LoadInt32(&ran) >= 1
	}, "handler should fire on our-env task")

	// Wait a bit more, then assert the cross-env task is still pending
	// (NOT executed). Since the worker keeps polling, this is a
	// not-after-N-seconds guarantee; if our env-filter is broken the
	// counter would have already incremented past 1.
	time.Sleep(2 * time.Second)
	assert.Equal(t, int32(1), atomic.LoadInt32(&ran),
		"handler must not run cross-env tasks")

	// Confirm the cross-env doc still has no DoneAt.
	doc, err := client.Collection(CollectionOnceTasks).Doc(otherTaskID).Get(ctx)
	require.NoError(t, err)
	var got OnceTask[testKind]
	require.NoError(t, doc.DataTo(&got))
	assert.Empty(t, got.DoneAt, "cross-env task should remain untouched")
	assert.Empty(t, got.LeasedUntil)
}
