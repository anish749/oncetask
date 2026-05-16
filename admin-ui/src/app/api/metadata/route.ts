import { NextResponse } from "next/server";
import { discoverEnvironments, discoverTypes } from "@/lib/oncetask/queries";

export async function GET() {
  try {
    const [types, environments] = await Promise.all([
      discoverTypes(),
      discoverEnvironments(),
    ]);
    return NextResponse.json({ types, environments });
  } catch (err) {
    console.error("metadata failed", err);
    return NextResponse.json(
      { error: err instanceof Error ? err.message : String(err) },
      { status: 500 },
    );
  }
}
