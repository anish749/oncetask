import { NextResponse } from "next/server";
import { cancelTask, MutationError } from "@/lib/oncetask/mutations";

export async function POST(
  _req: Request,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  try {
    await cancelTask(id);
    return NextResponse.json({ success: true });
  } catch (err) {
    if (err instanceof MutationError) {
      return NextResponse.json(
        { error: err.message, code: err.code },
        { status: 404 },
      );
    }
    console.error("cancelTask failed", err);
    return NextResponse.json(
      { error: err instanceof Error ? err.message : String(err) },
      { status: 500 },
    );
  }
}
