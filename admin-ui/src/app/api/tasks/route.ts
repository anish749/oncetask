import { NextRequest, NextResponse } from "next/server";
import { listTasks } from "@/lib/oncetask/queries";
import { TaskStatus } from "@/lib/types/oncetask";

export async function GET(req: NextRequest) {
  const params = req.nextUrl.searchParams;
  const status = params.get("status") as TaskStatus | null;
  const type = params.get("type");
  const env = params.get("env");
  const resourceKey = params.get("resourceKey");
  const limit = params.get("limit");
  const orderBy = params.get("orderBy");
  const orderDirRaw = params.get("orderDir");
  const orderDir =
    orderDirRaw === "asc" || orderDirRaw === "desc" ? orderDirRaw : undefined;

  try {
    const result = await listTasks({
      status: status ?? undefined,
      type: type ?? undefined,
      env: env ?? undefined,
      resourceKey: resourceKey ?? undefined,
      limit: limit ? Number(limit) : undefined,
      orderBy: orderBy ?? undefined,
      orderDir,
    });
    return NextResponse.json(result);
  } catch (err) {
    console.error("listTasks failed", err);
    return NextResponse.json(
      { error: err instanceof Error ? err.message : String(err) },
      { status: 500 },
    );
  }
}
