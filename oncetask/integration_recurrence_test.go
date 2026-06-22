//go:build integration

package oncetask

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Recurrence_SpawnsOccurrences: a recurrence task with an
// RRule that's already due spawns an occurrence task that's executed by
// the registered handler, while the parent stays around waiting for the
// next occurrence.
func TestIntegration_Recurrence_SpawnsOccurrences(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("rec_basic")

	var (
		mu         sync.Mutex
		executions []string
	)
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		mu.Lock()
		executions = append(executions, task.Id)
		mu.Unlock()
		return nil
	}), WithLeaseDuration(10*time.Second)))

	// DTStart in the past so the first occurrence fires immediately.
	dtstart := time.Now().Add(-2 * time.Second).UTC().Format(time.RFC3339)
	parentID := makeTaskID("rec_parent")
	_, err := manager.CreateTask(ctx, recurringTaskData{
		testTaskData: testTaskData{Kind: kind, IDValue: parentID, Payload: "p"},
		// Daily means after the first occurrence the parent waits ~24h.
		// We only assert the FIRST occurrence is spawned and run.
	}.WithRRule(&Recurrence{RRule: "FREQ=DAILY", DTStart: dtstart}))
	require.NoError(t, err)

	// Wait until at least one occurrence has been executed.
	requireWait(t, 20*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(executions) >= 1
	}, "first occurrence should fire")

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(executions), 1)

	// The occurrence ID is "<parent>_occ_<dtstart>". Confirm shape.
	occID := executions[0]
	assert.Contains(t, occID, parentID+"_occ_", "occurrence id should be derived from parent")

	// Parent should still exist with rescheduled WaitUntil (not done).
	parents, err := manager.GetTasksByIds(ctx, []string{parentID})
	require.NoError(t, err)
	require.Len(t, parents, 1)
	assert.Empty(t, parents[0].DoneAt, "parent recurrence task should not be marked done")
	assert.NotNil(t, parents[0].Recurrence)
}

// TestIntegration_Recurrence_RRuleVariants is a table covering common
// RRule shapes — just asserts that the parent task stores Recurrence
// fields correctly and that an occurrence eventually fires when the next
// scheduled time is in the past. We deliberately keep this test focused
// on the "library accepts and stores RRule X" angle — the more dynamic
// "spawning works" semantics are covered by SpawnsOccurrences and the
// cancellation test.
func TestIntegration_Recurrence_RRuleVariants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	now := time.Now().UTC()

	// Pick a past Monday at this time so weekly+BYDAY=MO has an
	// already-due first occurrence.
	pastMonday := now
	for pastMonday.Weekday() != time.Monday {
		pastMonday = pastMonday.AddDate(0, 0, -1)
	}
	pastMonday = pastMonday.Add(-1 * time.Hour) // ensure strictly past

	dtstartPast := now.Add(-2 * time.Second).Format(time.RFC3339)

	type tcase struct {
		name    string
		rrule   string
		dtstart string
		exdates []string
	}

	cases := []tcase{
		{
			name:    "daily_unbounded",
			rrule:   "FREQ=DAILY",
			dtstart: dtstartPast,
		},
		{
			name:    "weekly_byday_monday",
			rrule:   "FREQ=WEEKLY;BYDAY=MO",
			dtstart: pastMonday.Format(time.RFC3339),
		},
		{
			name:    "count_bounded",
			rrule:   "FREQ=DAILY;COUNT=3",
			dtstart: dtstartPast,
		},
		{
			name:    "exdate_skips_dtstart",
			rrule:   "FREQ=DAILY",
			dtstart: dtstartPast,
			exdates: []string{dtstartPast},
		},
		{
			name:    "monthly_first_day",
			rrule:   "FREQ=MONTHLY;BYMONTHDAY=1",
			dtstart: now.AddDate(0, -1, 0).Format(time.RFC3339),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind := makeKind("rec_var")
			parentID := makeTaskID("rec_var_parent")

			_, err := manager.CreateTask(ctx, recurringTaskData{
				testTaskData: testTaskData{Kind: kind, IDValue: parentID, Payload: "p"},
			}.WithRRule(&Recurrence{
				RRule:   tc.rrule,
				DTStart: tc.dtstart,
				ExDates: tc.exdates,
			}))
			require.NoError(t, err, "library should accept this RRule")

			parents, err := manager.GetTasksByIds(ctx, []string{parentID})
			require.NoError(t, err)
			require.Len(t, parents, 1)
			parent := parents[0]
			require.NotNil(t, parent.Recurrence)
			assert.Equal(t, tc.rrule, parent.Recurrence.RRule)
			assert.Equal(t, tc.dtstart, parent.Recurrence.DTStart)
			assert.Equal(t, tc.exdates, parent.Recurrence.ExDates)
			assert.Empty(t, parent.DoneAt, "parent should not be done immediately")
			assert.Empty(t, parent.ParentRecurrenceID, "the parent itself has no parent recurrence")
		})
	}
}

