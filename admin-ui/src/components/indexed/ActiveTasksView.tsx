"use client";

import { useMemo, useState } from "react";
import {
  TaskStatus,
  formatTaskType,
  getTaskStatus,
} from "@/lib/types/oncetask";
import { useActiveTasks } from "@/hooks/useIndexedTasks";
import { useTaskMetadata } from "@/hooks/useTaskMetadata";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import {
  ErrorBanner,
  FetchInfo,
  formatDate,
} from "@/components/oncetasks/shared";

type Sub = "all" | "ready" | "leased" | "waiting";

const SUB_LABEL: Record<Sub, string> = {
  all: "All",
  ready: "Ready",
  leased: "Leased",
  waiting: "Waiting",
};

interface Props {
  selectedTaskId: string | null;
  onSelectTask: (id: string) => void;
}

export function ActiveTasksView({ selectedTaskId, onSelectTask }: Props) {
  const { metadata } = useTaskMetadata();
  const [env, setEnv] = useState<string>("");
  const [type, setType] = useState<string>("");
  const [sub, setSub] = useState<Sub>("all");

  const args = env && type ? { env, type } : null;
  const { tasks, hasMore, isLoading, error } = useActiveTasks(args);

  const filtered = useMemo(() => {
    if (sub === "all") return tasks;
    const target: TaskStatus =
      sub === "ready"
        ? TaskStatus.PENDING
        : sub === "leased"
          ? TaskStatus.LEASED
          : TaskStatus.WAITING;
    return tasks.filter((t) => getTaskStatus(t) === target);
  }, [tasks, sub]);

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-3 flex-wrap items-end">
        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-muted-foreground">
            Environment <span className="text-destructive">*</span>
          </label>
          <Select value={env} onValueChange={(v) => setEnv(v ?? "")}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Pick env…" />
            </SelectTrigger>
            <SelectContent>
              {metadata.environments.map((e) => (
                <SelectItem key={e} value={e}>
                  {e}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="flex flex-col gap-1">
          <label className="text-xs font-medium text-muted-foreground">
            Task type <span className="text-destructive">*</span>
          </label>
          <Select value={type} onValueChange={(v) => setType(v ?? "")}>
            <SelectTrigger className="w-[220px]">
              <SelectValue placeholder="Pick type…">
                {(v: string | null) => (v ? formatTaskType(v) : "Pick type…")}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
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
            Sub-state
          </label>
          <div className="flex gap-1 rounded-md border p-0.5">
            {(["all", "ready", "leased", "waiting"] as const).map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => setSub(s)}
                className={`px-2.5 py-1 text-sm rounded ${
                  sub === s
                    ? "bg-primary text-primary-foreground"
                    : "hover:bg-muted"
                }`}
              >
                {SUB_LABEL[s]}
              </button>
            ))}
          </div>
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        Sorted by leasedUntil ▲, waitUntil ▲ (mirrors Go&apos;s readyTasks
        ordering). Sub-state filter is applied client-side over the returned
        rows.
      </p>
      {args && tasks.length > 0 && (
        <FetchInfo
          shown={filtered.length}
          fetched={tasks.length}
          hasMore={hasMore}
        />
      )}

      {!args ? (
        <div className="text-muted-foreground text-center py-8">
          Pick an environment and task type to load tasks.
        </div>
      ) : error ? (
        <ErrorBanner message={error.message} />
      ) : isLoading && tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          Loading tasks…
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          {tasks.length === 0
            ? "No active tasks for this env + type"
            : `No tasks match sub-state "${SUB_LABEL[sub]}" (showing ${tasks.length} active in total)`}
        </div>
      ) : (
        <div className="border rounded-md">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Leased Until</TableHead>
                <TableHead>Wait Until</TableHead>
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
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.createdAt)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.leasedUntil)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.waitUntil)}
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
