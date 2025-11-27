package oncetask

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/teambition/rrule-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// filterAndSpawnOccurrences handles recurrence tasks by spawning occurrences and rescheduling.
// Returns the non-recurrence tasks that need handler execution.
//
// Recurrence task cancellation behavior:
//   - If a recurrence task is cancelled (isCancelled=true), we stop spawning new occurrences
//     and mark the recurrence task as done. This prevents future occurrences from being created.
//   - Already-spawned occurrence tasks are independent and continue normally. They can be
//     cancelled individually like any other task if needed.
func (m *firestoreOnceTaskManager[TaskKind]) filterAndSpawnOccurrences(
	ctx context.Context,
	tasks []OnceTask[TaskKind],
) []OnceTask[TaskKind] {
	var executableTasks []OnceTask[TaskKind]
	now := time.Now().UTC()

	for _, task := range tasks {
		if task.Recurrence == nil {
			executableTasks = append(executableTasks, task)
			continue
		}

		// Check if recurrence task is cancelled
		if task.IsCancelled {
			// Stop spawning new occurrences - mark the recurrence task as done
			if err := m.markRecurrenceTaskDone(ctx, task); err != nil {
				slog.ErrorContext(ctx, "Failed to mark cancelled recurrence task as done", "error", err, "taskId", task.Id)
			}
			continue
		}

		// Recurrence task: spawn occurrence and reschedule
		if err := m.spawnOccurrence(ctx, &task, now); err != nil {
			slog.ErrorContext(ctx, "Failed to process recurrence task", "taskId", task.Id, "error", err)
		}
	}

	return executableTasks
}

// markRecurrenceTaskDone marks a recurrence task as done.
// Used when a recurrence task is cancelled to prevent further occurrences from being spawned.
func (m *firestoreOnceTaskManager[TaskKind]) markRecurrenceTaskDone(
	ctx context.Context,
	task OnceTask[TaskKind],
) error {
	docRef := m.queryBuilder.doc(task.Id)
	now := time.Now().UTC()

	updates := buildTaskDoneUpdates(now)

	_, err := docRef.Update(ctx, updates)
	return err
}

// spawnOccurrence creates an occurrence task and reschedules the recurrence task.
// Both operations are idempotent - safe to repeat if interrupted.
// Triggers immediate evaluation so the occurrence gets picked up right away.
func (m *firestoreOnceTaskManager[TaskKind]) spawnOccurrence(
	ctx context.Context,
	recurrenceTask *OnceTask[TaskKind],
	now time.Time,
) error {
	occurrenceTask, parentUpdates, err := prepareOccurrence(recurrenceTask, now)
	if err != nil {
		return fmt.Errorf("failed to process recurrence: %w", err)
	}

	// Create the occurrence task (idempotent - ignore if already exists)
	if occurrenceTask != nil {
		occurrenceDoc := m.queryBuilder.doc(occurrenceTask.Id)
		if _, err := occurrenceDoc.Create(ctx, occurrenceTask); err != nil {
			if status.Code(err) != codes.AlreadyExists {
				return fmt.Errorf("failed to create occurrence task: %w", err)
			}
		}
		// Trigger immediate evaluation so occurrence gets processed right away
		m.evaluateNow(recurrenceTask.Type)
	}

	// Update the recurrence task (reschedule or mark done)
	parentDoc := m.queryBuilder.doc(recurrenceTask.Id)
	if _, err := parentDoc.Update(ctx, parentUpdates); err != nil {
		return fmt.Errorf("failed to update recurrence task: %w", err)
	}

	return nil
}

// prepareOccurrence calculates what needs to happen for a recurrence task:
// 1. Creates an occurrence task for the current scheduled time
// 2. Determines updates to reschedule the recurrence task (or mark it done if exhausted)
//
// Returns:
//   - occurrenceTask: The occurrence task to create (nil if exhausted)
//   - parentUpdates: Updates to apply to the recurrence task
//   - error: If RRULE parsing fails
func prepareOccurrence[TaskKind ~string](
	parent *OnceTask[TaskKind],
	now time.Time,
) (*OnceTask[TaskKind], []firestore.Update, error) {
	if parent.Recurrence == nil {
		return nil, nil, fmt.Errorf("task %s has no recurrence config", parent.Id)
	}

	// Create occurrence task for the current scheduled time (parent.WaitUntil)
	occurrenceTask := createOccurrenceTask(parent, now)

	// Calculate next occurrence for the parent
	nextOccurrence, exhausted, err := calculateNextOccurrence(parent.Recurrence, now)
	if err != nil {
		return nil, nil, err
	}

	var parentUpdates []firestore.Update
	if exhausted {
		// No more occurrences - mark parent as done
		parentUpdates = buildTaskDoneUpdates(now)
	} else {
		// Schedule parent for next occurrence
		parentUpdates = []firestore.Update{
			{Path: "leasedUntil", Value: ""},
			{Path: "waitUntil", Value: nextOccurrence.Format(time.RFC3339)},
		}
	}

	return occurrenceTask, parentUpdates, nil
}

