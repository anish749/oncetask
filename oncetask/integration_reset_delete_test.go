//go:build integration

package oncetask

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Reset_TerminalOnly: only tasks in a terminal state
// (DoneAt set) get reset; pending/leased tasks are skipped.
func TestIntegration_Reset_TerminalOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("reset_terminal")

	// Task A: will be completed.
	completedID := makeTaskID("rst_completed")
	// Task B: will be left pending.
	pendingID := makeTaskID("rst_pending")
	// Task C: will be failed permanently.
	failedID := makeTaskID("rst_failed")

	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: completedID, Payload: "ok"})
	require.NoError(t, err)
	_, err = manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: pendingID, Payload: "skip"})
	require.NoError(t, err)
	_, err = manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: failedID, Payload: "fail"})
	require.NoError(t, err)

	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		switch task.Id {
		case completedID:
			return nil
		case failedID:
			return errors.New("permanent failure")
		case pendingID:
			// Block forever (or until cancelled). The lease will expire
			// and we'll see this task return to pending — but since we
			// never re-claim it (only one worker), it stays pending in
			// practice for this test's duration.
			//
			// Simpler: just return success to also mark this done… but
			// that defeats the test. So instead we use a separate task
			// type where there's NO handler at all for the pending one.
			return nil
		}
		return nil
	}),
		WithRetryPolicy(NoRetryPolicy{}),
		WithLeaseDuration(15*time.Second),
	))

	// Wait for completed and failed tasks to reach terminal state. Don't
	// rely on the pending task staying pending if the worker would also
	// pick it up — it would. So actually let's just delete the pending
	// task scenario from this test and rely on a fresh-create test for
	// "skipped because not terminal".
	_ = pendingID

	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{completedID, failedID})
		if err != nil || len(tasks) != 2 {
			return false
		}
		for _, task := range tasks {
			if task.DoneAt == "" {
				return false
			}
		}
		return true
	}, "completed and failed tasks should be done")

	// Add a fresh pending task that has no handler so it stays pending.
	freshPendingID := makeTaskID("rst_fresh_pending")
	freshKind := makeKind("reset_terminal_unhandled")
	_, err = manager.CreateTask(ctx, testTaskData{Kind: freshKind, IDValue: freshPendingID, Payload: "p"})
	require.NoError(t, err)

	// Reset all three terminal tasks plus the fresh-pending one. Expect
	// only completed and failed to actually reset.
	count, err := manager.ResetTasksByIds(ctx, []string{completedID, failedID, freshPendingID})
	require.NoError(t, err)
	assert.Equal(t, 2, count, "only terminal tasks should reset")

	// Verify: completed/failed are now pending; fresh-pending unchanged.
	tasks, err := manager.GetTasksByIds(ctx, []string{completedID, failedID, freshPendingID})
	require.NoError(t, err)
	require.Len(t, tasks, 3)

	byID := map[string]OnceTask[testKind]{}
	for _, task := range tasks {
		byID[task.Id] = task
	}
	for _, id := range []string{completedID, failedID} {
		task := byID[id]
		assert.Empty(t, task.DoneAt, "%s should have DoneAt cleared", id)
		assert.Empty(t, task.Errors, "%s should have Errors cleared", id)
		assert.Equal(t, 0, task.Attempts, "%s should have Attempts cleared", id)
		assert.Equal(t, NoWait, task.WaitUntil, "%s should be NoWait", id)
		assert.Nil(t, task.Result, "%s should have Result cleared", id)
		assert.False(t, task.IsCancelled, "%s should not be cancelled", id)
	}
	freshTask := byID[freshPendingID]
	assert.Empty(t, freshTask.DoneAt, "fresh pending task untouched")
	assert.Equal(t, 0, freshTask.Attempts)
}

// TestIntegration_Reset_RestartsExecution: a reset task is picked up and
// run again by the existing handler.
func TestIntegration_Reset_RestartsExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("reset_restart")
	var executions int32

	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&executions, 1)
		return nil
	}), WithLeaseDuration(10*time.Second)))

	taskID := makeTaskID("rst_restart")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
	require.NoError(t, err)

	requireWait(t, 10*time.Second, func() bool {
		return atomic.LoadInt32(&executions) >= 1
	}, "first execution should happen")
	requireWait(t, 10*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task should be done")

	// Reset.
	require.NoError(t, manager.ResetTask(ctx, taskID))

	requireWait(t, 10*time.Second, func() bool {
		return atomic.LoadInt32(&executions) >= 2
	}, "second execution should happen after reset")

	requireWait(t, 10*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task should be done again after re-execution")

	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	assert.Equal(t, 1, tasks[0].Attempts, "attempts after reset+reexec should be 1")
}

// TestIntegration_Reset_CrossEnvRejected: cannot reset tasks in a
// different environment.
func TestIntegration_Reset_CrossEnvRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, ourEnv, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("reset_xenv")
	taskID := makeTaskID("rst_xenv")
	otherTask := OnceTask[testKind]{
		Id:        taskID,
		Type:      kind,
		Env:       ourEnv + "_other",
		Data:      map[string]any{"payload": "p"},
		WaitUntil: NoWait,
		DoneAt:    time.Now().UTC().Format(time.RFC3339),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	client := rawTestClient(ctx, t)
	_, err := client.Collection(CollectionOnceTasks).Doc(taskID).Create(ctx, otherTask)
	require.NoError(t, err)

	count, err := manager.ResetTasksByIds(ctx, []string{taskID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different environment")
	assert.Equal(t, 0, count)
}

// TestIntegration_Delete: covers DeleteTask, DeleteTasksByIds, idempotency,
// and cross-env rejection in a single table.
func TestIntegration_Delete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, ourEnv, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("delete")

	t.Run("delete_existing_task", func(t *testing.T) {
		id := makeTaskID("del_exist")
		_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: id, Payload: "p"})
		require.NoError(t, err)

		require.NoError(t, manager.DeleteTask(ctx, id))

		tasks, err := manager.GetTasksByIds(ctx, []string{id})
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("delete_missing_task_is_idempotent", func(t *testing.T) {
		err := manager.DeleteTask(ctx, "does-not-exist")
		require.NoError(t, err, "deleting non-existent task should not error")
	})

	t.Run("bulk_delete_counts_only_deleted", func(t *testing.T) {
		ids := []string{makeTaskID("del_bulk"), makeTaskID("del_bulk"), makeTaskID("del_bulk")}
		for _, id := range ids {
			_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: id, Payload: "p"})
			require.NoError(t, err)
		}

		// Mix in a non-existent ID.
		count, err := manager.DeleteTasksByIds(ctx, append(ids, "missing-id"))
		require.NoError(t, err)
		assert.Equal(t, 3, count, "only existing tasks should count as deleted")

		tasks, err := manager.GetTasksByIds(ctx, ids)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})

	t.Run("cross_env_delete_rejected", func(t *testing.T) {
		id := makeTaskID("del_xenv")
		client := rawTestClient(ctx, t)
		other := OnceTask[testKind]{
			Id: id, Type: kind, Env: ourEnv + "_other",
			Data:      map[string]any{"payload": "p"},
			WaitUntil: NoWait,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		_, err := client.Collection(CollectionOnceTasks).Doc(id).Create(ctx, other)
		require.NoError(t, err)

		count, err := manager.DeleteTasksByIds(ctx, []string{id})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "different environment")
		assert.Equal(t, 0, count)

		// Verify still exists.
		doc, err := client.Collection(CollectionOnceTasks).Doc(id).Get(ctx)
		require.NoError(t, err)
		assert.True(t, doc.Exists())
	})
}
