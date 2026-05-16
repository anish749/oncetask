"use client";

import { useState } from "react";
import { BrowseView } from "./oncetasks/BrowseView";
import { IndexedView } from "./indexed/IndexedView";

type Tab = "browse" | "indexed";

const TAB_LABEL: Record<Tab, string> = {
  browse: "Browse",
  indexed: "Index-aware",
};

// TopLevelTabs is the only place that knows which page-level view is active.
// BrowseView and IndexedView don't know they're inside tabs — they just render.
export function TopLevelTabs() {
  const [tab, setTab] = useState<Tab>("browse");

  return (
    <div className="flex flex-col h-screen">
      <header className="border-b px-4 py-3 flex items-center gap-4">
        <h1 className="text-xl font-semibold">OnceTask Admin</h1>
        <nav className="flex gap-1">
          {(["browse", "indexed"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setTab(t)}
              className={`px-3 py-1 text-sm rounded-md ${
                tab === t
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted"
              }`}
            >
              {TAB_LABEL[t]}
            </button>
          ))}
        </nav>
      </header>
      {tab === "browse" ? <BrowseView /> : <IndexedView />}
    </div>
  );
}
