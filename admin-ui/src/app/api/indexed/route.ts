import { NextRequest, NextResponse } from "next/server";
import {
  listActiveTasks,
  listDoneTasks,
  listResourceTasks,
} from "@/lib/oncetask/indexedQueries";

// Dispatcher for the index-aware admin view. mode picks one of three query
// shapes; each shape only uses Firestore indexes that already exist for the
// production Go code.

export async function GET(req: NextRequest) {
  const params = req.nextUrl.searchParams;
  const mode = params.get("mode");

  try {
    if (mode === "active") {
      const env = params.get("env");
      const type = params.get("type");
      if (!env || !type) {
        return NextResponse.json(
          { error: "env and type are required for mode=active" },
          { status: 400 },
        );
      }
      const result = await listActiveTasks({ env, type });
      return NextResponse.json(result);
    }

    if (mode === "done") {
      const result = await listDoneTasks();
      return NextResponse.json(result);
    }

    if (mode === "resource") {
      const env = params.get("env");
      const resourceKey = params.get("resourceKey");
      if (!env || !resourceKey) {
        return NextResponse.json(
          { error: "env and resourceKey are required for mode=resource" },
          { status: 400 },
        );
      }
      const result = await listResourceTasks({ env, resourceKey });
      return NextResponse.json(result);
    }

    return NextResponse.json(
      { error: `unknown mode: ${mode ?? "(missing)"}` },
      { status: 400 },
    );
  } catch (err) {
    console.error(`indexed mode=${mode} failed`, err);
    return NextResponse.json(
      { error: err instanceof Error ? err.message : String(err) },
      { status: 500 },
    );
  }
}
