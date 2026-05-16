"use client";

import { TaskError } from "@/lib/types/oncetask";

interface TaskErrorsTimelineProps {
  errors: TaskError[];
}

// TaskErrorsTimeline renders the task's error history as a vertical timeline,
// latest attempt first.
export function TaskErrorsTimeline({ errors }: TaskErrorsTimelineProps) {
  if (!errors || errors.length === 0) return null;
  const reversed = [...errors].reverse();

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-muted-foreground">
        Errors ({errors.length})
      </h3>
      <ol className="space-y-2 border-l-2 border-destructive/30 pl-4">
        {reversed.map((e, i) => (
          <li key={i} className="space-y-0.5">
            <div className="text-xs text-muted-foreground font-mono">
              {formatDate(e.at)}
            </div>
            <div className="text-sm whitespace-pre-wrap break-words">
              {e.error}
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

function formatDate(iso: string): string {
  if (!iso) return "-";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}
