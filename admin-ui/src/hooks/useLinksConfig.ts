"use client";

import { useQuery } from "@tanstack/react-query";
import { EMPTY_LINKS_CONFIG, LinksConfig } from "@/lib/links/types";

async function fetchLinksConfig(): Promise<LinksConfig> {
  const res = await fetch("/api/links");
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

export function useLinksConfig(): LinksConfig {
  const query = useQuery({
    queryKey: ["links-config"] as const,
    queryFn: fetchLinksConfig,
    staleTime: Infinity, // config only changes on server restart
  });
  return query.data ?? EMPTY_LINKS_CONFIG;
}
