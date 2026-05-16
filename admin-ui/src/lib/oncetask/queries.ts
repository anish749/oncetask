import "server-only";
import { COLLECTION, getFirestore } from "@/lib/firestore";
import {
  MAX_TASKS_PER_QUERY,
  OnceTask,
  TaskStatus,
  getTaskStatus,
} from "@/lib/types/oncetask";

export interface ListTasksFilters {
  status?: TaskStatus;
  type?: string;
  env?: string;
  resourceKey?: string;
  limit?: number;
  orderBy?: string;
  orderDir?: "asc" | "desc";
}

export interface ListTasksResult {
  tasks: OnceTask[];
  // hasMore is true when the underlying Firestore query returned exactly `limit`
  // rows — i.e. the cap was the constraint, not data exhaustion. Surfaced so
  // the UI can warn the operator that more matching rows may exist.
  hasMore: boolean;
}

// listTasks fetches tasks from Firestore with optional filters.
// Env is just another filter, like type or status - the admin tool is not scoped
// to a single env. Operator picks env in the UI.
//
// Ordering is driven by the caller via `orderBy` + `orderDir`. The client is the
// source of truth for which field to sort on per status (so the UI's column
// marker always matches what the server applied). The per-status `where` clauses
// below are the only filtering logic we own here; we don't add server-side
// orderBy defaults.
export async function listTasks(
  filters: ListTasksFilters = {},
): Promise<ListTasksResult> {
  const db = getFirestore();
  let q: FirebaseFirestore.Query = db.collection(COLLECTION);

  if (filters.env) {
    q = q.where("env", "==", filters.env);
  }
  if (filters.type) {
    q = q.where("type", "==", filters.type);
  }

  // Sentinels match the Go production code (oncetask/firestore_queries.go):
  // doneAt and leasedUntil use the empty string for "not set"; waitUntil uses
  // the Go zero time string. Inequality comparisons against empty work the
  // same as against any zero value since "" is the lex-smallest string.
  if (filters.status) {
    const now = new Date().toISOString();
    switch (filters.status) {
      case TaskStatus.COMPLETED:
      case TaskStatus.FAILED:
      case TaskStatus.CANCELLED:
        q = q.where("doneAt", "!=", "");
        break;
      case TaskStatus.LEASED:
        q = q.where("leasedUntil", ">", now);
        break;
      case TaskStatus.WAITING:
        q = q.where("waitUntil", ">", now);
        break;
      case TaskStatus.PENDING:
      case TaskStatus.CANCELLATION_PENDING:
        q = q.where("doneAt", "==", "");
        break;
    }
  }

  if (filters.orderBy) {
    q = q.orderBy(filters.orderBy, filters.orderDir ?? "desc");
  }

  const limit = filters.limit ?? MAX_TASKS_PER_QUERY;
  const snapshot = await q.limit(limit).get();
  const hasMore = snapshot.docs.length === limit;

  let tasks: OnceTask[] = snapshot.docs.map((doc) => ({
    id: doc.id,
    ...(doc.data() as Omit<OnceTask, "id">),
  }));

  // Derived-status post-filter: getTaskStatus folds in fields Firestore can't
  // compare server-side (e.g. attempts > errors.length for COMPLETED vs FAILED).
  if (filters.status) {
    tasks = tasks.filter((t) => getTaskStatus(t) === filters.status);
  }

  if (filters.resourceKey && filters.resourceKey.trim()) {
    const needle = filters.resourceKey.trim().toLowerCase();
    tasks = tasks.filter((t) => t.resourceKey?.toLowerCase().includes(needle));
  }

  return { tasks, hasMore };
}

export async function getTask(id: string): Promise<OnceTask | null> {
  const db = getFirestore();
  const doc = await db.collection(COLLECTION).doc(id).get();
  if (!doc.exists) return null;
  return { id: doc.id, ...(doc.data() as Omit<OnceTask, "id">) };
}

// distinctValues enumerates every distinct value of `field` via an index walk:
// orderBy(field) + startAfter(cursor) jumps directly to the next value past
// the last one seen. One indexed seek per distinct value (not a full scan),
// and no cap from Firestore's 10-value `not-in` limit.
async function distinctValues(field: string): Promise<string[]> {
  const db = getFirestore();
  const values: string[] = [];
  let cursor: string | undefined;

  while (true) {
    let q: FirebaseFirestore.Query = db
      .collection(COLLECTION)
      .select(field)
      .orderBy(field)
      .limit(1);
    if (cursor !== undefined) q = q.startAfter(cursor);
    const snap = await q.get();
    if (snap.empty) break;
    const v = snap.docs[0].get(field) as string | undefined;
    if (typeof v !== "string" || v === "") break;
    values.push(v);
    cursor = v;
  }

  return values;
}

export const discoverTypes = () => distinctValues("type");
export const discoverEnvironments = () => distinctValues("env");
