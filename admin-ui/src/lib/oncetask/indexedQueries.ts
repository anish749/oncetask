import "server-only";
import { COLLECTION, getFirestore } from "@/lib/firestore";
import { MAX_TASKS_PER_QUERY, OnceTask } from "@/lib/types/oncetask";

interface ListResult {
  tasks: OnceTask[];
  // hasMore is true when the query returned exactly `limit` rows — i.e. the cap
  // was the constraint. UI surfaces this so operators know more may exist.
  hasMore: boolean;
}

function toResult(
  snap: FirebaseFirestore.QuerySnapshot,
  limit: number,
): ListResult {
  return {
    tasks: snap.docs.map((d) => ({
      id: d.id,
      ...(d.data() as Omit<OnceTask, "id">),
    })),
    hasMore: snap.docs.length === limit,
  };
}

// Index-aware queries that match the shapes Firestore already indexes for the
// production Go code (oncetask/firestore_queries.go). The 5-field composite
// [doneAt, env, type, leasedUntil, waitUntil] serves the Active modes; the
// single-field auto-index on doneAt serves Done; whatever index byResourceKey
// already uses in production serves Resource.
//
// Sentinels match the Go side: doneAt/leasedUntil use the empty string for
// "not set"; waitUntil uses the Go zero time (handled by the index since
// "" < "0001-01-01..." < any real timestamp).

export interface ActiveTasksArgs {
  env: string;
  type: string;
  limit?: number;
}

export interface DoneTasksArgs {
  limit?: number;
}

export interface ResourceTasksArgs {
  env: string;
  resourceKey: string;
  limit?: number;
}

// listActiveTasks returns every not-done task for env+type, ordered the same
// way Go's readyTasks orders them (leasedUntil asc, waitUntil asc). Uses the
// existing 5-field composite [doneAt, env, type, leasedUntil, waitUntil].
// Sub-state filtering (Ready / Leased / Waiting) is done client-side via
// getTaskStatus — Firestore can't push those without additional indexes.
export async function listActiveTasks({
  env,
  type,
  limit = MAX_TASKS_PER_QUERY,
}: ActiveTasksArgs): Promise<ListResult> {
  const db = getFirestore();
  const snap = await db
    .collection(COLLECTION)
    .where("env", "==", env)
    .where("type", "==", type)
    .where("doneAt", "==", "")
    .orderBy("leasedUntil", "asc")
    .orderBy("waitUntil", "asc")
    .limit(limit)
    .get();
  return toResult(snap, limit);
}

// listDoneTasks returns the most recently done tasks across all envs and types.
// Uses the single-field auto-index on doneAt. No env/type push-down — that
// would require a new composite. JS-side filtering is the caller's option.
export async function listDoneTasks({
  limit = MAX_TASKS_PER_QUERY,
}: DoneTasksArgs = {}): Promise<ListResult> {
  const db = getFirestore();
  const snap = await db
    .collection(COLLECTION)
    .where("doneAt", "!=", "")
    .orderBy("doneAt", "desc")
    .limit(limit)
    .get();
  return toResult(snap, limit);
}

// listResourceTasks returns every task (any status) for a specific env +
// resourceKey. Mirrors Go's byResourceKey query so it uses the same index.
export async function listResourceTasks({
  env,
  resourceKey,
  limit = MAX_TASKS_PER_QUERY,
}: ResourceTasksArgs): Promise<ListResult> {
  const db = getFirestore();
  const snap = await db
    .collection(COLLECTION)
    .where("env", "==", env)
    .where("resourceKey", "==", resourceKey)
    .limit(limit)
    .get();
  return toResult(snap, limit);
}
