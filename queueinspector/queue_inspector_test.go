package queueinspector

import (
	"testing"
	"time"

	"github.com/anish749/oncetask/oncetask"
)

func TestComputeQueueDepth(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name      string
		tasks     []oncetask.OnceTask[string]
		wantDepth QueueDepth
	}{
		{
			name:      "empty tasks",
			tasks:     nil,
			wantDepth: QueueDepth{},
		},
		{
			name: "single pending task",
			tasks: []oncetask.OnceTask[string]{
				{
					Id:        "t1",
					WaitUntil: now.Add(-1 * time.Hour).Format(time.RFC3339),
				},
			},
			wantDepth: QueueDepth{Pending: 1},
		},
		{
			name: "single leased task",
			tasks: []oncetask.OnceTask[string]{
				{
					Id:          "t1",
					LeasedUntil: now.Add(5 * time.Minute).Format(time.RFC3339),
				},
			},
			wantDepth: QueueDepth{Leased: 1},
		},
		{
			name: "single waiting task",
			tasks: []oncetask.OnceTask[string]{
				{
					Id:        "t1",
					WaitUntil: now.Add(1 * time.Hour).Format(time.RFC3339),
				},
			},
			wantDepth: QueueDepth{Waiting: 1},
		},
		{
			name: "single cancellation pending task",
			tasks: []oncetask.OnceTask[string]{
				{
					Id:          "t1",
					IsCancelled: true,
					WaitUntil:   oncetask.NoWait,
				},
			},
			wantDepth: QueueDepth{CancellationPending: 1},
		},
		{
			name: "mixed statuses",
			tasks: []oncetask.OnceTask[string]{
				{Id: "pending1", WaitUntil: now.Add(-1 * time.Hour).Format(time.RFC3339)},
				{Id: "pending2", WaitUntil: oncetask.NoWait},
				{Id: "leased1", LeasedUntil: now.Add(5 * time.Minute).Format(time.RFC3339)},
				{Id: "waiting1", WaitUntil: now.Add(30 * time.Minute).Format(time.RFC3339)},
				{Id: "waiting2", WaitUntil: now.Add(1 * time.Hour).Format(time.RFC3339)},
				{Id: "cancel1", IsCancelled: true, WaitUntil: oncetask.NoWait},
			},
			wantDepth: QueueDepth{
				Pending:             2,
				Leased:              1,
				Waiting:             2,
				CancellationPending: 1,
			},
		},
		{
			name: "terminal tasks are ignored",
			tasks: []oncetask.OnceTask[string]{
				// completed
				{Id: "done1", DoneAt: now.Add(-1 * time.Hour).Format(time.RFC3339), Attempts: 1},
				// failed
				{Id: "done2", DoneAt: now.Add(-1 * time.Hour).Format(time.RFC3339), Attempts: 1, Errors: []oncetask.TaskError{{At: now.Format(time.RFC3339), Error: "err"}}},
				// cancelled
				{Id: "done3", DoneAt: now.Add(-1 * time.Hour).Format(time.RFC3339), IsCancelled: true},
				// one active task
				{Id: "active", WaitUntil: oncetask.NoWait},
			},
			wantDepth: QueueDepth{Pending: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			depth, err := ComputeQueueDepth(tt.tasks, now)
			if err != nil {
				t.Fatalf("ComputeQueueDepth failed: %v", err)
			}
			if depth != tt.wantDepth {
				t.Errorf("got %+v, want %+v", depth, tt.wantDepth)
			}
		})
	}
}

func TestComputeQueueDepth_InvalidTimestamp(t *testing.T) {
	now := time.Now().UTC()
	tasks := []oncetask.OnceTask[string]{
		{Id: "bad", LeasedUntil: "not-a-timestamp"},
	}

	_, err := ComputeQueueDepth(tasks, now)
	if err == nil {
		t.Fatal("expected error for invalid timestamp, got nil")
	}
}

func TestQueueDepthTotal(t *testing.T) {
	depth := QueueDepth{
		Pending:             3,
		Waiting:             2,
		Leased:              1,
		CancellationPending: 1,
	}
	if got := depth.Total(); got != 7 {
		t.Errorf("Total() = %d, want 7", got)
	}
}

func TestQueueDepthTotal_Zero(t *testing.T) {
	depth := QueueDepth{}
	if got := depth.Total(); got != 0 {
		t.Errorf("Total() = %d, want 0", got)
	}
}