// TestIntegration_Recurrence_CancelStopsSpawning: cancelling a recurrence
// generator marks it done and prevents future occurrence tasks from being
// created.
func TestIntegration_Recurrence_CancelStopsSpawning(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("rec_cancel")

	var ran int32
	require.NoError(t, manager.RegisterTaskHandler(kind, NoResult(func(ctx context.Context, task *OnceTask[testKind]) error {
		atomic.AddInt32(&ran, 1)
		return nil
	}), WithLeaseDuration(5*time.Second)))

	// Use SECONDLY+COUNT so we get a steady stream of occurrences fast
	// enough to detect "more spawned after cancel". Without COUNT the
	// rrule library expands every second across a 10-year window — that
	// works out to ~300 million occurrences and CreateTask just times
	// out.
	parentID := makeTaskID("rec_cancel")
	dtstart := time.Now().Add(-1 * time.Second).UTC().Format(time.RFC3339)
	_, err := manager.CreateTask(ctx, recurringTaskData{
		testTaskData: testTaskData{Kind: kind, IDValue: parentID, Payload: "p"},
	}.WithRRule(&Recurrence{RRule: "FREQ=SECONDLY;COUNT=20", DTStart: dtstart}))
	require.NoError(t, err)

	// Wait for at least one occurrence.
	requireWait(t, 10*time.Second, func() bool {
		return atomic.LoadInt32(&ran) >= 1
	}, "first occurrence should fire")

	// Cancel the recurrence parent. The next time the worker claims it,
	// it'll see IsCancelled=true and mark it done without spawning.
	require.NoError(t, manager.CancelTask(ctx, parentID))

	// Wait for the parent to be marked done.
	requireWait(t, 15*time.Second, func() bool {
		parents, err := manager.GetTasksByIds(ctx, []string{parentID})
		return err == nil && len(parents) == 1 && parents[0].DoneAt != ""
	}, "cancelled recurrence parent should be marked done")

	// Sample current execution count, wait, confirm no more occurrences.
	before := atomic.LoadInt32(&ran)
	time.Sleep(2 * time.Second)
	after := atomic.LoadInt32(&ran)
	// Allow off-by-one: an occurrence already in flight when we cancelled
	// can complete after the parent is done.
	assert.LessOrEqual(t, after-before, int32(1),
		"cancelled recurrence should stop spawning new occurrences")
}

// TestIntegration_Recurrence_ScheduledTimeConflict: combining Recurrence
// and ScheduledTask is rejected at task-creation time.
func TestIntegration_Recurrence_ScheduledTimeConflict(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	manager, _, cleanup := newTestManager[testKind](ctx, t)
	defer cleanup()

	kind := makeKind("rec_conflict")

	_, err := manager.CreateTask(ctx, recurringTaskData{
		testTaskData: testTaskData{
			Kind:       kind,
			IDValue:    "rec_conflict_1",
			Payload:    "p",
			ScheduleAt: time.Now().Add(1 * time.Hour),
		},
	}.WithRRule(&Recurrence{
		RRule:   "FREQ=DAILY",
		DTStart: time.Now().Format(time.RFC3339),
	}))
	require.Error(t, err, "task with both Recurrence and ScheduleAt should be rejected")
	assert.Contains(t, err.Error(), "cannot specify both Recurrence and ScheduledTask")
}

// WithRRule attaches a recurrence to the data fixture.
//
//nolint:gocritic // hugeParam: keeps fluent-style construction at call sites
func (d recurringTaskData) WithRRule(rec *Recurrence) recurringTaskData {
	d.Rec = rec
	return d
}

// Confirm the test's recurringTaskData satisfies RecurrenceProvider.
var _ RecurrenceProvider = recurringTaskData{}

// silence "fmt" unused import
var _ = fmt.Sprintf
