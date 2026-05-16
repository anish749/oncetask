import "server-only";
import { COLLECTION, getFirestore } from "@/lib/firestore";
import {
  OnceTask,
  TaskStatus,
  getTaskStatus,
  isNonZeroTime,
} from "@/lib/types/oncetask";

export interface ListTasksFilters {
  status?: TaskStatus;
  type?: string;
  env?: string;
  resourceKey?: string;
  limit?: number;
}

export interface ListTasksResult {
  tasks: OnceTask[];
}

// listTasks fetches tasks from Firestore with optional filters.
// Env is just another filter, like type or status - the admin tool is not scoped
// to a single env. Operator picks env in the UI.
export async function listTasks(filters: ListTasksFilters = {}): Promise<ListTasksResult> {
  const db = getFirestore();
  let q: FirebaseFirestore.Query = db.collection(COLLECTION);

  if (filters.env) {
    q = q.where("env", "==", filters.env);
  }
  if (filters.type) {
    q = q.where("type", "==", filters.type);
  }

  // Pre-filter on server side where status maps cleanly to a stored field.
  if (filters.status) {
    const now = new Date().toISOString();
    if (
      filters.status === TaskStatus.COMPLETED ||
      filters.status === TaskStatus.FAILED ||
      filters.status === TaskStatus.CANCELLED
    ) {
      q = q.where("doneAt", "!=", "");
    } else if (filters.status === TaskStatus.LEASED) {
      q = q.where("leasedUntil", ">", now);
    }
  }

  const limit = filters.limit ?? 500;
  const snapshot = await q.limit(limit).get();

  let tasks: OnceTask[] = snapshot.docs.map((doc) => ({
    id: doc.id,
    ...(doc.data() as Omit<OnceTask, "id">),
  }));

  // Exact-match status filter (derived).
  if (filters.status) {
    tasks = tasks.filter((t) => getTaskStatus(t) === filters.status);
  }

  // Substring filter for resourceKey.
  if (filters.resourceKey && filters.resourceKey.trim()) {
    const needle = filters.resourceKey.trim().toLowerCase();
    tasks = tasks.filter((t) => t.resourceKey?.toLowerCase().includes(needle));
  }

  // Sort by createdAt desc.
  tasks.sort((a, b) => {
    const ta = isNonZeroTime(a.createdAt) ? new Date(a.createdAt).getTime() : 0;
    const tb = isNonZeroTime(b.createdAt) ? new Date(b.createdAt).getTime() : 0;
    return tb - ta;
  });

  return { tasks };
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
