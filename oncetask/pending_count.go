package oncetask

import (
	"context"
	"fmt"
	"time"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

// GetPendingCount returns the number of tasks that are ready to execute (pending)
// for the given task type. Uses a Firestore COUNT aggregation query — no documents
// are transferred, costing 1 read per 1000 tasks counted.
//
// Pending means: not done, waitUntil <= now, leasedUntil <= now.
// Reuses the same Firestore index as task processing, so no additional indexes are needed.
func (m *firestoreOnceTaskManager[TaskKind]) GetPendingCount(ctx context.Context, taskType TaskKind) (int, error) {
	now := time.Now().UTC()

	query := m.queryBuilder.pendingTasks(string(taskType), now)
	result, err := query.NewAggregationQuery().WithCount("count").Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending tasks for type %s: %w", taskType, err)
	}

	countVal, ok := result["count"]
	if !ok {
		return 0, fmt.Errorf("count alias missing from aggregation result for type %s", taskType)
	}

	protoVal, ok := countVal.(*pb.Value)
	if !ok {
		return 0, fmt.Errorf("unexpected count type %T", countVal)
	}

	return int(protoVal.GetIntegerValue()), nil
}
