"use client";

import { useState } from "react";
import { RotateCcw, Ban, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { OnceTask, TaskStatus, getTaskStatus } from "@/lib/types/oncetask";

interface TaskActionsProps {
  task: OnceTask;
  onDeleted: () => void;
}

type Action = "reset" | "cancel" | "delete";

const labels: Record<
  Action,
  { title: string; confirm: string; description: (id: string) => string }
> = {
  reset: {
    title: "Reset this task?",
    confirm: "Reset task",
    description: (id) =>
      `${id} will be cleared of all execution state (attempts, errors, result, cancellation) and made immediately available for re-execution.`,
  },
  cancel: {
    title: "Cancel this task?",
    confirm: "Cancel task",
    description: (id) =>
      `${id} will be marked as cancelled. If a cancellation handler is registered, it will run.`,
  },
  delete: {
    title: "Delete this task?",
    confirm: "Delete",
    description: (id) =>
      `${id} will be permanently deleted from Firestore. If the task is currently leased, the running handler will fail when it tries to complete.`,
  },
};

export function TaskActions({ task, onDeleted }: TaskActionsProps) {
  const [pending, setPending] = useState<Action | null>(null);
  const queryClient = useQueryClient();

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["tasks"] });
  };

  const mutation = useMutation({
    mutationFn: async (action: Action) => {
      const path =
        action === "delete"
          ? `/api/tasks/${task.id}`
          : `/api/tasks/${task.id}/${action}`;
      const res = await fetch(path, {
        method: action === "delete" ? "DELETE" : "POST",
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || `HTTP ${res.status}`);
      }
      return action;
    },
    onSuccess: (action) => {
      invalidate();
      toast.success(
        action === "reset"
          ? "Task reset"
          : action === "cancel"
            ? "Task cancelled"
            : "Task deleted",
      );
      if (action === "delete") onDeleted();
      setPending(null);
    },
    onError: (err: Error) => {
      toast.error(err.message);
      setPending(null);
    },
  });

  const status = getTaskStatus(task);
  const isTerminal =
    status === TaskStatus.COMPLETED ||
    status === TaskStatus.FAILED ||
    status === TaskStatus.CANCELLED;
  const canCancel =
    status !== TaskStatus.COMPLETED &&
    status !== TaskStatus.FAILED &&
    status !== TaskStatus.CANCELLED &&
    status !== TaskStatus.CANCELLATION_PENDING;

  return (
    <>
      <div className="flex gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={() => setPending("reset")}
          disabled={!isTerminal}
        >
          <RotateCcw />
          Reset
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => setPending("cancel")}
          disabled={!canCancel}
        >
          <Ban />
          Cancel
        </Button>
        <Button
          size="sm"
          variant="outline"
          onClick={() => setPending("delete")}
        >
          <Trash2 />
          Delete
        </Button>
      </div>

      <Dialog
        open={pending !== null}
        onOpenChange={(open) =>
          !open && !mutation.isPending && setPending(null)
        }
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{pending && labels[pending].title}</DialogTitle>
            <DialogDescription>
              {pending && labels[pending].description(task.id)}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setPending(null)}
              disabled={mutation.isPending}
            >
              Cancel
            </Button>
            <Button
              variant={pending === "delete" ? "destructive" : "default"}
              onClick={() => pending && mutation.mutate(pending)}
              disabled={mutation.isPending}
            >
              {mutation.isPending
                ? "Working..."
                : pending && labels[pending].confirm}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