// createOccurrenceTask creates a new occurrence task from a recurrence task.
// Occurrence tasks are independent one-time tasks - they inherit the parent's data
// but not its cancellation status. Each occurrence can be cancelled individually.
func createOccurrenceTask[TaskKind ~string](parent *OnceTask[TaskKind], now time.Time) *OnceTask[TaskKind] {
	occurrenceTimestamp := parent.WaitUntil
	occurrenceID := fmt.Sprintf("%s_occ_%s", parent.Id, occurrenceTimestamp)

	return &OnceTask[TaskKind]{
		Id:                  occurrenceID,
		Type:                parent.Type,
		Data:                parent.Data,
		ResourceKey:         parent.ResourceKey,
		Env:                 parent.Env,
		WaitUntil:           occurrenceTimestamp,
		LeasedUntil:         "",
		CreatedAt:           now.Format(time.RFC3339),
		DoneAt:              "",
		Attempts:            0,
		Errors:              nil,
		Result:              nil,
		Recurrence:          nil,                 // Occurrence tasks are one-time tasks
		ParentRecurrenceID:  parent.Id,           // Links back to generator
		OccurrenceTimestamp: occurrenceTimestamp, // Scheduled time for this occurrence
		IsCancelled:         false,               // Always starts as not cancelled
		CancelledAt:         "",
	}
}

// buildTaskDoneUpdates creates the Firestore updates to mark a task as done.
// This is used for both exhausted recurrence tasks and cancelled recurrence tasks.
func buildTaskDoneUpdates(now time.Time) []firestore.Update {
	return []firestore.Update{
		{Path: "doneAt", Value: now.Format(time.RFC3339)},
		{Path: "leasedUntil", Value: ""},
	}
}

// calculateNextOccurrence finds the next occurrence strictly after `after` based on RRULE.
//
// Returns:
//   - nextTime: The timestamp of the next occurrence
//   - exhausted: true if no more occurrences (UNTIL reached or COUNT satisfied)
//   - error: Invalid RRULE data that cannot be parsed
func calculateNextOccurrence(recurrence *Recurrence, after time.Time) (time.Time, bool, error) {
	return findOccurrence(recurrence, after, false)
}

// calculateFirstOccurrence finds the first occurrence starting from DTStart.
// This is a convenience function for initial task scheduling - it validates the recurrence
// config and uses DTStart as the search starting point.
//
// Returns:
//   - nextTime: The timestamp of the first occurrence
//   - error: Invalid/missing recurrence configuration, or no valid occurrences exist
func calculateFirstOccurrence(recurrence *Recurrence) (time.Time, error) {
	if recurrence.DTStart == "" {
		return time.Time{}, fmt.Errorf("DTStart is required")
	}
	if recurrence.RRule == "" {
		return time.Time{}, fmt.Errorf("RRule is required")
	}

	dtstart, err := time.Parse(time.RFC3339, recurrence.DTStart)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid DTStart %q (must be RFC3339): %w", recurrence.DTStart, err)
	}

	firstOccurrence, exhausted, err := findOccurrence(recurrence, dtstart, true)
	if err != nil {
		return time.Time{}, err
	}
	if exhausted {
		return time.Time{}, fmt.Errorf("recurrence has no valid occurrences")
	}

	return firstOccurrence, nil
}

// findOccurrence is the shared implementation for finding RRULE occurrences.
// If inclusive is true, includes occurrences at exactly the reference time.
// If inclusive is false, finds occurrences strictly after the reference time.
func findOccurrence(recurrence *Recurrence, referenceTime time.Time, inclusive bool) (time.Time, bool, error) {
	if recurrence.DTStart == "" {
		return time.Time{}, false, fmt.Errorf("DTStart is required")
	}
	if recurrence.RRule == "" {
		return time.Time{}, false, fmt.Errorf("RRule is required")
	}

	dtstart, err := time.Parse(time.RFC3339, recurrence.DTStart)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid DTStart %q: %w", recurrence.DTStart, err)
	}

	rr, err := rrule.StrToRRule(recurrence.RRule)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid RRule %q: %w", recurrence.RRule, err)
	}
	rr.DTStart(dtstart)

	var set rrule.Set
	set.RRule(rr)

	for _, exdate := range recurrence.ExDates {
		t, err := time.Parse(time.RFC3339, exdate)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("failed to parse exdate %q: %w", exdate, err)
		}
		set.ExDate(t)
	}

	futureLimit := referenceTime.AddDate(10, 0, 0)

	var searchStart time.Time
	if inclusive {
		// Include occurrences at exactly referenceTime
		searchStart = referenceTime
	} else {
		// Strictly after referenceTime
		searchStart = referenceTime.Add(time.Second)
	}

	occurrences := set.Between(searchStart, futureLimit, true)

	if len(occurrences) == 0 {
		return time.Time{}, true, nil
	}

	return occurrences[0], false, nil
}
