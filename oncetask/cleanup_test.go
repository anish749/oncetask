package oncetask

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// stubFirestoreClient returns a *firestore.Client whose RPCs will fail —
// the goal is just to construct a manager whose worker goroutines run
// through the runLoop without ever doing any real work.
//
// The endpoint is a closed loopback port. firestore.NewClient is lazy and
// won't actually dial here; the workers' first transaction will error
// out, the runLoop will set shouldWait=true, and the goroutine will sit
// in its select waiting for shutdown — which is exactly the state we
// need to assert that cleanup() blocks until the goroutine exits.
func stubFirestoreClient(t *testing.T) *firestore.Client {
	t.Helper()
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "test-project",
		option.WithEndpoint("127.0.0.1:1"),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("stub firestore client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestCleanup_BlocksUntilWorkersExit guards the contract that cleanup()
// returned by NewFirestoreOnceTaskManager waits for every worker
// goroutine spawned via RegisterTaskHandler / RegisterResourceKeyHandler
// to actually exit before returning.
//
// Without this guarantee, the cleanup function returns the moment it has
// signalled cancellation, while worker goroutines may still be mid-RPC
// against Firestore. That has been observed in practice to leak
// transactions whose locks block the next caller.
//
// The test uses two complementary assertions:
//
//  1. Black-box: a stack dump taken immediately after cleanup() returns
//     must not contain any frame from runLoop.
//  2. White-box: the manager's WaitGroup must be drained by the time
//     cleanup() returns — verified by racing a separate wg.Wait() against
//     a tight timeout.
func TestCleanup_BlocksUntilWorkersExit(t *testing.T) {
	const concurrency = 4
	client := stubFirestoreClient(t)

	manager, cleanup := NewFirestoreOnceTaskManager[string](context.Background(), client)

	noopHandler := func(ctx context.Context, _ *OnceTask[string]) (any, error) { return nil, nil }
	if err := manager.RegisterTaskHandler("kind-a", noopHandler, WithConcurrency(concurrency)); err != nil {
		t.Fatalf("RegisterTaskHandler: %v", err)
	}

	// Give the worker goroutines a moment to actually start running.
	// Without this, "no runLoop frames in stack" is trivially true.
	requireRunLoopOnStack(t, 2*time.Second)

	// Black-box assertion: cleanup blocks until workers exit. We measure
	// this by checking the runtime stack immediately after cleanup
	// returns — there must be no remaining runLoop frames. Without the
	// fix (cleanup just calls cancel without waiting), workers may still
	// be present in the stack.
	cleanup()

	if frames := countRunLoopFrames(t); frames != 0 {
		t.Fatalf("cleanup returned with %d runLoop goroutine(s) still on the stack; cleanup must wait for workers to exit", frames)
	}

	// White-box assertion: the manager's WaitGroup is drained.
	// Doing wg.Wait() now must return immediately. We allow a tiny
	// scheduling slack but anything beyond that means cleanup left work
	// behind.
	mgr := manager.(*firestoreOnceTaskManager[string])
	done := make(chan struct{})
	go func() {
		mgr.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("manager.wg.Wait() did not return immediately after cleanup; goroutines are still tracked")
	}
}

// TestCleanup_BlocksUntilWorkersExit_ResourceKeyHandler covers the same
// contract for handlers registered via RegisterResourceKeyHandler.
func TestCleanup_BlocksUntilWorkersExit_ResourceKeyHandler(t *testing.T) {
	const concurrency = 3
	client := stubFirestoreClient(t)

	manager, cleanup := NewFirestoreOnceTaskManager[string](context.Background(), client)

	noopHandler := func(ctx context.Context, _ []OnceTask[string]) (any, error) { return nil, nil }
	if err := manager.RegisterResourceKeyHandler("kind-rk", noopHandler, WithConcurrency(concurrency)); err != nil {
		t.Fatalf("RegisterResourceKeyHandler: %v", err)
	}

	requireRunLoopOnStack(t, 2*time.Second)

	cleanup()

	if frames := countRunLoopFrames(t); frames != 0 {
		t.Fatalf("cleanup returned with %d runLoop goroutine(s) still on the stack", frames)
	}
}

// TestCleanup_NoWorkers_ReturnsImmediately: when no handlers have been
// registered, cleanup() has nothing to wait for and must not block.
func TestCleanup_NoWorkers_ReturnsImmediately(t *testing.T) {
	client := stubFirestoreClient(t)

	_, cleanup := NewFirestoreOnceTaskManager[string](context.Background(), client)

	done := make(chan struct{})
	go func() {
		cleanup()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup blocked despite no handlers being registered")
	}
}

// requireRunLoopOnStack asserts that at least one runLoop goroutine is
// observable in the runtime stack within timeout. Use it to confirm
// the workers are actually running before measuring cleanup behaviour.
func requireRunLoopOnStack(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if countRunLoopFrames(t) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no runLoop goroutine observed within %s — workers never started", timeout)
}

// countRunLoopFrames returns the number of goroutines whose stack
// contains a frame from firestoreOnceTaskManager.runLoop. The runtime
// formats generic methods with bracketed type parameters, e.g.
// "github.com/anish749/oncetask/oncetask.(*firestoreOnceTaskManager[...]).runLoop"
// — matching on ".runLoop(" is enough to identify those frames
// unambiguously since no other symbol in this package shares that name.
func countRunLoopFrames(t *testing.T) int {
	t.Helper()
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			return strings.Count(string(buf[:n]), ".runLoop(")
		}
		buf = make([]byte, 2*len(buf))
	}
}
