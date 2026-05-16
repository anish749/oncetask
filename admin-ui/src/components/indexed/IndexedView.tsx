"use client";

import { useCallback, useState } from "react";
import { OnceTaskDetail } from "@/components/oncetasks/OnceTaskDetail";
import { ActiveTasksView } from "./ActiveTasksView";
import { DoneTasksView } from "./DoneTasksView";
import { ResourceTasksView } from "./ResourceTasksView";

type Mode = "active" | "done" | "resource";

const MODE_LABEL: Record<Mode, string> = {
  active: "Active",
  done: "Done",
  resource: "Resource",
};

// IndexedView is the index-respecting admin view. Each sub-mode issues only
// queries that already have a Firestore index from production Go code, so
// using it never prompts you to create a new composite index.
export function IndexedView() {
  const [mode, setMode] = useState<Mode>("active");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const handleDeleted = useCallback(() => setSelectedId(null), []);

  // Selection is per-mode — switching modes shouldn't keep a stale selection.
  const handleSetMode = (next: Mode) => {
    if (next !== mode) setSelectedId(null);
    setMode(next);
  };

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <div className="border-b px-4 pt-2 flex gap-1">
        {(["active", "done", "resource"] as const).map((m) => (
          <button
            key={m}
            type="button"
            onClick={() => handleSetMode(m)}
            className={`px-3 py-1.5 text-sm rounded-t-md border-b-2 -mb-px ${
              mode === m
                ? "border-primary font-medium"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            {MODE_LABEL[m]}
          </button>
        ))}
      </div>

      <div className="flex flex-1 overflow-hidden">
        <div className="w-2/3 border-r p-4 overflow-auto">
          {mode === "active" ? (
            <ActiveTasksView
              selectedTaskId={selectedId}
              onSelectTask={setSelectedId}
            />
          ) : mode === "done" ? (
            <DoneTasksView
              selectedTaskId={selectedId}
              onSelectTask={setSelectedId}
            />
          ) : (
            <ResourceTasksView
              selectedTaskId={selectedId}
              onSelectTask={setSelectedId}
            />
          )}
        </div>
        <div className="w-1/3 overflow-hidden">
          <OnceTaskDetail taskId={selectedId} onDeleted={handleDeleted} />
        </div>
      </div>
    </div>
  );
}
