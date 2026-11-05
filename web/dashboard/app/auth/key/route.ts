import { NextRequest, NextResponse } from "next/server";
import { createHash, timingSafeEqual } from "node:crypto";
import { publicUrl } from "@/lib/discord";
import { authConfig, createSession, SESSION_COOKIE, SESSION_MAX_AGE, type SessionProfile } from "@/lib/session";
import { clientKey, lockedFor, recordFailure, recordSuccess } from "@/lib/login-limit";

// Solara: sign in with the dashboard admin key (DASHBOARD_ADMIN_KEY), as an alternative to Discord.
//  * POST only, from the login form: the key travels in the request body, never in the URL (no history, no logs,
//    no Referer), and is never stored in the browser -- what the browser keeps is the same signed, httpOnly,
//    12-hour session cookie a Discord admin gets. The key itself is never logged or echoed back.
//  * Constant-time comparison (SHA-256 of both sides, so the lengths always match).
//  * Shares the Discord sign-in's brute-force lockout (lib/login-limit: 10 failures in 5 minutes -> 15 minutes),
//    and a locked-out client is refused even with the right key.
//  * Cross-site posts are refused by proxy.ts (Origin check) before this runs.
// The admin key grants full access: give people Discord roles or API keys (/tokens) instead where possible.

const MAX_KEY_LENGTH = 512;

function back(err: string) {
  return NextResponse.redirect(publicUrl(`/login?err=${encodeURIComponent(err)}`), 303);
}

function sameKey(given: string, adminKey: string): boolean {
  const a = createHash("sha256").update(given, "utf8").digest();
  const b = createHash("sha256").update(adminKey, "utf8").digest();
  return timingSafeEqual(a, b);
}

export async function POST(req: NextRequest) {
  const auth = authConfig();
  if (!auth) return back("key-not-configured");
  const client = clientKey(req.headers);
  if (lockedFor(client) > 0) return back("too-many-attempts");

  let given = "";
  try {
    const form = await req.formData();
    const v = form.get("key");
    given = typeof v === "string" ? v.trim() : "";
  } catch {
    given = "";
  }
  if (!given || given.length > MAX_KEY_LENGTH || !sameKey(given, auth.adminKey)) {
    recordFailure(client);
    console.log(`[dash-login] admin key: FAILED from ${client}`);
    return back("bad-key");
  }

  recordSuccess(client);
  console.log(`[dash-login] admin key: signed in from ${client}`);
  const profile: SessionProfile = { id: "admin-key", name: "Admin (key)", avatar: null, roles: [], admin: true };
  const proto = req.headers.get("x-forwarded-proto") ?? req.nextUrl.protocol.replace(":", "");
  const res = NextResponse.redirect(publicUrl("/"), 303);
  res.cookies.set(SESSION_COOKIE, createSession(profile), {
    httpOnly: true, sameSite: "lax", secure: proto.split(",")[0].trim() === "https", path: "/", maxAge: SESSION_MAX_AGE,
  });
  return res;
}

export const dynamic = "force-dynamic";
