import { NextRequest } from "next/server";
import { proxyBackend } from "@/lib/backend-proxy";

type Ctx = { params: Promise<{ path: string[] }> };

async function handle(req: NextRequest, ctx: Ctx) {
  return proxyBackend(req, (await ctx.params).path, "station");
}

export { handle as GET, handle as POST, handle as PUT, handle as PATCH, handle as DELETE };
export const dynamic = "force-dynamic";
