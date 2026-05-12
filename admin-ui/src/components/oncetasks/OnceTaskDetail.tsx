"use client";

import { getTaskStatus, formatTaskType } from "@/lib/types/oncetask";
import { TaskStatusBadge } from "./TaskStatusBadge";
import { TaskActions } from "./TaskActions";
import { TaskLinks } from "./TaskLinks";
import { TaskErrorsTimeline } from "./TaskErrorsTimeline";
import { Separator } from "@/components/ui/separator";
import { useTask } from "@/hooks/useTask";

interface OnceTaskDetailProps {
  taskId: string | null;
  onDeleted: () => void;
}

export function OnceTaskDetail({ taskId, onDeleted }: OnceTaskDetailProps) {
  const { data: task, error } = useTask(taskId);

  if (!taskId) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        Select a task to view details
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full text-destructive">
        Error loading task: {error.message}
      </div>
    );
  }

  if (!task) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        Loading...
      </div>
    );
  }

  const status = getTaskStatus(task);

  return (
    <div className="flex flex-col h-full min-w-0">
      <div className="p-4 border-b space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 flex-1">
            <h2 className="text-lg font-semibold leading-tight">
              {formatTaskType(task.type)}
            </h2>
            <p className="text-xs text-muted-foreground font-mono break-all">
              {task.id}
            </p>
          </div>
          <TaskStatusBadge status={status} />
        </div>
        <TaskActions task={task} onDeleted={onDeleted} />
      </div>

      <div className="flex-1 min-h-0 p-4 overflow-y-auto space-y-4">
        <TaskLinks task={task} />

        {task.errors && task.errors.length > 0 && (
          <>
            <Separator />
            <TaskErrorsTimeline errors={task.errors} />
          </>
        )}

        <Separator />

        <div>
          <h3 className="text-sm font-medium text-muted-foreground mb-2">Task JSON</h3>
          <div className="overflow-x-auto rounded-md">
            <pre className="bg-muted p-4 text-xs whitespace-pre w-fit min-w-full">
              {JSON.stringify(task, null, 2)}
            </pre>
          </div>
        </div>
      </div>
    </div>
  );
}
