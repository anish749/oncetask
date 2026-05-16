"use client";

import { useState } from "react";
import {
  OnceTask,
  TaskStatus,
  formatTaskType,
  getTaskStatus,
  isNonZeroTime,
} from "@/lib/types/oncetask";
import { useTasks } from "@/hooks/useTasks";
import { useTaskMetadata } from "@/hooks/useTaskMetadata";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { TaskStatusBadge } from "./TaskStatusBadge";
import { TaskLinksInline } from "./TaskLinks";

interface OnceTaskListProps {
  selectedTaskId: string | null;
  onSelectTask: (taskId: string) => void;
}

export function OnceTaskList({ selectedTaskId, onSelectTask }: OnceTaskListProps) {
  const [statusFilter, setStatusFilter] = useState<TaskStatus | "all">("all");
  const [typeFilter, setTypeFilter] = useState<string>("all");
  const [envFilter, setEnvFilter] = useState<string>("all");
  const [resourceKeySearch, setResourceKeySearch] = useState<string>("");

  const { metadata } = useTaskMetadata();

  const { data: tasks = [], isLoading, error } = useTasks({
    status: statusFilter === "all" ? undefined : statusFilter,
    type: typeFilter === "all" ? undefined : typeFilter,
    env: envFilter === "all" ? undefined : envFilter,
    resourceKey: resourceKeySearch.trim() || undefined,
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <div className="flex gap-3 flex-wrap">
          <div className="flex flex-col gap-1">
            <label className="text-xs font-medium text-muted-foreground">
              Status
            </label>
            <Select
              value={statusFilter as never}
              onValueChange={(v) =>
                setStatusFilter((v ?? "all") as TaskStatus | "all")
              }
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="All" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                <SelectItem value={TaskStatus.WAITING}>Waiting</SelectItem>
                <SelectItem value={TaskStatus.PENDING}>Pending</SelectItem>
                <SelectItem value={TaskStatus.LEASED}>Leased</SelectItem>
                <SelectItem value={TaskStatus.CANCELLATION_PENDING}>
                  Cancelling
                </SelectItem>
                <SelectItem value={TaskStatus.COMPLETED}>Completed</SelectItem>
                <SelectItem value={TaskStatus.FAILED}>Failed</SelectItem>
                <SelectItem value={TaskStatus.CANCELLED}>Cancelled</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-xs font-medium text-muted-foreground">
              Task type
            </label>
            <Select
              value={typeFilter}
              onValueChange={(v) => setTypeFilter(v ?? "all")}
            >
              <SelectTrigger className="w-[200px]">
                <SelectValue placeholder="All">
                  {(value: string | null) =>
                    !value || value === "all" ? "All" : formatTaskType(value)
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                {metadata.types.map((t) => (
                  <SelectItem key={t} value={t}>
                    {formatTaskType(t)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-xs font-medium text-muted-foreground">
              Environment
            </label>
            <Select
              value={envFilter}
              onValueChange={(v) => setEnvFilter(v ?? "all")}
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="All" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All</SelectItem>
                {metadata.environments.map((e) => (
                  <SelectItem key={e} value={e}>
                    {e}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>

        <Input
          placeholder="Search by resource key..."
          value={resourceKeySearch}
          onChange={(e) => setResourceKeySearch(e.target.value)}
        />
      </div>

      {error ? (
        <ErrorBanner message={error.message} />
      ) : isLoading && tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">Loading tasks...</div>
      ) : tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">No tasks found</div>
      ) : (
        <div className="border rounded-md">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Environment</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Wait Until</TableHead>
                <TableHead>Done At</TableHead>
                <TableHead>Resource Key</TableHead>
                <TableHead>Links</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tasks.map((task) => (
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
                    {formatDate(task.waitUntil)}
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

function ErrorBanner({ message }: { message: string }) {
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

function formatDate(s: string): string {
  if (!isNonZeroTime(s)) return "-";
  try {
    return new Date(s).toLocaleString();
  } catch {
    return s;
  }
}
