"use client";

import { ExternalLink } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { OnceTask } from "@/lib/types/oncetask";
import { evaluateLinks } from "@/lib/links/evaluate";
import { useLinksConfig } from "@/hooks/useLinksConfig";
import { cn } from "@/lib/utils";

interface TaskLinksProps {
  task: OnceTask;
}

// TaskLinks renders every applicable link from the YAML config for a task.
// Used in the detail pane.
export function TaskLinks({ task }: TaskLinksProps) {
  const config = useLinksConfig();
  const links = evaluateLinks(task, config);

  if (links.length === 0) return null;

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium text-muted-foreground">Links</h3>
      <div className="flex flex-col gap-1">
        {links.map((link, i) => (
          <a
            key={`${link.label}-${i}`}
            href={link.url}
            target="_blank"
            rel="noopener noreferrer"
            className={cn(
              buttonVariants({ variant: "outline", size: "sm" }),
              "justify-start",
            )}
          >
            <ExternalLink className="mr-2 h-3 w-3" />
            {link.label}
          </a>
        ))}
      </div>
    </div>
  );
}

// TaskLinksInline renders only `primary: true` links as small icons for the list row.
export function TaskLinksInline({ task }: TaskLinksProps) {
  const config = useLinksConfig();
  const links = evaluateLinks(task, config).filter((l) => l.primary);

  if (links.length === 0) return <span className="text-muted-foreground">-</span>;

  return (
    <div className="flex gap-1">
      {links.map((link, i) => (
        <a
          key={`${link.label}-${i}`}
          href={link.url}
          target="_blank"
          rel="noopener noreferrer"
          title={link.label}
          onClick={(e) => e.stopPropagation()}
          className={cn(buttonVariants({ variant: "ghost", size: "icon-sm" }))}
        >
          <ExternalLink className="h-3 w-3" />
        </a>
      ))}
    </div>
  );
}
