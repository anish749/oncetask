//go:build integration

package oncetask

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CreateTask exercises every documented behaviour of
// Manager.CreateTask, including idempotency on duplicate IDs.
func TestIntegration_CreateTask(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	manager, envName, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("crud_create")

	//nolint:govet // fieldalignment: readability over packing for tests
	type tcase struct {
		name       string
		first      testTaskData
		second     testTaskData // optional duplicate; zero value = skip
		wantFirst  bool
		wantSecond bool
	}
	cases := []tcase{
		{
			name:      "first create returns true",
			first:     testTaskData{Kind: kind, IDValue: makeTaskID("first"), Payload: "a"},
			wantFirst: true,
		},
		{
			name:       "duplicate ID returns false (idempotent)",
			first:      testTaskData{Kind: kind, IDValue: "duplicate-id", Payload: "a"},
			second:     testTaskData{Kind: kind, IDValue: "duplicate-id", Payload: "different-payload"},
			wantFirst:  true,
			wantSecond: false,
		},
		{
			name:      "deterministic ID via GenerateIdempotentID",
			first:     testTaskData{Kind: kind, Payload: "stable-payload"},
			wantFirst: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := manager.CreateTask(ctx, tc.first)
			require.NoError(t, err)
			assert.Equal(t, tc.wantFirst, created)

			if tc.second.Kind == "" {
				return
			}
			created2, err := manager.CreateTask(ctx, tc.second)
			require.NoError(t, err)
			assert.Equal(t, tc.wantSecond, created2)

			// Confirm the original payload survived the duplicate attempt.
			tasks, err := manager.GetTasksByIds(ctx, []string{tc.first.IDValue})
			require.NoError(t, err)
			require.Len(t, tasks, 1)
			assert.Equal(t, "a", tasks[0].Data["payload"])
			assert.Equal(t, envName, tasks[0].Env)
		})
	}
}

// TestIntegration_CreateTasks covers bulk creation: partial success with
// AlreadyExists, idempotency on retry, and end-to-end success.
func TestIntegration_CreateTasks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("crud_bulk")

	t.Run("creates all when none exist", func(t *testing.T) {
		batch := []Data[testKind]{
			testTaskData{Kind: kind, IDValue: "bulk-a-1", Payload: "a"},
			testTaskData{Kind: kind, IDValue: "bulk-a-2", Payload: "b"},
			testTaskData{Kind: kind, IDValue: "bulk-a-3", Payload: "c"},
		}
		created, err := manager.CreateTasks(ctx, batch)
		require.NoError(t, err)
		assert.Equal(t, 3, created)

		tasks, err := manager.GetTasksByIds(ctx, []string{"bulk-a-1", "bulk-a-2", "bulk-a-3"})
		require.NoError(t, err)
		assert.Len(t, tasks, 3)
	})

	t.Run("partial overlap silently skips already-existing", func(t *testing.T) {
		// Pre-create two
		_, err := manager.CreateTasks(ctx, []Data[testKind]{
			testTaskData{Kind: kind, IDValue: "bulk-b-1", Payload: "x"},
			testTaskData{Kind: kind, IDValue: "bulk-b-2", Payload: "y"},
		})
		require.NoError(t, err)

		// Re-submit with overlap: 2 existing + 2 new
		batch := []Data[testKind]{
			testTaskData{Kind: kind, IDValue: "bulk-b-1", Payload: "x"}, // exists
			testTaskData{Kind: kind, IDValue: "bulk-b-2", Payload: "y"}, // exists
			testTaskData{Kind: kind, IDValue: "bulk-b-3", Payload: "z"}, // new
			testTaskData{Kind: kind, IDValue: "bulk-b-4", Payload: "w"}, // new
		}
		created, err := manager.CreateTasks(ctx, batch)
		require.NoError(t, err, "AlreadyExists is silently treated as success per CreateTasks contract")
		assert.Equal(t, 2, created, "only the two genuinely new tasks count as created")
	})

	t.Run("empty input is a no-op", func(t *testing.T) {
		created, err := manager.CreateTasks(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, 0, created)
	})
}

