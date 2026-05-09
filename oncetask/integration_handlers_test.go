//go:build integration

package oncetask

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_SingleHandler_Success: handler returns nil, task is marked
// done with the result stored on the document.
func TestIntegration_SingleHandler_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_success")

	require.NoError(t, manager.RegisterTaskHandler(kind, func(ctx context.Context, task *OnceTask[testKind]) (any, error) {
		return map[string]any{"status": "ok", "id": task.Id}, nil
	}, WithLeaseDuration(20*time.Second)))

	taskID := makeTaskID("success")
	created, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "data"})
	require.NoError(t, err)
	require.True(t, created)

	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task did not complete")

	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.NotEmpty(t, tasks[0].DoneAt)
	assert.Empty(t, tasks[0].Errors)
	require.NotNil(t, tasks[0].Result)
	resultMap, ok := tasks[0].Result.(map[string]any)
	require.True(t, ok, "result should be a map, got %T", tasks[0].Result)
	assert.Equal(t, "ok", resultMap["status"])
}

// TestIntegration_NoResultAdapter exercises the NoResult helper.
func TestIntegration_NoResultAdapter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_noresult")
	var ran int32
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&ran, 1)
		return nil
	}), WithLeaseDuration(20*time.Second)))

	taskID := makeTaskID("nores")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "x"})
	require.NoError(t, err)

	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task did not complete")
	assert.Equal(t, int32(1), atomic.LoadInt32(&ran))
}

// TestIntegration_DuplicateHandlerRegistration verifies ErrHandlerAlreadyExists.
func TestIntegration_DuplicateHandlerRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_dup")
	noop := NoResult(func(ctx context.Context, task *OnceTask[testKind]) error { return nil })

	require.NoError(t, manager.RegisterTaskHandler(kind, noop))

	t.Run("re-registering single handler errors", func(t *testing.T) {
		err := manager.RegisterTaskHandler(kind, noop)
		require.ErrorIs(t, err, ErrHandlerAlreadyExists)
	})

	t.Run("registering resource-key handler on same kind errors", func(t *testing.T) {
		err := manager.RegisterResourceKeyHandler(kind, NoResultResourceKey(func(ctx context.Context, tasks []OnceTask[testKind]) error { return nil }))
		require.ErrorIs(t, err, ErrHandlerAlreadyExists)
	})
}

// TestIntegration_ResourceKey_BatchedHandler covers the AllPerResourceKey
// strategy: all pending tasks with the same resource key are claimed and
// passed to the handler in a single invocation.
func TestIntegration_ResourceKey_BatchedHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_batched")
	resourceKey := fmt.Sprintf("rk_%d", uniqueSuffix())

	type call struct {
		ids  []string
		size int
	}
	var (
		mu    sync.Mutex
		calls []call
	)

	// Create the tasks BEFORE registering the handler. If we register
	// first, the worker goroutine starts polling immediately and the
	// emulator gets into a state where active read-transactions block our
	// CreateTask writes with "Transaction lock timeout". Doing creates
	// first lets the worker pick up an existing batch on its first poll.
	taskIDs := make([]string, 4)
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("batched_%d_%d", uniqueSuffix(), i)
		taskIDs[i] = id
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind: kind, IDValue: id, Payload: "p", ResourceKey: resourceKey,
		})
		require.NoError(t, err)
	}

	require.NoError(t, manager.RegisterResourceKeyHandler(kind, NoResultResourceKey(func(ctx context.Context, tasks []OnceTask[testKind]) error {
		ids := make([]string, len(tasks))
		for i, task := range tasks {
			ids[i] = task.Id
		}
		mu.Lock()
		calls = append(calls, call{size: len(tasks), ids: ids})
		mu.Unlock()
		return nil
	}), WithLeaseDuration(15*time.Second)))

	requireWait(t, 20*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, taskIDs)
		if err != nil || len(tasks) != 4 {
			return false
		}
		for _, task := range tasks {
			if task.DoneAt == "" {
				return false
			}
		}
		return true
	}, "all 4 tasks should reach done")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, calls)
	// Total task IDs across all calls must equal 4. Number of calls is at
	// most 4 — typically 1 if all created before the worker claimed,
	// possibly more if the first claim happens before the rest are created.
	seen := make(map[string]struct{})
	for _, c := range calls {
		for _, id := range c.ids {
			seen[id] = struct{}{}
		}
	}
	assert.Len(t, seen, 4, "every task should be handled exactly once")
}

