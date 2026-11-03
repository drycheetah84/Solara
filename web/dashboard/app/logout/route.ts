import { NextRequest, NextResponse } from "next/server";
import { authConfig, SESSION_COOKIE, sessionCredential } from "@/lib/session";

// POST /logout -> revoke the session's scoped token (so it stops working at once) and clear the cookie.
export async function POST(req: NextRequest) {
  const cred = sessionCredential(req.cookies.get(SESSION_COOKIE)?.value);
  const auth = authConfig();
  if (cred?.kind === "scoped" && auth) {
    const host = process.env.BACKEND_HOST ?? "127.0.0.1";
    const port = process.env.DASHBOARD_API_PORT ?? "8080";
    const prefix = cred.key.slice(0, "sdt1_".length + 8);   // Solara accepts the display prefix as the token id
    await fetch(`http://${host}:${port}/api/scoped-tokens/${encodeURIComponent(prefix)}`, {
      method: "DELETE", headers: { "x-dashboard-key": auth.adminKey }, signal: AbortSignal.timeout(5000),
    }).catch(() => null);
  }
  const res = NextResponse.json({ ok: true }, { headers: { "cache-control": "no-store" } });
  res.cookies.set(SESSION_COOKIE, "", { path: "/", maxAge: 0 });
  return res;
}

export const dynamic = "force-dynamic";
