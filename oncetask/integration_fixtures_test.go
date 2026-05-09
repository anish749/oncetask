//go:build integration

package oncetask

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"
)

// testKind is the TaskKind type used throughout integration tests.
type testKind string

// testTaskData is a generic Data implementation. By implementing the optional
// provider interfaces dynamically (via the PayloadXxx fields), each test can
// shape a single fixture into resource-keyed, scheduled, or recurring tasks
// without inventing N bespoke types per test case.
//
// Methods use value receivers (not pointer receivers) because the library's
// Data[TaskKind] interface check works on the concrete type passed to
// CreateTask — and tests routinely pass these as struct literals. This
// triggers gocritic's hugeParam, which we silence with a single nolint.
//
//nolint:govet // fieldalignment: grouping by purpose is more readable here
type testTaskData struct {
	Kind        testKind    `json:"kind"`
	IDValue     string      `json:"id"`
	Payload     string      `json:"payload"`
	ResourceKey string      `json:"resourceKey,omitempty"`
	ScheduleAt  time.Time   `json:"scheduleAt,omitempty"`
	Rec         *Recurrence `json:"rec,omitempty"`
}

//nolint:gocritic // hugeParam: value receiver required so struct literals satisfy Data
func (d testTaskData) GetType() testKind { return d.Kind }

//nolint:gocritic // hugeParam: value receiver required so struct literals satisfy Data
func (d testTaskData) GenerateIdempotentID() string {
	if d.IDValue != "" {
		return d.IDValue
	}
	h := sha1.New()
	fmt.Fprintf(h, "%s|%s|%s", d.Kind, d.Payload, d.ResourceKey)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// GetResourceKey is part of the optional ResourceKeyProvider interface.
// Returning empty disables resource-key behaviour for that task.
//
//nolint:gocritic // hugeParam: value receiver required for the optional provider check
func (d testTaskData) GetResourceKey() string { return d.ResourceKey }

// GetScheduledTime is part of the optional ScheduledTask interface. The
// zero time disables scheduled execution.
//
//nolint:gocritic // hugeParam: value receiver required for the optional provider check
func (d testTaskData) GetScheduledTime() time.Time { return d.ScheduleAt }

// recurringTaskData is used only when a test wants a recurrence task.
// We can't expose Rec on testTaskData unconditionally because the provider
// interface check uses a non-nil result to mean "this is a recurrence task".
type recurringTaskData struct {
	testTaskData
}

//nolint:gocritic // hugeParam: value receiver required for the optional provider check
func (d recurringTaskData) GetRecurrence() *Recurrence { return d.Rec }

// makeKind generates a unique TaskKind per test so handlers from different
// tests don't share state.
func makeKind(prefix string) testKind {
	return testKind(fmt.Sprintf("%s_%d", prefix, uniqueSuffix()))
}

// makeTaskID generates a stable but unique id within the test.
func makeTaskID(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, uniqueSuffix())
}
