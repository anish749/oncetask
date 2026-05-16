"use client";

import { useQuery } from "@tanstack/react-query";
import { OnceTask, TaskStatus } from "@/lib/types/oncetask";

export interface TaskFilters {
  status?: TaskStatus;
  type?: string;
  env?: string;
  resourceKey?: string;
}

export interface OrderBy {
  field: "createdAt" | "doneAt" | "leasedUntil" | "waitUntil";
  direction: "asc" | "desc";
}

// STATUS_DEFAULT_ORDER maps each filter state to the field + direction the UI
// should sort by. Each entry uses an index Firestore already serves for free
// (single-field auto-index, or the same composite the where clauses already
// require) — picking these specifically avoids prompting new indexes.
const STATUS_DEFAULT_ORDER = {
  all: { field: "createdAt", direction: "desc" },
  [TaskStatus.COMPLETED]: { field: "doneAt", direction: "desc" },
  [TaskStatus.FAILED]: { field: "doneAt", direction: "desc" },
  [TaskStatus.CANCELLED]: { field: "doneAt", direction: "desc" },
  [TaskStatus.LEASED]: { field: "leasedUntil", direction: "asc" },
  [TaskStatus.WAITING]: { field: "waitUntil", direction: "asc" },
  [TaskStatus.PENDING]: { field: "waitUntil", direction: "asc" },
  [TaskStatus.CANCELLATION_PENDING]: { field: "createdAt", direction: "desc" },
} as const satisfies Record<string, OrderBy>;

export function orderForStatus(status: TaskStatus | undefined): OrderBy {
  return status ? STATUS_DEFAULT_ORDER[status] : STATUS_DEFAULT_ORDER.all;
}

const POLL_MS = 3000;

async function fetchTasks(
  filters: TaskFilters,
  order: OrderBy,
): Promise<OnceTask[]> {
  const params = new URLSearchParams();
  if (filters.status) params.set("status", filters.status);
  if (filters.type) params.set("type", filters.type);
  if (filters.env) params.set("env", filters.env);
  if (filters.resourceKey) params.set("resourceKey", filters.resourceKey);
  params.set("orderBy", order.field);
  params.set("orderDir", order.direction);

  const res = await fetch(`/api/tasks?${params.toString()}`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  const data = (await res.json()) as { tasks: OnceTask[] };
  return data.tasks;
}

export function useTasks(filters: TaskFilters) {
  const order = orderForStatus(filters.status);
  const query = useQuery({
    queryKey: ["tasks", filters, order] as const,
    queryFn: () => fetchTasks(filters, order),
    // Don't poll or retry once a query has errored; the user usually needs to
    // act (e.g. follow a Firestore "create index" link) before the next request
    // can succeed. Changing filters produces a new key and a fresh attempt.
    refetchInterval: (q) => (q.state.error ? false : POLL_MS),
    retry: false,
  });
  return { ...query, orderBy: order };
}
