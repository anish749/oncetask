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

// stubFirestoreClient returns a client whose RPCs will fail. The endpoint
// is a closed loopback port, so workers spin up runLoop, error out on the
// first RPC, and sit waiting for shutdown — no emulator required.
func stubFirestoreClient(t *testing.T) *firestore.Client {
	t.Helper()
	client, err := firestore.NewClient(context.Background(), "test-project",
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

func TestCleanup_BlocksUntilWorkersExit(t *testing.T) {
	const concurrency = 4
	client := stubFirestoreClient(t)

	manager, cleanup := NewFirestoreOnceTaskManager[string](context.Background(), client)

	noopHandler := func(ctx context.Context, _ *OnceTask[string]) (any, error) { return nil, nil }
	if err := manager.RegisterTaskHandler("kind-a", noopHandler, WithConcurrency(concurrency)); err != nil {
		t.Fatalf("RegisterTaskHandler: %v", err)
	}

	requireRunLoopOnStack(t, 2*time.Second)

	cleanup()

	if frames := countRunLoopFrames(t); frames != 0 {
		t.Fatalf("cleanup returned with %d runLoop goroutine(s) still running", frames)
	}

	// cleanupWaitGroup must be drained by the time cleanup returns.
	mgr := manager.(*firestoreOnceTaskManager[string])
	done := make(chan struct{})
	go func() {
		mgr.cleanupWaitGroup.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cleanupWaitGroup did not drain after cleanup returned")
	}
}

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
		t.Fatalf("cleanup returned with %d runLoop goroutine(s) still running", frames)
	}
}

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

func requireRunLoopOnStack(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if countRunLoopFrames(t) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no runLoop goroutine observed within %s", timeout)
}

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
