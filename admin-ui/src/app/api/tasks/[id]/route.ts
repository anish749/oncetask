import { NextResponse } from "next/server";
import { getTask } from "@/lib/oncetask/queries";
import { deleteTask } from "@/lib/oncetask/mutations";

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  try {
    const task = await getTask(id);
    if (!task) return NextResponse.json({ error: "not found" }, { status: 404 });
    return NextResponse.json({ task });
  } catch (err) {
    console.error("getTask failed", err);
    return NextResponse.json(
      { error: err instanceof Error ? err.message : String(err) },
      { status: 500 },
    );
  }
}

export async function DELETE(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  try {
    await deleteTask(id);
    return NextResponse.json({ success: true });
  } catch (err) {
    console.error("deleteTask failed", err);
    return NextResponse.json(
      { error: err instanceof Error ? err.message : String(err) },
      { status: 500 },
    );
  }
}
