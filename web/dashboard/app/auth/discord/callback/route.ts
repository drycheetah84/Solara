import { NextRequest, NextResponse } from "next/server";
import { timingSafeEqual } from "node:crypto";
import { accessForRoles, discordConfig, discordMember, publicUrl, stateHash } from "@/lib/discord";
import { authConfig, createSession, SESSION_COOKIE, SESSION_MAX_AGE, type SessionProfile } from "@/lib/session";
import { clientKey, lockedFor, recordFailure, recordSuccess } from "@/lib/login-limit";

const STATE_COOKIE = "halcyon_dstate";

function back(req: NextRequest, err: string) {
  const res = NextResponse.redirect(publicUrl(`/login?err=${encodeURIComponent(err)}`));
  res.cookies.set(STATE_COOKIE, "", { path: "/auth/discord", maxAge: 0 });
  return res;
}

// Mint a short-lived Solara scoped token for this Discord user (revoking their previous ones) with the master key.
async function mintToken(adminKey: string, label: string, permissions: string[], stations: string[]): Promise<string | null> {
  const host = process.env.BACKEND_HOST ?? "127.0.0.1";
  const port = process.env.DASHBOARD_API_PORT ?? "8080";
  const base = `http://${host}:${port}/api/scoped-tokens`;
  const headers = { "x-dashboard-key": adminKey, "content-type": "application/json" };
  try {
    const list = await fetch(base, { headers, cache: "no-store", signal: AbortSignal.timeout(5000) });
    if (list.ok) {
      const body = await list.json().catch(() => ({})) as { tokens?: { id: string; name: string; revoked_at: string | null }[] };
      for (const t of body.tokens ?? [])
        if (t.name === label && !t.revoked_at)
          await fetch(`${base}/${encodeURIComponent(t.id)}`, { method: "DELETE", headers, signal: AbortSignal.timeout(5000) }).catch(() => null);
    }
    const r = await fetch(base, {
      method: "POST", headers, cache: "no-store", signal: AbortSignal.timeout(5000),
      body: JSON.stringify({ name: label, permissions, stations, expires_in_minutes: Math.floor(SESSION_MAX_AGE / 60) }),
    });
    if (!r.ok) return null;
    const out = await r.json().catch(() => null) as { token?: string } | null;
    return out?.token ?? null;
  } catch {
    return null;
  }
}

// GET /auth/discord/callback?code&state -> session cookie for a member whose roles grant dashboard access.
export async function GET(req: NextRequest) {
  const auth = authConfig();
  const cfg = discordConfig();
  if (!auth || !cfg) return back(req, "discord-not-configured");

  const client = clientKey(req.headers);
  if (lockedFor(client) > 0) return back(req, "too-many-attempts");

  const code = req.nextUrl.searchParams.get("code") ?? "";
  const state = req.nextUrl.searchParams.get("state") ?? "";
  const want = req.cookies.get(STATE_COOKIE)?.value ?? "";
  const got = state ? stateHash(state) : "";
  if (!code || !want || want.length !== got.length || !timingSafeEqual(Buffer.from(want), Buffer.from(got))) {
    recordFailure(client);
    return back(req, req.nextUrl.searchParams.get("error") === "access_denied" ? "discord-cancelled" : "discord-state");
  }

  const member = await discordMember(code, cfg);
  if (member === "not-member") return back(req, "not-in-server");
  if (!member) return back(req, "discord-failed");
  const access = accessForRoles(member.roles, cfg.roleMap, member.userId, cfg.owners);
  if (!access) return back(req, "no-dashboard-role");

  const profile: SessionProfile = {
    id: member.userId, name: member.name, avatar: member.avatar, admin: access.admin,
    roles: member.roles.filter(r => cfg.roleMap[r] !== undefined).map(r => cfg.roleNames[r] ?? r),
  };
  let session: string;
  if (access.admin) {
    session = createSession(profile);
  } else {
    const token = await mintToken(auth.adminKey, `discord:${member.userId} (${member.name})`, access.permissions, access.stations);
    if (!token) return back(req, "token-failed");
    session = createSession(profile, token);
  }
  recordSuccess(client);
  console.log(`[dash-login] discord ${member.userId} (${member.name}) -> ${access.admin ? "admin" : access.permissions.join(",")}`);

  const proto = req.headers.get("x-forwarded-proto") ?? req.nextUrl.protocol.replace(":", "");
  const res = NextResponse.redirect(publicUrl("/"));
  res.cookies.set(SESSION_COOKIE, session, {
    httpOnly: true, sameSite: "lax", secure: proto.split(",")[0].trim() === "https", path: "/", maxAge: SESSION_MAX_AGE,
  });
  res.cookies.set(STATE_COOKIE, "", { path: "/auth/discord", maxAge: 0 });
  return res;
}

export const dynamic = "force-dynamic";
