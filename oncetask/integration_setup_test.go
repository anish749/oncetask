//go:build integration

package oncetask

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/gcloud"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	// 506.0.0 (Oct 2024) supports the multi-field inequality queries the
	// query builder relies on. The 428.0.0 image used elsewhere in the
	// org pre-dates that feature and rejects readyTasks.
	testFirestoreImage = "gcr.io/google.com/cloudsdktool/cloud-sdk:506.0.0-emulators"
	testProjectID      = "test-project"
)

// One emulator instance is shared across all tests in the package. Booting a
// fresh emulator per test would add ~10s each; isolating tests via per-test
// ONCE_TASK_ENV values gives us logical separation on the shared collection.
var (
	emulatorOnce   sync.Once
	emulatorClient *firestore.Client
	emulatorErr    error
)

// suffixCounter increments per call to give each test unique values for
// ONCE_TASK_ENV, task IDs, and task type kinds.
var suffixCounter int64

func uniqueSuffix() int64 {
	return atomic.AddInt64(&suffixCounter, 1)
}

// setupFirestoreEmulator returns a Firestore client connected to a shared
// testcontainers-managed emulator. The emulator boots once on first call and
// is reaped when the test process exits (via testcontainers' Ryuk reaper).
func setupFirestoreEmulator(ctx context.Context, t *testing.T) *firestore.Client {
	t.Helper()
	emulatorOnce.Do(func() {
		container, err := gcloud.RunFirestore(ctx,
			testFirestoreImage,
			gcloud.WithProjectID(testProjectID),
			testcontainers.WithWaitStrategy(
				wait.ForLog("Dev App Server is now running").
					WithStartupTimeout(2*time.Minute),
			),
		)
		if err != nil {
			emulatorErr = fmt.Errorf("failed to start firestore emulator: %w", err)
			return
		}

		// FIRESTORE_EMULATOR_HOST flips the Firestore SDK into emulator
		// mode — without it, BulkWriter rejects writes with
		// "Batch writes require admin authentication." Set globally
		// (not via t.Setenv) so all subsequent test goroutines see it.
		if err := os.Setenv("FIRESTORE_EMULATOR_HOST", container.URI); err != nil {
			emulatorErr = fmt.Errorf("failed to set FIRESTORE_EMULATOR_HOST: %w", err)
			return
		}

		client, err := firestore.NewClient(ctx, testProjectID,
			option.WithEndpoint(container.URI),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
		)
		if err != nil {
			emulatorErr = fmt.Errorf("failed to create firestore client: %w", err)
			return
		}

		// Confirm the emulator is actually serving requests.
		if _, err := client.Collection("emulator_handshake").Doc("ping").Set(ctx, map[string]any{"ok": true}); err != nil {
			emulatorErr = fmt.Errorf("emulator handshake failed: %w", err)
			return
		}
		emulatorClient = client
	})

	if emulatorErr != nil {
		t.Fatalf("emulator setup failed: %v", emulatorErr)
	}
	return emulatorClient
}

// newTestManager builds a manager scoped to a unique ONCE_TASK_ENV. Tasks it
// creates are invisible to other tests since every query is env-filtered.
//
// IMPORTANT: this calls t.Setenv, which is incompatible with t.Parallel().
// Tests using this helper (including their subtests) MUST NOT run in parallel.
func newTestManager[TaskKind ~string](ctx context.Context, t *testing.T) (manager Manager[TaskKind], envName string, cleanup func()) {
	t.Helper()
	client := setupFirestoreEmulator(ctx, t)
	envName = fmt.Sprintf("test_env_%d", uniqueSuffix())
	t.Setenv(EnvVariable, envName)

	manager, cleanup = NewFirestoreOnceTaskManager[TaskKind](ctx, client)
	return manager, envName, cleanup
}

// rawTestClient returns the shared Firestore client without creating a
// manager. Useful for low-level assertions on document state.
func rawTestClient(ctx context.Context, t *testing.T) *firestore.Client {
	t.Helper()
	return setupFirestoreEmulator(ctx, t)
}

// waitFor polls cond every 50ms until it returns true or the timeout elapses.
// Returns true if the condition was met, false on timeout.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

// requireWait fails the test if cond is not met within timeout.
func requireWait(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	if !waitFor(t, timeout, cond) {
		require.Fail(t, "timeout waiting for condition", msg)
	}
}
