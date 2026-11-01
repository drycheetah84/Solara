import { NextRequest, NextResponse } from "next/server";
import { authConfig, createSession, SESSION_COOKIE, SESSION_MAX_AGE, validAdminKey } from "@/lib/session";
import { clientKey, lockedFor, recordFailure, recordSuccess } from "@/lib/login-limit";

// The existing login protocol stays POST /dash-login with JSON { key }.
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
  if (!validAdminKey(key)) {
    recordFailure(client);
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }
  recordSuccess(client);

  // HTTPS may terminate at a reverse proxy; HTTP also supports local SSH tunnels.
  const proto = req.headers.get("x-forwarded-proto") ?? req.nextUrl.protocol.replace(":", "");
  const res = NextResponse.json({ ok: true }, { headers: { "cache-control": "no-store" } });
  res.cookies.set(SESSION_COOKIE, createSession(), {
    httpOnly: true,
    sameSite: "lax",
    secure: proto.split(",")[0].trim() === "https",
    path: "/",
    maxAge: SESSION_MAX_AGE,
  });
  return res;
}

export const dynamic = "force-dynamic";