// TestIntegration_ResourceKey_MutualExclusion: when a single-task handler
// is configured but tasks have a resource key, only one runs at a time per
// key. We launch many concurrent tasks under one key with a slow handler
// and verify max-in-flight stays at 1.
func TestIntegration_ResourceKey_MutualExclusion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_mutex")
	resourceKey := fmt.Sprintf("rk_%d", uniqueSuffix())

	var (
		inFlight    int32
		maxInFlight int32
	)

	const totalTasks = 6
	taskIDs := make([]string, totalTasks)
	for i := 0; i < totalTasks; i++ {
		id := fmt.Sprintf("mutex_%d_%d", uniqueSuffix(), i)
		taskIDs[i] = id
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind: kind, IDValue: id, Payload: "p", ResourceKey: resourceKey,
		})
		require.NoError(t, err)
	}

	// Register handler AFTER creates: starting 4 polling workers before
	// the writes complete causes lock-timeout failures on the emulator.
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, cur) {
				break
			}
		}
		time.Sleep(300 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	}), WithLeaseDuration(15*time.Second), WithConcurrency(4)))

	requireWait(t, 60*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, taskIDs)
		if err != nil || len(tasks) != totalTasks {
			return false
		}
		for _, task := range tasks {
			if task.DoneAt == "" {
				return false
			}
		}
		return true
	}, "all tasks should complete")

	assert.Equal(t, int32(1), atomic.LoadInt32(&maxInFlight),
		"resource-key serialization should ensure only one task runs at a time per key")
}

// TestIntegration_Concurrent_NoResourceKey: tasks without a resource key
// run concurrently up to the configured concurrency.
func TestIntegration_Concurrent_NoResourceKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_concurrent")

	const concurrency = 3
	var (
		inFlight    int32
		maxInFlight int32
	)

	// Create more tasks than the concurrency so the workers actually overlap.
	const totalTasks = 6
	taskIDs := make([]string, totalTasks)
	for i := 0; i < totalTasks; i++ {
		id := fmt.Sprintf("concurrent_%d_%d", uniqueSuffix(), i)
		taskIDs[i] = id
		_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: id, Payload: "p"})
		require.NoError(t, err)
	}

	// Register after creates so multiple polling workers don't lock the
	// collection against our writes.
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, cur) {
				break
			}
		}
		time.Sleep(400 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	}), WithLeaseDuration(15*time.Second), WithConcurrency(concurrency)))

	requireWait(t, 30*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, taskIDs)
		if err != nil || len(tasks) != totalTasks {
			return false
		}
		for _, task := range tasks {
			if task.DoneAt == "" {
				return false
			}
		}
		return true
	}, "all tasks should complete")

	got := atomic.LoadInt32(&maxInFlight)
	assert.Greater(t, got, int32(1), "with no resource key, multiple tasks should run in parallel")
	assert.LessOrEqual(t, got, int32(concurrency), "concurrency cap should be respected")
}

// TestIntegration_ResourceKeyHandler_OrderedByWaitUntil: tasks delivered
// to a resource-key handler are sorted by waitUntil ascending.
func TestIntegration_ResourceKeyHandler_OrderedByWaitUntil(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_order")
	resourceKey := fmt.Sprintf("rk_%d", uniqueSuffix())

	var (
		mu       sync.Mutex
		gotOrder []string
	)

	// Create tasks with distinct waitUntil values, then sleep until all of
	// them are due, THEN register the handler. The worker's first poll
	// after registration claims everything in one batch ordered by
	// waitUntil ascending. Doing it this way avoids the 1-minute polling
	// interval the worker falls back to once it sees an empty batch.
	now := time.Now()
	wantOrder := []string{
		fmt.Sprintf("ord_%d_a", uniqueSuffix()),
		fmt.Sprintf("ord_%d_b", uniqueSuffix()),
		fmt.Sprintf("ord_%d_c", uniqueSuffix()),
	}
	schedules := []time.Time{
		now.Add(500 * time.Millisecond),
		now.Add(900 * time.Millisecond),
		now.Add(1300 * time.Millisecond),
	}

	for i, id := range wantOrder {
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind: kind, IDValue: id, Payload: "p", ResourceKey: resourceKey,
			ScheduleAt: schedules[i],
		})
		require.NoError(t, err)
	}

	// Wait until every task is past its scheduled time before starting
	// the worker. Otherwise the first claim is empty, the worker sleeps
	// on its 1-minute ticker, and nothing wakes it because evaluateChan
	// only fires on creates.
	time.Sleep(1500 * time.Millisecond)

	require.NoError(t, manager.RegisterResourceKeyHandler(kind, NoResultResourceKey(func(ctx context.Context, tasks []OnceTask[testKind]) error {
		mu.Lock()
		for _, task := range tasks {
			gotOrder = append(gotOrder, task.Id)
		}
		mu.Unlock()
		return nil
	}), WithLeaseDuration(20*time.Second)))

	requireWait(t, 60*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, wantOrder)
		if err != nil || len(tasks) != len(wantOrder) {
			return false
		}
		for _, task := range tasks {
			if task.DoneAt == "" {
				return false
			}
		}
		return true
	}, "all tasks should complete")

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, wantOrder, gotOrder, "resource-key handler should see tasks ordered by waitUntil")
}

