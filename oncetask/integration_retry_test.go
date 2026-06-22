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

// TestIntegration_RetryPolicies is a table-driven exercise of every
// built-in retry policy plus a custom one, asserting attempt counts, errors
// captured on the document, terminal state, and overall status.
func TestIntegration_RetryPolicies(t *testing.T) {
	type expect struct {
		minAttempts  int  // attempts >= this on completion
		maxAttempts  int  // attempts <= this on completion (0 = no upper bound)
		minErrors    int  // len(errors) >= this on completion
		maxErrors    int  // 0 = no upper bound
		terminalDone bool // task should be marked done (terminal)
	}
	//nolint:govet // fieldalignment: readability over packing for tests
	type tcase struct {
		name       string
		policy     HandlerOption
		alwaysFail bool
		failTimes  int // when alwaysFail=false, fail this many times then succeed
		expect     expect
	}

	cases := []tcase{
		{
			name:       "no_retry_fails_after_first_attempt",
			policy:     WithRetryPolicy(NoRetryPolicy{}),
			alwaysFail: true,
			expect: expect{
				minAttempts: 1, maxAttempts: 1,
				minErrors: 1, maxErrors: 1,
				terminalDone: true,
			},
		},
		{
			name: "fixed_delay_max_3_attempts_all_failing",
			policy: WithRetryPolicy(FixedDelayPolicy{
				MaxAttempts: 3,
				Delay:       100 * time.Millisecond,
			}),
			alwaysFail: true,
			expect: expect{
				minAttempts: 3, maxAttempts: 3,
				minErrors: 3, maxErrors: 3,
				terminalDone: true,
			},
		},
		{
			name: "exponential_backoff_succeeds_on_3rd_try",
			policy: WithRetryPolicy(ExponentialBackoffPolicy{
				MaxAttempts: 5,
				BaseDelay:   50 * time.Millisecond,
				MaxDelay:    500 * time.Millisecond,
				Multiplier:  2.0,
			}),
			alwaysFail: false,
			failTimes:  2,
			expect: expect{
				minAttempts: 3, maxAttempts: 3,
				minErrors: 2, maxErrors: 2,
				terminalDone: true,
			},
		},
		{
			name:       "with_no_retry_helper_disables_retries",
			policy:     WithNoRetry(),
			alwaysFail: true,
			expect: expect{
				minAttempts: 1, maxAttempts: 1,
				minErrors: 1, maxErrors: 1,
				terminalDone: true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			manager, _, cleanup := newTestManager[testKind](ctx, t)
			defer cleanup()

			kind := makeKind("retry")
			var attempts int32

			handler := NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
				n := atomic.AddInt32(&attempts, 1)
				if tc.alwaysFail {
					return fmt.Errorf("always fails (attempt %d)", n)
				}
				if int(n) <= tc.failTimes {
					return fmt.Errorf("transient (attempt %d)", n)
				}
				return nil
			})

			require.NoError(t, manager.RegisterTaskHandler(kind, handler,
				tc.policy,
				WithLeaseDuration(10*time.Second),
			))

			taskID := makeTaskID("retry")
			_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
			require.NoError(t, err)

			requireWait(t, 30*time.Second, func() bool {
				tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
				return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
			}, "task should reach a terminal state")

			tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
			require.NoError(t, err)
			require.Len(t, tasks, 1)
			task := tasks[0]

			if tc.expect.terminalDone {
				assert.NotEmpty(t, task.DoneAt, "task should be marked done")
			}
			assert.GreaterOrEqual(t, task.Attempts, tc.expect.minAttempts,
				"attempts >= minAttempts")
			if tc.expect.maxAttempts > 0 {
				assert.LessOrEqual(t, task.Attempts, tc.expect.maxAttempts,
					"attempts <= maxAttempts")
			}
			assert.GreaterOrEqual(t, len(task.Errors), tc.expect.minErrors,
				"errors >= minErrors")
			if tc.expect.maxErrors > 0 {
				assert.LessOrEqual(t, len(task.Errors), tc.expect.maxErrors,
					"errors <= maxErrors")
			}
		})
	}
}

// customRetryPolicy is used to verify that user-defined RetryPolicy
// implementations integrate correctly.
type customRetryPolicy struct {
	maxAttempts int
}

func (p customRetryPolicy) ShouldRetry(attempts int, err error) bool {
	if errors.Is(err, errFailFast) {
		return false
	}
	return attempts < p.maxAttempts
}

func (p customRetryPolicy) NextRetryDelay(attempts int, err error) time.Duration {
	return 50 * time.Millisecond
}

var errFailFast = errors.New("fail fast — do not retry")

