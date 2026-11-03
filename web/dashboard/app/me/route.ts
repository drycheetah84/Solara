import { NextRequest, NextResponse } from "next/server";
import { SESSION_COOKIE, sessionCredential } from "@/lib/session";

// GET /me -> the signed-in Discord profile and what the session may do. For a scoped session the permissions come
// from Solara itself (GET /api/scoped-tokens/self with the session's own token), so they are exactly what is enforced.
export async function GET(req: NextRequest) {
  const cred = sessionCredential(req.cookies.get(SESSION_COOKIE)?.value);
  if (!cred) return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  let permissions: string[] = [];
  let stations: string[] = [];
  if (cred.kind === "scoped") {
    const host = process.env.BACKEND_HOST ?? "127.0.0.1";
    const port = process.env.DASHBOARD_API_PORT ?? "8080";
    try {
      const r = await fetch(`http://${host}:${port}/api/scoped-tokens/self`, {
        headers: { "x-dashboard-key": cred.key }, cache: "no-store", signal: AbortSignal.timeout(5000),
      });
      if (r.ok) {
        const b = await r.json() as { permissions?: string[]; stations?: string[] };
        permissions = b.permissions ?? [];
        stations = b.stations ?? [];
      }
    } catch { /* profile still renders without the permission list */ }
  }
  return NextResponse.json({
    id: cred.profile.id, name: cred.profile.name, avatar: cred.profile.avatar, roles: cred.profile.roles,
    admin: cred.kind === "master", permissions, stations, expires_at: new Date(cred.expires * 1000).toISOString(),
  }, { headers: { "cache-control": "no-store" } });
}

export const dynamic = "force-dynamic";
