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

// TestSmoke_CreateAndExecute is the canary: if this fails, the rest of the
// integration suite is meaningless because the emulator/manager wiring is
// broken. Keep it minimal.
func TestSmoke_CreateAndExecute(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("smoke")

	var executions int32
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&executions, 1)
		return nil
	}), WithLeaseDuration(30*time.Second)))

	created, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: "smoke-1", Payload: "x"})
	require.NoError(t, err)
	assert.True(t, created)

	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{"smoke-1"})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task did not reach done state")

	assert.Equal(t, int32(1), atomic.LoadInt32(&executions), "handler should run exactly once")
	tasks, err := manager.GetTasksByIds(ctx, []string{"smoke-1"})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.NotEmpty(t, tasks[0].DoneAt)
	assert.Empty(t, tasks[0].Errors)
}
