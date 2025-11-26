package oncetask

import (
	"time"

	"cloud.google.com/go/firestore"
)

// firestoreQueryBuilder provides environment-scoped queries for all task operations.
// All queries automatically include the environment filter.
type firestoreQueryBuilder struct {
	collection *firestore.CollectionRef
	env        string
}

func newFirestoreQueryBuilder(collection *firestore.CollectionRef, env string) *firestoreQueryBuilder {
	return &firestoreQueryBuilder{
		collection: collection,
		env:        env,
	}
}

// base returns the base query scoped to the current environment.
func (q *firestoreQueryBuilder) base() firestore.Query {
	return q.collection.Where("env", "==", q.env)
}

// readyTasks returns a query for tasks ready to execute.
// Ready means: waitUntil <= now, leasedUntil <= now (expired or never leased), not done.
func (q *firestoreQueryBuilder) readyTasks(taskType string, now time.Time) firestore.Query {
	nowStr := now.Format(time.RFC3339)
	return q.base().
		Where("type", "==", taskType).
		Where("doneAt", "==", "").
		Where("waitUntil", "<=", nowStr).
		Where("leasedUntil", "<=", nowStr).
		OrderBy("leasedUntil", firestore.Asc).
		OrderBy("waitUntil", firestore.Asc)
}

// readyTasksForResourceKey returns all ready tasks for a specific resource key.
// Does NOT filter by leasedUntil - caller must check lease status in-memory.
func (q *firestoreQueryBuilder) readyTasksForResourceKey(taskType, resourceKey string, now time.Time) firestore.Query {
	nowStr := now.Format(time.RFC3339)
	return q.base().
		Where("type", "==", taskType).
		Where("resourceKey", "==", resourceKey).
		Where("doneAt", "==", "").
		Where("waitUntil", "<=", nowStr).
		OrderBy("waitUntil", firestore.Asc)
}

// activeLeasesForResourceKey returns tasks with active leases for a resource key.
// Used for mutual exclusion checks - ensures only one task per resource key runs at a time.
func (q *firestoreQueryBuilder) activeLeasesForResourceKey(taskType, resourceKey string, now time.Time) firestore.Query {
	nowStr := now.Format(time.RFC3339)
	return q.base().
		Where("type", "==", taskType).
		Where("resourceKey", "==", resourceKey).
		Where("doneAt", "==", "").
		Where("leasedUntil", ">", nowStr)
}

// byResourceKey returns all tasks (any status) for a resource key.
func (q *firestoreQueryBuilder) byResourceKey(resourceKey string) firestore.Query {
	return q.base().Where("resourceKey", "==", resourceKey)
}

// doc returns a document reference for a task ID.
func (q *firestoreQueryBuilder) doc(taskID string) *firestore.DocumentRef {
	return q.collection.Doc(taskID)
}
