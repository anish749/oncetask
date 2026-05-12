// OnceTask type definitions mirroring github.com/anish749/oncetask/oncetask/fs_model.go.
// Task type strings are treated as opaque - this admin UI is intentionally generic and
// does not hardcode the consumer's task type enum.

export const NO_WAIT = "0001-01-01T00:00:00Z";

export interface TaskError {
  at: string;
  error: string;
}

export interface Recurrence {
  rrule: string;
  dtstart: string;
  exdates: string[];
}

export interface OnceTask {
  id: string;
  type: string;
  data: Record<string, unknown>;
  resourceKey: string;
  env: string;
  waitUntil: string;
  leasedUntil: string;
  createdAt: string;
  doneAt: string;
  attempts: number;
  errors: TaskError[];
  result: unknown;
  recurrence: Recurrence | null;
  parentRecurrenceId: string;
  occurrenceTimestamp: string;
  isCancelled: boolean;
  cancelledAt: string;
}

export enum TaskStatus {
  WAITING = "waiting",
  PENDING = "pending",
  LEASED = "leased",
  CANCELLATION_PENDING = "cancellationPending",
  COMPLETED = "completed",
  FAILED = "failed",
  CANCELLED = "cancelled",
}

export function isNonZeroTime(timestamp: string): boolean {
  return !!timestamp && !timestamp.startsWith("0001-01-01");
}

// getTaskStatus derives a task's current status from its persisted fields.
// Mirrors pkg/oncetask/task_status.go logic.
export function getTaskStatus(task: OnceTask): TaskStatus {
  const now = new Date();

  if (isNonZeroTime(task.doneAt)) {
    if (task.isCancelled) return TaskStatus.CANCELLED;
    const errorCount = task.errors?.length ?? 0;
    const attempts = task.attempts ?? 0;
    return attempts > errorCount ? TaskStatus.COMPLETED : TaskStatus.FAILED;
  }

  if (task.isCancelled) return TaskStatus.CANCELLATION_PENDING;

  if (isNonZeroTime(task.leasedUntil)) {
    if (new Date(task.leasedUntil) > now) return TaskStatus.LEASED;
  }

  if (isNonZeroTime(task.waitUntil)) {
    if (new Date(task.waitUntil) > now) return TaskStatus.WAITING;
  }

  return TaskStatus.PENDING;
}

// Format a camelCase task type string as a human label: "incomingEmail" -> "Incoming Email".
export function formatTaskType(type: string): string {
  if (!type) return "";
  return type
    .replace(/([A-Z])/g, " $1")
    .replace(/^./, (c) => c.toUpperCase())
    .trim();
}
