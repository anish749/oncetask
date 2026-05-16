"use client";

import { useCallback, useState } from "react";
import { OnceTaskList } from "./OnceTaskList";
import { OnceTaskDetail } from "./OnceTaskDetail";

// BrowseView is the original "filter by anything, JS-derive status" admin UI.
// It freely issues queries that may prompt new Firestore indexes — useful when
// you're willing to create indexes for ad-hoc browsing. For the index-respecting
// alternative, see IndexedView.
export function BrowseView() {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const handleDeleted = useCallback(() => setSelectedId(null), []);

  return (
    <div className="flex flex-1 overflow-hidden">
      <div className="w-2/3 border-r p-4 overflow-auto">
        <OnceTaskList selectedTaskId={selectedId} onSelectTask={setSelectedId} />
      </div>
      <div className="w-1/3 overflow-hidden">
        <OnceTaskDetail taskId={selectedId} onDeleted={handleDeleted} />
      </div>
    </div>
  );
}
