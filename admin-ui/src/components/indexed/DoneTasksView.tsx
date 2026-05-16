"use client";

import {
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
import { ErrorBanner, formatDate } from "@/components/oncetasks/shared";

interface Props {
  selectedTaskId: string | null;
  onSelectTask: (id: string) => void;
}

export function DoneTasksView({ selectedTaskId, onSelectTask }: Props) {
  const { data: tasks = [], isLoading, error } = useDoneTasks();

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-muted-foreground">
        The 500 most recently completed tasks across all environments and
        types, ordered by Done At ▼.
      </p>

      {error ? (
        <ErrorBanner message={error.message} />
      ) : isLoading && tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          Loading tasks…
        </div>
      ) : tasks.length === 0 ? (
        <div className="text-muted-foreground text-center py-8">
          No done tasks found
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