// TestIntegration_CustomRetryPolicy verifies that a user policy is
// consulted both for ShouldRetry and NextRetryDelay.
func TestIntegration_CustomRetryPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	t.Run("does_not_retry_on_fail_fast_error", func(t *testing.T) {
		kind := makeKind("custom_ff")
		var attempts int32
		handler := NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
			atomic.AddInt32(&attempts, 1)
			return errFailFast
		})
		require.NoError(t, manager.RegisterTaskHandler(kind, handler,
			WithRetryPolicy(customRetryPolicy{maxAttempts: 5}),
			WithLeaseDuration(5*time.Second),
		))

		taskID := makeTaskID("ff")
		_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
		require.NoError(t, err)

		requireWait(t, 15*time.Second, func() bool {
			tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
			return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
		}, "task should reach terminal state quickly")

		assert.Equal(t, int32(1), atomic.LoadInt32(&attempts),
			"custom policy should refuse retry after fail-fast error")
	})

	t.Run("respects_max_attempts", func(t *testing.T) {
		kind := makeKind("custom_max")
		var attempts int32
		handler := NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
			atomic.AddInt32(&attempts, 1)
			return errors.New("transient")
		})
		require.NoError(t, manager.RegisterTaskHandler(kind, handler,
			WithRetryPolicy(customRetryPolicy{maxAttempts: 4}),
			WithLeaseDuration(5*time.Second),
		))

		taskID := makeTaskID("max")
		_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
		require.NoError(t, err)

		requireWait(t, 15*time.Second, func() bool {
			tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
			return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
		}, "task should reach terminal state")

		assert.Equal(t, int32(4), atomic.LoadInt32(&attempts), "should attempt exactly maxAttempts")

		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		require.NoError(t, err)
		assert.Len(t, tasks[0].Errors, 4, "should record one error per attempt")
	})
}

// TestIntegration_HandlerTimeout: handlers that exceed the lease duration
// are cancelled via the handler context (lease - 1s), the task fails, and
// the retry policy decides whether to retry.
func TestIntegration_HandlerTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_timeout")
	var attempts int32
	handler := NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&attempts, 1)
		select {
		case <-ctx.Done():
			return ctx.Err() // expect deadline exceeded
		case <-time.After(30 * time.Second):
			return nil
		}
	})
	require.NoError(t, manager.RegisterTaskHandler(kind, handler,
		// 2s lease → 1s handler budget. Plenty short to verify timeout.
		WithLeaseDuration(2*time.Second),
		WithRetryPolicy(FixedDelayPolicy{MaxAttempts: 2, Delay: 100 * time.Millisecond}),
	))

	taskID := makeTaskID("timeout")
	_, err := manager.CreateTask(ctx, testTaskData{Kind: kind, IDValue: taskID, Payload: "p"})
	require.NoError(t, err)

	requireWait(t, 25*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "task should reach terminal state via timeout")

	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	task := tasks[0]
	assert.NotEmpty(t, task.DoneAt)
	assert.Equal(t, 2, task.Attempts, "should attempt exactly MaxAttempts before failing")
	require.Len(t, task.Errors, 2)
	for _, e := range task.Errors {
		assert.Contains(t, e.Error, "deadline exceeded",
			"each error should reflect handler timeout")
	}
}

// TestIntegration_LeaseExpiryRecovery: a task with a leasedUntil
// timestamp in the past (simulating a worker that crashed mid-execution,
// leaving its lease behind) can still be claimed by a subsequent worker.
//
// We plant the stale-lease task directly via the raw client because the
// public CreateTask API never produces such a state — the simulation is
// the whole point.
func TestIntegration_LeaseExpiryRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, ourEnv, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("h_lease_expiry")

	taskID := makeTaskID("lease_recover")
	staleLease := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	planted := OnceTask[testKind]{
		Id:          taskID,
		Type:        kind,
		Env:         ourEnv,
		Data:        map[string]any{"payload": "p"},
		WaitUntil:   NoWait,
		LeasedUntil: staleLease, // expired lease
		CreatedAt:   time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		Attempts:    1, // a previous attempt acquired the lease then crashed
	}
	client := rawTestClient(ctx, t)
	_, err := client.Collection(CollectionOnceTasks).Doc(taskID).Create(ctx, planted)
	require.NoError(t, err)

	var ran int32
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&ran, 1)
		return nil
	}), WithLeaseDuration(10*time.Second)))

	// We didn't go through CreateTask, so evaluateNow wasn't fired. But
	// the worker's first poll on startup happens immediately and must
	// see the planted task as ready (waitUntil <= now AND
	// leasedUntil <= now).
	requireWait(t, 15*time.Second, func() bool {
		return atomic.LoadInt32(&ran) >= 1
	}, "worker should reclaim a task with an expired lease")

	requireWait(t, 10*time.Second, func() bool {
		tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
		return err == nil && len(tasks) == 1 && tasks[0].DoneAt != ""
	}, "reclaimed task should be marked done")

	tasks, err := manager.GetTasksByIds(ctx, []string{taskID})
	require.NoError(t, err)
	task := tasks[0]
	assert.NotEmpty(t, task.DoneAt)
	assert.Equal(t, 2, task.Attempts, "attempts should increment from the planted 1 to 2")
}
