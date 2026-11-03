import { NextRequest, NextResponse } from "next/server";
import {
  authConfig, createScopedSession, createSession, looksLikeScopedToken, SESSION_COOKIE, SESSION_MAX_AGE, validAdminKey,
} from "@/lib/session";
import { clientKey, lockedFor, recordFailure, recordSuccess } from "@/lib/login-limit";

// Solara: is this a live scoped dashboard token? The backend decides (GET /api/scoped-tokens/self with the token).
async function scopedTokenIsLive(token: string): Promise<boolean> {
  const host = process.env.BACKEND_HOST ?? "127.0.0.1";
  const port = process.env.DASHBOARD_API_PORT ?? "8080";
  try {
    const r = await fetch(`http://${host}:${port}/api/scoped-tokens/self`, {
      headers: { "x-dashboard-key": token }, cache: "no-store", redirect: "manual",
      signal: AbortSignal.timeout(5000),
    });
    if (!r.ok) return false;
    const body = await r.json().catch(() => null);
    return !!body && body.master === false;
  } catch {
    return false;
  }
}

// The existing login protocol stays POST /dash-login with JSON { key }. The key is the master admin key, or a
// scoped dashboard token (Solara), in which case the session acts with that token's permissions only.
export async function POST(req: NextRequest) {
  if (!authConfig()) {
    return NextResponse.json({ error: "Dashboard authentication is not configured" }, { status: 503 });
  }

  const client = clientKey(req.headers);
  const wait = lockedFor(client);
  if (wait > 0) {
    return NextResponse.json({ error: "too many attempts" }, { status: 429, headers: { "retry-after": String(wait) } });
  }

  let body: unknown;
  try {
    body = await req.json();
  } catch {
    return NextResponse.json({ error: "bad body" }, { status: 400 });
  }
  const key = body && typeof body === "object" && "key" in body ? body.key : undefined;
  let session: string | null = null;
  if (validAdminKey(key)) {
    session = createSession();
  } else if (looksLikeScopedToken(key) && await scopedTokenIsLive(key)) {
    session = createScopedSession(key);
  }
  if (!session) {
    recordFailure(client);
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  recordSuccess(client);

  // HTTPS may terminate at a reverse proxy; HTTP also supports local SSH tunnels.
  const proto = req.headers.get("x-forwarded-proto") ?? req.nextUrl.protocol.replace(":", "");
  const res = NextResponse.json({ ok: true }, { headers: { "cache-control": "no-store" } });
  res.cookies.set(SESSION_COOKIE, session, {
    httpOnly: true,
    sameSite: "lax",
    secure: proto.split(",")[0].trim() === "https",
    path: "/",
    maxAge: SESSION_MAX_AGE,
  });
  return res;
}

export const dynamic = "force-dynamic";
