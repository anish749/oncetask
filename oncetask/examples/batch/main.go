// This package demonstrates batch processing with OnceTask.
//
// This example shows how to implement batch processing as a usage pattern
// on top of the OnceTask library. A batch consists of:
//   - Item tasks: individual work items, each a regular OnceTask
//   - Batch task: a monitoring task that checks when all items are done
//
// The batch task doesn't process items itself. It watches them and triggers
// a notification when all items complete.
//
// Flow:
//  1. Create all item tasks with deterministic IDs
//  2. Create a batch completion task that stores the item task IDs in its data
//  3. Item tasks execute independently (parallel)
//  4. Batch completion task periodically checks if all items are done
//  5. When complete, batch completion task sends notification and marks itself done
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/anish749/go-donna/pkg/oncetask"
)

// TaskKind defines the types of tasks in our batch processing system.
type TaskKind string

const (
	TaskKindBatchItem       TaskKind = "batchItem"
	TaskKindBatchCompletion TaskKind = "batchCompletion"
)

// Batch represents a complete batch with its metadata and items.
// This struct is used to define the batch before creating tasks.
type Batch struct {
	ID    string
	Name  string
	Items []BatchItem
}

// NewBatch creates a new batch with the given metadata and items.
func NewBatch(name string, payloads []string) Batch {
	batchID := "batch_" + time.Now().Format("20060102_150405")
	items := make([]BatchItem, len(payloads))
	for i, payload := range payloads {
		items[i] = BatchItem{
			BatchID: batchID,
			Index:   i,
			Payload: payload,
		}
	}
	return Batch{
		ID:    batchID,
		Name:  name,
		Items: items,
	}
}

// BatchItem represents a single item in a batch.
// It is both the domain object and the task data type.
type BatchItem struct {
	BatchID string `json:"batchId"`
	Index   int    `json:"index"`
	Payload string `json:"payload"`
}

var _ oncetask.OnceTaskData[TaskKind] = (*BatchItem)(nil)

func (t BatchItem) GetType() TaskKind { return TaskKindBatchItem }

// GenerateIdempotentID creates a deterministic ID based on batch and item index.
// This ensures the same item always generates the same task ID.
func (t BatchItem) GenerateIdempotentID() string {
	return fmt.Sprintf("%s_item_%d", t.BatchID, t.Index)
}

// BatchCompletionTaskData monitors the completion of all items in a batch.
type BatchCompletionTaskData struct {
	BatchID     string   `json:"batchId"`
	ItemTaskIds []string `json:"itemTaskIds"`
	BatchName   string   `json:"batchName"`
}

func (t BatchCompletionTaskData) GetType() TaskKind { return TaskKindBatchCompletion }

// GenerateIdempotentID creates a deterministic ID for the batch monitoring task.
func (t BatchCompletionTaskData) GenerateIdempotentID() string {
	return fmt.Sprintf("batch_%s", t.BatchID)
}

// BatchPendingError is returned when batch items are still pending.
// It carries progress information that BatchCompletionRetryPolicy uses to calculate delay.
type BatchPendingError struct {
	Pending int
	Total   int
}

func (e *BatchPendingError) Error() string {
	return fmt.Sprintf("batch not complete: %d/%d items pending", e.Pending, e.Total)
}

// BatchCompletionRetryPolicy is a custom retry policy for batch completion tasks.
// It calculates retry delay based on batch progress extracted from BatchPendingError.
type BatchCompletionRetryPolicy struct {
	MinDelay time.Duration // Delay when almost done (e.g., 5s)
	MaxDelay time.Duration // Delay when just started (e.g., 1min)
}

func (p BatchCompletionRetryPolicy) ShouldRetry(attempts int, err error) bool {
	return true // Unlimited retries - keep checking until batch is complete
}

