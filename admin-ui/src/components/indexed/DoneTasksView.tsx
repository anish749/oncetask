"use client";

import { useMemo, useState } from "react";
import {
  TaskStatus,
  formatTaskType,
  getTaskStatus,
} from "@/lib/types/oncetask";
import { useDoneTasks } from "@/hooks/useIndexedTasks";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { TaskStatusBadge } from "@/components/oncetasks/TaskStatusBadge";
import { TaskLinksInline } from "@/components/oncetasks/TaskLinks";
import { ErrorBanner, FetchInfo, formatDate } from "@/components/oncetasks/shared";

type DoneStatus = "all" | TaskStatus.COMPLETED | TaskStatus.FAILED | TaskStatus.CANCELLED;

const STATUS_LABEL: Record<DoneStatus, string> = {
  all: "All",
  [TaskStatus.COMPLETED]: "Completed",
  [TaskStatus.FAILED]: "Failed",
  [TaskStatus.CANCELLED]: "Cancelled",
};

const STATUS_OPTIONS: DoneStatus[] = [
  "all",
  TaskStatus.COMPLETED,
  TaskStatus.FAILED,
  TaskStatus.CANCELLED,
];

interface Props {
  selectedTaskId: string | null;
  onSelectTask: (id: string) => void;
}

export function DoneTasksView({ selectedTaskId, onSelectTask }: Props) {
  const { tasks, hasMore, isLoading, error } = useDoneTasks();
  const [status, setStatus] = useState<DoneStatus>("all");

  const filtered = useMemo(() => {
    if (status === "all") return tasks;
    return tasks.filter((t) => getTaskStatus(t) === status);
  }, [tasks, status]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <label className="text-xs font-medium text-muted-foreground">
          Status
        </label>
        <div className="flex gap-1 rounded-md border p-0.5 w-fit">
          {STATUS_OPTIONS.map((s) => (
            <button
              key={s}
              type="button"
              onClick={() => setStatus(s)}
              className={`px-2.5 py-1 text-sm rounded ${
                status === s
                  ? "bg-primary text-primary-foreground"
                  : "hover:bg-muted"
              }`}
            >
              {STATUS_LABEL[s]}
            </button>
          ))}
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        Most recently completed tasks across all environments and types,
        ordered by Done At ▼. Status filter is applied client-side over the
        returned rows.
      </p>
      {tasks.length > 0 && (
        <FetchInfo shown={filtered.length} fetched={tasks.length} hasMore={hasMore} />
      )}

      {error ? (
        <ErrorBanner message={error.message} />
      ) : isLoading && tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          Loading tasks…
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          {tasks.length === 0
            ? "No done tasks found"
            : `No tasks match status "${STATUS_LABEL[status]}" (showing ${tasks.length} done in total)`}
        </div>
      ) : (
        <div className="border rounded-md">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Environment</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Done At ▼</TableHead>
                <TableHead>Resource Key</TableHead>
                <TableHead>Links</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((task) => (
                <TableRow
                  key={task.id}
                  className={`cursor-pointer ${selectedTaskId === task.id ? "bg-muted" : ""}`}
                  onClick={() => onSelectTask(task.id)}
                >
                  <TableCell className="font-medium">
                    {formatTaskType(task.type)}
                  </TableCell>
                  <TableCell>
                    <TaskStatusBadge status={getTaskStatus(task)} />
                  </TableCell>
                  <TableCell>{task.env}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.createdAt)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.doneAt)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground max-w-[200px] truncate">
                    {task.resourceKey || "-"}
                  </TableCell>
                  <TableCell onClick={(e) => e.stopPropagation()}>
                    <TaskLinksInline task={task} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