// TestIntegration_ScheduledTask_FutureExecution covers ScheduledTask:
//   - a task whose scheduled time is in the future is NOT run early
//   - a task whose scheduled time has already passed IS run on the next poll
//
// We deliberately split these two assertions instead of waiting for a
// future-scheduled task to become due in-test: the worker's polling ticker
// is 1 minute, so a task scheduled <60s out can take up to ~60s to be
// claimed. Splitting keeps the test fast and the assertions clean.
func TestIntegration_ScheduledTask_FutureExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_scheduled")
	var (
		mu     sync.Mutex
		ranIDs = map[string]time.Time{}
	)
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		mu.Lock()
		ranIDs[task.Id] = time.Now()
		mu.Unlock()
		return nil
	}), WithLeaseDuration(15*time.Second)))

	farFutureID := makeTaskID("sched_future")
	pastID := makeTaskID("sched_past")

	// Task A: scheduled 30s out — must not run within the test's wait
	// window (and the worker's first ticker would only fire 60s after
	// startup, so we have plenty of margin).
	_, err := manager.CreateTask(ctx, testTaskData{
		Kind: kind, IDValue: farFutureID, Payload: "p",
		ScheduleAt: time.Now().Add(30 * time.Second),
	})
	require.NoError(t, err)

	// Task B: scheduled 1s in the past — already due. The worker's
	// evaluateChan signal from this CreateTask wakes it up and the
	// upcoming poll will claim it.
	_, err = manager.CreateTask(ctx, testTaskData{
		Kind: kind, IDValue: pastID, Payload: "p",
		ScheduleAt: time.Now().Add(-1 * time.Second),
	})
	require.NoError(t, err)

	// Past-scheduled task should run quickly.
	requireWait(t, 15*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		_, ok := ranIDs[pastID]
		return ok
	}, "past-scheduled task should run")

	// Future-scheduled task should not have run.
	mu.Lock()
	_, futureRan := ranIDs[farFutureID]
	mu.Unlock()
	assert.False(t, futureRan, "future-scheduled task must not run before its scheduleAt")

	// And the past-scheduled task is marked done in Firestore.
	requireWait(t, 15*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{pastID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "past-scheduled task should be done")
}

// TestIntegration_ResourceKeyHandler_NoKeyTreatedAsSingle: per docs, a
// resource-key handler with an empty key processes one task at a time
// (single-task semantics for that one task).
func TestIntegration_ResourceKeyHandler_NoKeyTreatedAsSingle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_rk_nokey")

	type record struct {
		size int
	}
	var (
		mu      sync.Mutex
		records []record
	)

	const count = 3
	taskIDs := make([]string, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("rk_nokey_%d_%d", uniqueSuffix(), i)
		taskIDs[i] = id
		_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: id, Payload: "p"})
		require.NoError(t, err)
	}

	require.NoError(t, manager.RegisterResourceKeyHandler(kind, NoResultResourceKey(func(ctx context.Context, tasks []OnceTask[testKind]) error {
		mu.Lock()
		records = append(records, record{size: len(tasks)})
		mu.Unlock()
		return nil
	}), WithLeaseDuration(15*time.Second)))

	requireWait(t, 30*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, taskIDs)
		if err != nil || len(tasks) != count {
			return false
		}
		for _, task := range tasks {
			if task.DoneAt == "" {
				return false
			}
		}
		return true
	}, "all tasks should complete")

	mu.Lock()
	defer mu.Unlock()
	// Each call should have size exactly 1 (no resource key, no batching).
	for _, r := range records {
		assert.Equal(t, 1, r.size, "without a resource key, each call should hold one task")
	}
	// Sanity: total tasks delivered matches creation count.
	total := 0
	for _, r := range records {
		total += r.size
	}
	assert.Equal(t, count, total)
	_ = sort.IntSlice{}
}
