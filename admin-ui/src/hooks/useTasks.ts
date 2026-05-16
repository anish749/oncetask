"use client";

import { useQuery } from "@tanstack/react-query";
import { OnceTask, TaskStatus } from "@/lib/types/oncetask";

export interface TaskFilters {
  status?: TaskStatus;
  type?: string;
  env?: string;
  resourceKey?: string;
}

const POLL_MS = 3000;

async function fetchTasks(filters: TaskFilters): Promise<OnceTask[]> {
  const params = new URLSearchParams();
  if (filters.status) params.set("status", filters.status);
  if (filters.type) params.set("type", filters.type);
  if (filters.env) params.set("env", filters.env);
  if (filters.resourceKey) params.set("resourceKey", filters.resourceKey);

  const res = await fetch(`/api/tasks?${params.toString()}`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  const data = (await res.json()) as { tasks: OnceTask[] };
  return data.tasks;
}

export function useTasks(filters: TaskFilters) {
  return useQuery({
    queryKey: ["tasks", filters] as const,
    queryFn: () => fetchTasks(filters),
    // Don't poll or retry once a query has errored; the user usually needs to
    // act (e.g. follow a Firestore "create index" link) before the next request
    // can succeed. Changing filters produces a new key and a fresh attempt.
    refetchInterval: (query) => (query.state.error ? false : POLL_MS),
    retry: false,
  });
}
