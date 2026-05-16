"use client";

import { useQuery } from "@tanstack/react-query";

export interface TaskMetadata {
  types: string[];
  environments: string[];
}

const EMPTY: TaskMetadata = { types: [], environments: [] };

async function fetchMetadata(): Promise<TaskMetadata> {
  const res = await fetch("/api/metadata");
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export function useTaskMetadata() {
  const query = useQuery({
    queryKey: ["metadata"] as const,
    queryFn: fetchMetadata,
    staleTime: 60_000, // metadata changes rarely; cache for a minute
  });
  return {
    metadata: query.data ?? EMPTY,
    loading: query.isLoading,
    error: query.error,
  };
}
