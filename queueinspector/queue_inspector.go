package queueinspector

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/anish749/oncetask/oncetask"
)

// QueueDepth holds counts of non-terminal tasks grouped by status.
// Only includes active (non-done) tasks — completed, failed, and cancelled tasks are excluded.
type QueueDepth struct {
	Pending             int `json:"pending"`
	Waiting             int `json:"waiting"`
	Leased              int `json:"leased"`
	CancellationPending int `json:"cancellationPending"`
}

// Total returns the total number of non-terminal tasks.
func (q QueueDepth) Total() int {
	return q.Pending + q.Waiting + q.Leased + q.CancellationPending
}

// QueueInspector provides visibility into the task queue state.
// This is a separate interface from Manager — it can be created and used independently.
type QueueInspector[TaskKind ~string] interface {
	// GetQueueDepth returns counts of non-terminal tasks for the given task type,
	// grouped by status (pending, waiting, leased, cancellationPending).
	//
	// This queries all non-done tasks from Firestore and computes status in memory.
	// Cost: 1 Firestore read per active task of the given type.
	GetQueueDepth(ctx context.Context, taskType TaskKind) (QueueDepth, error)
}

// firestoreQueueInspector implements QueueInspector using Firestore queries.
type firestoreQueueInspector[TaskKind ~string] struct {
	collection *firestore.CollectionRef
	env        string
}

// NewFirestoreQueueInspector creates a QueueInspector backed by Firestore.
// It uses the same collection and environment as the task manager.
func NewFirestoreQueueInspector[TaskKind ~string](client *firestore.Client) QueueInspector[TaskKind] {
	env := oncetask.DefaultEnv
	if e := getEnv(); e != "" {
		env = e
	}
	return &firestoreQueueInspector[TaskKind]{
		collection: client.Collection(oncetask.CollectionOnceTasks),
		env:        env,
	}
}

func (q *firestoreQueueInspector[TaskKind]) GetQueueDepth(ctx context.Context, taskType TaskKind) (QueueDepth, error) {
	// Query all non-done tasks for this type
	query := q.collection.
		Where("env", "==", q.env).
		Where("type", "==", string(taskType)).
		Where("doneAt", "==", "")

	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return QueueDepth{}, fmt.Errorf("failed to query non-done tasks for type %s: %w", taskType, err)
	}

	now := time.Now().UTC()

	tasks := make([]oncetask.OnceTask[TaskKind], 0, len(docs))
	for _, doc := range docs {
		var task oncetask.OnceTask[TaskKind]
		if err := doc.DataTo(&task); err != nil {
			return QueueDepth{}, fmt.Errorf("failed to parse task %s: %w", doc.Ref.ID, err)
		}
		tasks = append(tasks, task)
	}

	return ComputeQueueDepth(tasks, now)
}

// ComputeQueueDepth computes a QueueDepth from a slice of tasks at the given time.
// Only non-terminal statuses are counted; completed/failed/cancelled tasks are ignored.
func ComputeQueueDepth[TaskKind ~string](tasks []oncetask.OnceTask[TaskKind], now time.Time) (QueueDepth, error) {
	var depth QueueDepth

	for _, task := range tasks {
		status, err := task.GetStatus(now)
		if err != nil {
			return QueueDepth{}, fmt.Errorf("failed to get status for task %s: %w", task.Id, err)
		}

		switch status {
		case oncetask.TaskStatusPending:
			depth.Pending++
		case oncetask.TaskStatusWaiting:
			depth.Waiting++
		case oncetask.TaskStatusLeased:
			depth.Leased++
		case oncetask.TaskStatusCancellationPending:
			depth.CancellationPending++
		}
	}

	return depth, nil
}