// NextRetryDelay calculates delay based on batch progress.
// Linear scale: pendingRatio 0→1 maps to MinDelay→MaxDelay.
// More items pending = longer delay, fewer pending = shorter delay.
func (p BatchCompletionRetryPolicy) NextRetryDelay(attempts int, err error) time.Duration {
	if batchErr, ok := err.(*BatchPendingError); ok && batchErr.Total > 0 {
		pendingRatio := float64(batchErr.Pending) / float64(batchErr.Total)
		return p.MinDelay + time.Duration(float64(p.MaxDelay-p.MinDelay)*pendingRatio)
	}
	return p.MinDelay
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Initialize Firestore client
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = "your-project-id" // Replace with your project ID
		slog.WarnContext(ctx, "GOOGLE_CLOUD_PROJECT not set, using default", "projectID", projectID)
	}

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create Firestore client: %v", err)
	}
	defer client.Close()

	// Create the OnceTask manager
	manager, cancelManager := oncetask.NewFirestoreOnceTaskManager[TaskKind](client)

	// Register handlers
	if err := registerHandlers(manager); err != nil {
		log.Fatalf("Failed to register handlers: %v", err)
	}

	// Create a sample batch
	payloads := make([]string, 10)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("Data for row %d", i)
	}
	batch := NewBatch("Sample Batch Processing Job", payloads)

	slog.InfoContext(ctx, "Creating batch", "batchId", batch.ID, "itemCount", len(batch.Items))

	if err := createBatchTasks(ctx, manager, batch); err != nil {
		log.Fatalf("Failed to create batch: %v", err)
	}

	slog.InfoContext(ctx, "Batch created, waiting for processing...", "batchId", batch.ID)
	slog.InfoContext(ctx, "Press Ctrl+C to exit")

	// Wait for signal
	<-sigChan
	slog.InfoContext(ctx, "Shutting down...")
	cancelManager()
}

