import "server-only";
import { COLLECTION, getFirestore } from "@/lib/firestore";
import { NO_WAIT, OnceTask } from "@/lib/types/oncetask";

export class MutationError extends Error {
  constructor(
    message: string,
    public readonly code: "NOT_FOUND" | "INVALID_STATE",
  ) {
    super(message);
  }
}

// resetTask mirrors oncetask.ResetTask in the Go library (pkg/oncetask/reset.go).
// Only valid on tasks in terminal state (doneAt != "").
// Clears: attempts, errors, doneAt, leasedUntil, result, isCancelled, cancelledAt.
// Sets: waitUntil = NoWait (immediate execution).
//
// Env scoping is intentionally not enforced here. The Go library enforces it
// to protect workers from cross-env contamination; for an admin operating on a
// specific clicked task, the env is whatever the task itself is stamped with.
export async function resetTask(id: string): Promise<void> {
  const db = getFirestore();
  const ref = db.collection(COLLECTION).doc(id);
  const snap = await ref.get();

  if (!snap.exists) {
    throw new MutationError(`task ${id} not found`, "NOT_FOUND");
  }
  const task = snap.data() as OnceTask;
  if (!task.doneAt) {
    throw new MutationError(
      `task ${id} is not in a terminal state - only completed, failed, or cancelled tasks can be reset`,
      "INVALID_STATE",
    );
  }

  await ref.update({
    attempts: 0,
    errors: [],
    doneAt: "",
    leasedUntil: "",
    waitUntil: NO_WAIT,
    isCancelled: false,
    cancelledAt: "",
    result: null,
  });
}

// cancelTask mirrors oncetask.CancelTask in the Go library (pkg/oncetask/cancellation.go).
// Marks a non-done task as cancelled. Idempotent for already-done/already-cancelled tasks.
export async function cancelTask(id: string): Promise<void> {
  const db = getFirestore();
  const ref = db.collection(COLLECTION).doc(id);
  const snap = await ref.get();

  if (!snap.exists) {
    throw new MutationError(`task ${id} not found`, "NOT_FOUND");
  }
  const task = snap.data() as OnceTask;
  if (task.doneAt || task.isCancelled) {
    // Idempotent no-op, mirroring Go library behavior.
    return;
  }

  await ref.update({
    isCancelled: true,
    cancelledAt: new Date().toISOString(),
    waitUntil: NO_WAIT,
  });
}

// deleteTask permanently removes a task. Idempotent.
// WARNING: deleting a leased task will cause the running handler to fail on completion.
export async function deleteTask(id: string): Promise<void> {
  const db = getFirestore();
  await db.collection(COLLECTION).doc(id).delete();
}