// TestIntegration_GetTasksByResourceKey covers query-by-resource semantics.
func TestIntegration_GetTasksByResourceKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("crud_byrk")
	resourceKey := fmt.Sprintf("rk_%d", uniqueSuffix())

	// Insert: 3 tasks share the key, 2 don't.
	for i := 0; i < 3; i++ {
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind:        kind,
			IDValue:     fmt.Sprintf("rk_match_%d", i),
			Payload:     "p",
			ResourceKey: resourceKey,
		})
		require.NoError(t, err)
	}
	for i := 0; i < 2; i++ {
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind:        kind,
			IDValue:     fmt.Sprintf("rk_other_%d", i),
			Payload:     "p",
			ResourceKey: "other",
		})
		require.NoError(t, err)
	}

	tasks, err := manager.GetTasksByResourceKey(ctx, resourceKey)
	require.NoError(t, err)
	assert.Len(t, tasks, 3)
	for _, task := range tasks {
		assert.Equal(t, resourceKey, task.ResourceKey)
	}

	none, err := manager.GetTasksByResourceKey(ctx, "nonexistent_resource_key")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// TestIntegration_GetTasksByIds: existing tasks returned, missing skipped, no error.
func TestIntegration_GetTasksByIds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("crud_byids")
	for i := 0; i < 3; i++ {
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind:    kind,
			IDValue: fmt.Sprintf("byid_%d", i),
			Payload: "p",
		})
		require.NoError(t, err)
	}

	t.Run("returns existing", func(t *testing.T) {
		tasks, err := manager.GetTasksByIds(ctx, []string{"byid_0", "byid_1", "byid_2"})
		require.NoError(t, err)
		ids := make([]string, len(tasks))
		for i, task := range tasks {
			ids[i] = task.Id
		}
		sort.Strings(ids)
		assert.Equal(t, []string{"byid_0", "byid_1", "byid_2"}, ids)
	})

	t.Run("skips missing without error", func(t *testing.T) {
		tasks, err := manager.GetTasksByIds(ctx, []string{"byid_0", "missing_xyz"})
		require.NoError(t, err)
		assert.Len(t, tasks, 1)
		assert.Equal(t, "byid_0", tasks[0].Id)
	})

	t.Run("empty input is a no-op", func(t *testing.T) {
		tasks, err := manager.GetTasksByIds(ctx, nil)
		require.NoError(t, err)
		assert.Empty(t, tasks)
	})
}

// TestIntegration_GetPendingCount uses the COUNT aggregation. Pending means
// not done, waitUntil <= now, leasedUntil <= now.
func TestIntegration_GetPendingCount(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("crud_count")
	otherKind := makeKind("crud_count_other")

	t.Run("empty starts at zero", func(t *testing.T) {
		count, err := manager.GetPendingCount(ctx, kind)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("counts pending tasks of the queried type only", func(t *testing.T) {
		for i := 0; i < 4; i++ {
			_, err := manager.CreateTask(ctx, testTaskData{
				Kind:    kind,
				IDValue: fmt.Sprintf("count_%d", i),
				Payload: "p",
			})
			require.NoError(t, err)
		}
		// One of the other kind — must not pollute the count.
		_, err := manager.CreateTask(ctx, testTaskData{Kind: otherKind, IDValue: "count_other", Payload: "p"})
		require.NoError(t, err)

		count, err := manager.GetPendingCount(ctx, kind)
		require.NoError(t, err)
		assert.Equal(t, 4, count)
	})

	t.Run("does not count future-scheduled tasks", func(t *testing.T) {
		_, err := manager.CreateTask(ctx, testTaskData{
			Kind:       kind,
			IDValue:    "count_future",
			Payload:    "p",
			ScheduleAt: time.Now().Add(1 * time.Hour),
		})
		require.NoError(t, err)

		count, err := manager.GetPendingCount(ctx, kind)
		require.NoError(t, err)
		assert.Equal(t, 4, count, "future-scheduled task should not count as pending")
	})
}
