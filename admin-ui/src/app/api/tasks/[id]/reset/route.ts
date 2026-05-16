import { NextResponse } from "next/server";
import { resetTask, MutationError } from "@/lib/oncetask/mutations";

export async function POST(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  try {
    await resetTask(id);
    return NextResponse.json({ success: true });
  } catch (err) {
    if (err instanceof MutationError) {
      const status = err.code === "NOT_FOUND" ? 404 : 409;
      return NextResponse.json(
        { error: err.message, code: err.code },
        { status },
      );
    }
    console.error("resetTask failed", err);
    return NextResponse.json(
      { error: err instanceof Error ? err.message : String(err) },
      { status: 500 },
    );
  }
}
