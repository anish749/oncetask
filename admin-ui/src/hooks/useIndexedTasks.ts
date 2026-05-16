"use client";

import { useQuery } from "@tanstack/react-query";
import { OnceTask } from "@/lib/types/oncetask";

const POLL_MS = 3000;

interface IndexedResult {
  tasks: OnceTask[];
  hasMore: boolean;
}

async function fetchIndexed(params: URLSearchParams): Promise<IndexedResult> {
  const res = await fetch(`/api/indexed?${params.toString()}`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `HTTP ${res.status}`);
  }
  return (await res.json()) as IndexedResult;
}

// Shared react-query options: stop polling/retrying on error so the user can
// act on (e.g.) a Firestore "create index" link without a loop. Matches
// useTasks behavior in the browse view.
const sharedOptions = {
  refetchInterval: <T,>(q: { state: { error: unknown } }): number | false =>
    q.state.error ? false : POLL_MS,
  retry: false as const,
};

function unpack<T extends { data?: IndexedResult }>(query: T) {
  return {
    ...query,
    tasks: query.data?.tasks ?? [],
    hasMore: query.data?.hasMore ?? false,
  };
}

export function useActiveTasks(args: { env: string; type: string } | null) {
  return unpack(
    useQuery({
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
    }),
  );
}

export function useDoneTasks() {
  return unpack(
    useQuery({
      queryKey: ["indexed", "done"] as const,
      queryFn: () => fetchIndexed(new URLSearchParams({ mode: "done" })),
      ...sharedOptions,
    }),
  );
}

export function useResourceTasks(
  args: { env: string; resourceKey: string } | null,
) {
  return unpack(
    useQuery({
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
    }),
  );
}
