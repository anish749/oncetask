"use client";

import { useCallback, useState } from "react";
import { OnceTaskList } from "@/components/oncetasks/OnceTaskList";
import { OnceTaskDetail } from "@/components/oncetasks/OnceTaskDetail";

export default function Page() {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const handleDeleted = useCallback(() => {
    setSelectedId(null);
  }, []);

  return (
    <div className="flex flex-col h-screen">
      <header className="border-b px-4 py-3 flex items-baseline gap-3">
        <h1 className="text-xl font-semibold">OnceTask Admin</h1>
        <span className="text-xs text-muted-foreground">
          oncetask execution monitor
        </span>
      </header>

      <div className="flex flex-1 overflow-hidden">
        <div className="w-2/3 border-r p-4 overflow-auto">
          <OnceTaskList selectedTaskId={selectedId} onSelectTask={setSelectedId} />
        </div>
        <div className="w-1/3 overflow-hidden">
          <OnceTaskDetail taskId={selectedId} onDeleted={handleDeleted} />
        </div>
      </div>
    </div>
  );
}
