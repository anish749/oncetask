"use client";

import { useQuery } from "@tanstack/react-query";
import { OnceTask } from "@/lib/types/oncetask";

const POLL_MS = 3000;

async function fetchTask(id: string): Promise<OnceTask | null> {
  const res = await fetch(`/api/tasks/${encodeURIComponent(id)}`);
  if (res.status === 404) return null;
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  const data = (await res.json()) as { task: OnceTask };
  return data.task;
}

export function useTask(id: string | null) {
  return useQuery({
    queryKey: ["tasks", "by-id", id] as const,
    queryFn: () => fetchTask(id!),
    enabled: !!id,
    refetchInterval: POLL_MS,
  });
}
