"use client";

import { useState } from "react";
import { formatTaskType, getTaskStatus } from "@/lib/types/oncetask";
import { useResourceTasks } from "@/hooks/useIndexedTasks";
import { useTaskMetadata } from "@/hooks/useTaskMetadata";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
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

interface Props {
  selectedTaskId: string | null;
  onSelectTask: (id: string) => void;
}

export function ResourceTasksView({ selectedTaskId, onSelectTask }: Props) {
  const { metadata } = useTaskMetadata();
  const [env, setEnv] = useState<string>("");
  const [keyInput, setKeyInput] = useState<string>("");
  const [committedKey, setCommittedKey] = useState<string>("");

  const args = env && committedKey ? { env, resourceKey: committedKey } : null;
  const { tasks, hasMore, isLoading, error } = useResourceTasks(args);

  const submit = () => setCommittedKey(keyInput.trim());

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-3 items-end flex-wrap">
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

        <div className="flex flex-col gap-1 flex-1 min-w-[260px]">
          <label className="text-xs font-medium text-muted-foreground">
            Resource key <span className="text-destructive">*</span>
          </label>
          <Input
            value={keyInput}
            onChange={(e) => setKeyInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") submit();
            }}
            placeholder="Exact resource key…"
          />
        </div>

        <Button onClick={submit} disabled={!env || !keyInput.trim()}>
          Look up
        </Button>
      </div>

      <p className="text-xs text-muted-foreground">
        Full history (any status) for a single resource key. Mirrors Go&apos;s
        byResourceKey query.
      </p>
      {args && tasks.length > 0 && (
        <FetchInfo
          shown={tasks.length}
          fetched={tasks.length}
          hasMore={hasMore}
        />
      )}

      {!args ? (
        <div className="text-muted-foreground text-center py-8">
          Pick an environment and enter a resource key to look up.
        </div>
      ) : error ? (
        <ErrorBanner message={error.message} />
      ) : isLoading && tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          Loading tasks…
        </div>
      ) : tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          No tasks found for this resource key in {env}
        </div>
      ) : (
        <div className="border rounded-md">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Type</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Created</TableHead>
                <TableHead>Wait Until</TableHead>
                <TableHead>Leased Until</TableHead>
                <TableHead>Done At</TableHead>
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
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.createdAt)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.waitUntil)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.leasedUntil)}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDate(task.doneAt)}
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
