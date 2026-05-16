import { Badge } from "@/components/ui/badge";
import { TaskStatus } from "@/lib/types/oncetask";

interface TaskStatusBadgeProps {
  status: TaskStatus;
}

export function TaskStatusBadge({ status }: TaskStatusBadgeProps) {
  const variant = getVariantForStatus(status);
  const label = getLabelForStatus(status);

  return <Badge variant={variant}>{label}</Badge>;
}

function getVariantForStatus(
  status: TaskStatus,
): "default" | "secondary" | "destructive" | "outline" {
  switch (status) {
    case TaskStatus.COMPLETED:
      return "default";
    case TaskStatus.LEASED:
      return "secondary";
    case TaskStatus.FAILED:
      return "destructive";
    case TaskStatus.CANCELLED:
    case TaskStatus.CANCELLATION_PENDING:
      return "destructive";
    case TaskStatus.PENDING:
    case TaskStatus.WAITING:
      return "outline";
    default:
      return "outline";
  }
}

function getLabelForStatus(status: TaskStatus): string {
  switch (status) {
    case TaskStatus.COMPLETED:
      return "Completed";
    case TaskStatus.LEASED:
      return "Leased";
    case TaskStatus.FAILED:
      return "Failed";
    case TaskStatus.CANCELLED:
      return "Cancelled";
    case TaskStatus.CANCELLATION_PENDING:
      return "Cancelling";
    case TaskStatus.PENDING:
      return "Pending";
    case TaskStatus.WAITING:
      return "Waiting";
    default:
      return "Unknown";
  }
}
