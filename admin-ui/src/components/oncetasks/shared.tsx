// Shared helpers used by both the Browse view (OnceTaskList) and the
// Index-aware view's sub-views. Pure presentation utilities.

import { isNonZeroTime } from "@/lib/types/oncetask";
import type { OrderBy } from "@/hooks/useTasks";

export function sortArrow(orderBy: OrderBy, field: OrderBy["field"]) {
  if (orderBy.field !== field) return null;
  return (
    <span className="ml-1 text-muted-foreground">
      {orderBy.direction === "asc" ? "▲" : "▼"}
    </span>
  );
}

export function ErrorBanner({ message }: { message: string }) {
  const parts = message.split(/(https?:\/\/\S+)/g);
  return (
    <div className="rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive break-all">
      <span className="font-medium">Error loading tasks: </span>
      {parts.map((p, i) =>
        /^https?:\/\//.test(p) ? (
          <a
            key={i}
            href={p}
            target="_blank"
            rel="noreferrer"
            className="underline underline-offset-2 hover:no-underline"
          >
            {p}
          </a>
        ) : (
          <span key={i}>{p}</span>
        ),
      )}
    </div>
  );
}

export function formatDate(s: string): string {
  if (!isNonZeroTime(s)) return "-";
  try {
    return new Date(s).toLocaleString();
  } catch {
    return s;
  }
}
