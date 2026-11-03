// Discord sign-in for the dashboard. A member of the configured Discord server gets the dashboard access their roles
// map to (DISCORD_ROLE_MAP); admins get a master session, everyone else a session backed by a short-lived Solara
// scoped token minted for their permissions, so Solara itself enforces them.
//
// Environment (/etc/halcyon/dashboard.env):
//   DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET   the Discord application (OAuth2)
//   DISCORD_GUILD_ID                           the Discord server whose roles count
//   DISCORD_ROLE_MAP                           JSON: { "<role id>": "admin" | { "permissions": [...], "stations": [...] } }
//                                              a station entry "@owned" means "the fleets this Discord user owns" (below)
//   DISCORD_FLEET_OWNERS                       JSON: { "<discord user id>": ["<station id>", ...] }  (community captains)
//   DISCORD_ROLE_NAMES                         JSON: { "<role id>": "Developer", ... }  (shown on the profile page)
//   DASHBOARD_PUBLIC_URL                       default https://dash.halcy0n.net (the OAuth redirect is <url>/auth/discord/callback)
import { createHash } from "node:crypto";

export type RoleGrant = "admin" | { permissions: string[]; stations: string[] };
export type Access = { admin: true } | { admin: false; permissions: string[]; stations: string[] };

export function discordConfig() {
  const clientId = process.env.DISCORD_CLIENT_ID ?? "";
  const clientSecret = process.env.DISCORD_CLIENT_SECRET ?? "";
  const guildId = process.env.DISCORD_GUILD_ID ?? "";
  if (!clientId || !clientSecret || !guildId) return null;
  let roleMap: Record<string, RoleGrant> = {};
  try { roleMap = JSON.parse(process.env.DISCORD_ROLE_MAP ?? "{}"); } catch { roleMap = {}; }
  let owners: Record<string, string[]> = {};
  try { owners = JSON.parse(process.env.DISCORD_FLEET_OWNERS ?? "{}"); } catch { owners = {}; }
  let roleNames: Record<string, string> = {};
  try { roleNames = JSON.parse(process.env.DISCORD_ROLE_NAMES ?? "{}"); } catch { roleNames = {}; }
  const base = (process.env.DASHBOARD_PUBLIC_URL ?? "https://dash.halcy0n.net").replace(/\/+$/, "");
  return { clientId, clientSecret, guildId, roleMap, owners, roleNames, redirectUri: `${base}/auth/discord/callback` };
}

/** An absolute URL on the dashboard's public origin (the server itself may only see its internal address). */
export function publicUrl(path: string): URL {
  const base = (process.env.DASHBOARD_PUBLIC_URL ?? "https://dash.halcy0n.net").replace(/\/+$/, "");
  return new URL(path, base + "/");
}

/** Everything a member's roles grant, as station-scoped entries ("perm@station"), so one role's permissions never
 *  spread to another role's stations; null when no role grants anything. "admin" wins outright. */
export function accessForRoles(roles: string[], roleMap: Record<string, RoleGrant>, userId = "",
  owners: Record<string, string[]> = {}): Access | null {
  const entries = new Set<string>();
  for (const r of roles) {
    const g = roleMap[r];
    if (!g) continue;
    if (g === "admin") return { admin: true };
    // "@owned" -> this user's own fleets; a grant whose stations resolve to nothing grants nothing
    const st = (g.stations ?? []).flatMap(s => s === "@owned" ? (owners[userId] ?? []) : [s]);
    for (const p of g.permissions ?? []) for (const s of st) entries.add(`${p}@${s}`);
  }
  if (entries.size === 0) return null;
  return { admin: false, permissions: [...entries].sort(), stations: [] };
}

/** Hash of the state value kept in the httpOnly cookie (the raw state only travels through Discord). */
export function stateHash(state: string): string {
  return createHash("sha256").update(state).digest("hex");
}

export type DiscordMember = { userId: string; name: string; roles: string[]; avatar: string };

async function discordFetch(url: string, init: RequestInit): Promise<Response> {
  return fetch(url, { ...init, cache: "no-store", redirect: "manual", signal: AbortSignal.timeout(8000) });
}

/** Exchange the OAuth code and read the user's membership (roles) in the configured server. */
export async function discordMember(code: string, cfg: NonNullable<ReturnType<typeof discordConfig>>): Promise<DiscordMember | "not-member" | null> {
  const tok = await discordFetch("https://discord.com/api/oauth2/token", {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      grant_type: "authorization_code", code, redirect_uri: cfg.redirectUri,
      client_id: cfg.clientId, client_secret: cfg.clientSecret,
    }),
  });
  if (!tok.ok) return null;
  const t = await tok.json().catch(() => null) as { access_token?: string } | null;
  if (!t?.access_token) return null;
  const m = await discordFetch(`https://discord.com/api/users/@me/guilds/${encodeURIComponent(cfg.guildId)}/member`, {
    headers: { authorization: `Bearer ${t.access_token}` },
  });
  if (m.status === 404) return "not-member";
  if (!m.ok) return null;
  const body = await m.json().catch(() => null) as {
    roles?: string[]; nick?: string | null; avatar?: string | null;
    user?: { id?: string; username?: string; global_name?: string | null; avatar?: string | null };
  } | null;
  if (!body?.user?.id) return null;
  const uid = body.user.id;
  const safe = (h?: string | null) => (h && /^(a_)?[a-f0-9]{32}$/.test(h) ? h : null);
  // server avatar > account avatar > Discord's default avatar
  const guildAvatar = safe(body.avatar), userAvatar = safe(body.user.avatar);
  const avatar = guildAvatar
    ? `https://cdn.discordapp.com/guilds/${cfg.guildId}/users/${uid}/avatars/${guildAvatar}.png?size=128`
    : userAvatar
      ? `https://cdn.discordapp.com/avatars/${uid}/${userAvatar}.png?size=128`
      : `https://cdn.discordapp.com/embed/avatars/${Number((BigInt(uid) >> BigInt(22)) % BigInt(6))}.png`;
  return {
    userId: uid,
    name: body.nick || body.user.global_name || body.user.username || uid,
    roles: Array.isArray(body.roles) ? body.roles : [],
    avatar,
  };
}
