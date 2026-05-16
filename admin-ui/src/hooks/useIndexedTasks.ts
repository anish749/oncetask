"use client";

import { useQuery } from "@tanstack/react-query";
import { OnceTask } from "@/lib/types/oncetask";

const POLL_MS = 3000;

async function fetchIndexed(params: URLSearchParams): Promise<OnceTask[]> {
  const res = await fetch(`/api/indexed?${params.toString()}`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  const data = (await res.json()) as { tasks: OnceTask[] };
  return data.tasks;
}

// Shared react-query options: stop polling/retrying on error so the user can
// act on (e.g.) a Firestore "create index" link without a loop. Matches
// useTasks behavior in the browse view.
const sharedOptions = {
  refetchInterval: <T,>(q: { state: { error: unknown } }): number | false =>
    q.state.error ? false : POLL_MS,
  retry: false as const,
};

export function useActiveTasks(args: { env: string; type: string } | null) {
  return useQuery({
    queryKey: ["indexed", "active", args] as const,
    queryFn: () => {
      if (!args) throw new Error("env and type required");
      const p = new URLSearchParams({
        mode: "active",
        env: args.env,
        type: args.type,
      });
      return fetchIndexed(p);
    },
    enabled: args !== null,
    ...sharedOptions,
  });
}

export function useDoneTasks() {
  return useQuery({
    queryKey: ["indexed", "done"] as const,
    queryFn: () => fetchIndexed(new URLSearchParams({ mode: "done" })),
    ...sharedOptions,
  });
}

export function useResourceTasks(
  args: { env: string; resourceKey: string } | null,
) {
  return useQuery({
    queryKey: ["indexed", "resource", args] as const,
    queryFn: () => {
      if (!args) throw new Error("env and resourceKey required");
      const p = new URLSearchParams({
        mode: "resource",
        env: args.env,
        resourceKey: args.resourceKey,
      });
      return fetchIndexed(p);
    },
    enabled: args !== null,
    ...sharedOptions,
  });
}
