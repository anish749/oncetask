import { NextResponse } from "next/server";
import { loadLinksConfig } from "@/lib/links/config";

export async function GET() {
  const config = loadLinksConfig();
  return NextResponse.json(config);
}