// registerHandlers sets up the task handlers for batch processing.
func registerHandlers(manager oncetask.OnceTaskManager[TaskKind]) error {
	// Register the batch item handler
	err := manager.RegisterTaskHandler(
		TaskKindBatchItem,
		batchItemHandler,
		oncetask.WithRetryPolicy(oncetask.ExponentialBackoffPolicy{
			MaxAttempts: 3,
			BaseDelay:   1 * time.Second,
			MaxDelay:    30 * time.Second,
			Multiplier:  2.0,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register batch_item handler: %w", err)
	}

	// Register the batch completion handler with custom progress-based retry policy
	err = manager.RegisterTaskHandler(
		TaskKindBatchCompletion,
		createBatchCompletionHandler(manager),
		oncetask.WithRetryPolicy(BatchCompletionRetryPolicy{
			MinDelay: 5 * time.Second,
			MaxDelay: 1 * time.Minute,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to register batch_completion handler: %w", err)
	}

	return nil
}

// batchItemHandler processes a single item in the batch.
func batchItemHandler(ctx context.Context, task *oncetask.OnceTask[TaskKind]) (any, error) {
	var item BatchItem
	if err := task.ReadInto(&item); err != nil {
		return nil, fmt.Errorf("failed to read task data: %w", err)
	}

	slog.InfoContext(ctx, "Processing batch item",
		"batchId", item.BatchID,
		"index", item.Index,
		"payload", item.Payload,
	)

	// Simulate processing time (0-2 seconds)
	processingTime := time.Duration(rand.Intn(2000)) * time.Millisecond
	time.Sleep(processingTime)

	// Simulate occasional failures (10% chance)
	if rand.Float32() < 0.1 {
		return nil, fmt.Errorf("simulated random failure for item %d", item.Index)
	}

	result := map[string]any{
		"processedAt":    time.Now().Format(time.RFC3339),
		"processingTime": processingTime.String(),
		"status":         "success",
	}

	slog.InfoContext(ctx, "Batch item processed successfully",
		"batchId", item.BatchID,
		"index", item.Index,
	)

	return result, nil
}

// createBatchCompletionHandler creates a handler that monitors batch completion.
// It needs access to the manager to query item tasks.
func createBatchCompletionHandler(manager oncetask.OnceTaskManager[TaskKind]) oncetask.OnceTaskHandler[TaskKind] {
	return func(ctx context.Context, task *oncetask.OnceTask[TaskKind]) (any, error) {
		var data BatchCompletionTaskData
		if err := task.ReadInto(&data); err != nil {
			return nil, fmt.Errorf("failed to read task data: %w", err)
		}

		// Fetch all item tasks
		items, err := manager.GetTasksByIds(ctx, data.ItemTaskIds)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch item tasks: %w", err)
		}

		now := time.Now()
		var pending, completed, failed int
		for _, item := range items {
			status, err := item.GetStatus(now)
			if err != nil {
				return nil, fmt.Errorf("failed to get status for task %s: %w", item.Id, err)
			}
			switch status {
			case oncetask.TaskStatusCompleted:
				completed++
			case oncetask.TaskStatusFailed:
				failed++
			default:
				// TaskStatusPending, TaskStatusWaiting, TaskStatusLeased are all "pending" for batch purposes
				pending++
			}
		}

		slog.InfoContext(ctx, "Batch status", "batchId", data.BatchID, "pending", pending, "completed", completed, "failed", failed, "total", len(data.ItemTaskIds))

		// If any items are still pending, retry later
		// BatchCompletionRetryPolicy extracts progress from the error to calculate delay
		if pending > 0 {
			return nil, &BatchPendingError{
				Pending: pending,
				Total:   len(data.ItemTaskIds),
			}
		}

		// All items are done - send notification
		sendNotification(ctx, data, completed, failed)

		slog.InfoContext(ctx, "Batch completed!", "batchId", data.BatchID, "completed", completed, "failed", failed)

		return nil, nil
	}
}

// sendNotification simulates sending a notification to the user.
func sendNotification(ctx context.Context, data BatchCompletionTaskData, completed, failed int) {
	slog.InfoContext(ctx, "Sending notification",
		"batchName", data.BatchName,
		"completed", completed,
		"failed", failed,
	)

	fmt.Printf("\n========================================\n")
	fmt.Printf("NOTIFICATION: Batch '%s' complete!\n", data.BatchName)
	fmt.Printf("Results: %d completed, %d failed\n", completed, failed)
	fmt.Printf("========================================\n\n")
}

// createBatchTasks creates item tasks and a monitoring task from a Batch.
func createBatchTasks(ctx context.Context, manager oncetask.OnceTaskManager[TaskKind], batch Batch) error {
	// Step 1: Build all item tasks and collect their IDs
	itemTaskIds := make([]string, len(batch.Items))
	itemsData := make([]oncetask.OnceTaskData[TaskKind], len(batch.Items))
	for i, item := range batch.Items {
		itemTaskIds[i] = item.GenerateIdempotentID()
		itemsData[i] = item
	}

	// Step 2: Create all item tasks in a single bulk operation
	created, err := manager.CreateTasks(ctx, itemsData)
	if err != nil {
		return fmt.Errorf("failed to create item tasks: %w", err)
	}
	slog.InfoContext(ctx, "Created item tasks", "created", created, "total", len(batch.Items))

	// Step 3: Create the batch monitoring task
	batchCompletionData := BatchCompletionTaskData{
		BatchID:     batch.ID,
		ItemTaskIds: itemTaskIds,
		BatchName:   batch.Name,
	}

	completionCreated, err := manager.CreateTask(ctx, batchCompletionData)
	if err != nil {
		return fmt.Errorf("failed to create batch completion task: %w", err)
	}
	if completionCreated {
		slog.InfoContext(ctx, "Created batch completion task", "taskId", batchCompletionData.GenerateIdempotentID(), "itemCount", len(itemTaskIds))
	}

	return nil
}
