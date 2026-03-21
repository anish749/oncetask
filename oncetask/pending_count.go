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
// Pending means: not done, not cancelled, waitUntil <= now, leasedUntil <= now.
// This reuses the same index as the readyTasks query, so no additional indexes are needed.
func (m *firestoreOnceTaskManager[TaskKind]) GetPendingCount(ctx context.Context, taskType TaskKind) (int, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	query := m.queryBuilder.base().
		Where("type", "==", string(taskType)).
		Where("doneAt", "==", "").
		Where("waitUntil", "<=", nowStr).
		Where("leasedUntil", "<=", nowStr)

	result, err := query.NewAggregationQuery().WithCount("count").Get(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending tasks for type %s: %w", taskType, err)
	}

	countVal, ok := result["count"]
	if !ok {
		return 0, nil
	}

	protoVal, ok := countVal.(*pb.Value)
	if !ok {
		return 0, fmt.Errorf("unexpected count type %T", countVal)
	}

	return int(protoVal.GetIntegerValue()), nil
}
