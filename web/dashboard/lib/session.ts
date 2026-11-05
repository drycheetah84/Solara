import { createCipheriv, createDecipheriv, createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";

export const SESSION_COOKIE = "halcyon_dash";
export const SESSION_MAX_AGE = 60 * 60 * 12;

export function authConfig() {
  const adminKey = process.env.DASHBOARD_ADMIN_KEY;
  const sessionSecret = process.env.DASHBOARD_SESSION_SECRET;
  if (!adminKey || !sessionSecret) return null;
  return { adminKey, sessionSecret };
}

/** Solara scoped dashboard tokens (backend /api/scoped-tokens) look like sdt1_<64 hex>. */
function looksLikeScopedToken(key: unknown): key is string {
  return typeof key === "string" && /^sdt1_[a-f0-9]{64}$/.test(key);
}

function signature(payload: string, config: NonNullable<ReturnType<typeof authConfig>>) {
  // Rotating either credential invalidates existing sessions.
  return createHmac("sha256", config.sessionSecret)
    .update(JSON.stringify(["halcyon-dashboard", config.adminKey, payload]))
    .digest("hex");
}

// A scoped-token session carries the token, encrypted (AES-256-GCM) under a key derived from the session secret,
// so the httpOnly cookie never holds it in the clear. The HMAC signature still covers the whole payload.
function tokenKey(config: NonNullable<ReturnType<typeof authConfig>>) {
  return createHash("sha256").update(JSON.stringify(["halcyon-dashboard-token", config.sessionSecret])).digest();
}

function sealToken(token: string, config: NonNullable<ReturnType<typeof authConfig>>): string {
  const iv = randomBytes(12);
  const c = createCipheriv("aes-256-gcm", tokenKey(config), iv);
  const enc = Buffer.concat([c.update(token, "utf8"), c.final()]);
  return Buffer.concat([iv, c.getAuthTag(), enc]).toString("base64url");
}

function openToken(sealed: string, config: NonNullable<ReturnType<typeof authConfig>>): string | null {
  try {
    const b = Buffer.from(sealed, "base64url");
    if (b.length < 29) return null;
    const d = createDecipheriv("aes-256-gcm", tokenKey(config), b.subarray(0, 12));
    d.setAuthTag(b.subarray(12, 28));
    const t = Buffer.concat([d.update(b.subarray(28)), d.final()]).toString("utf8");
    return looksLikeScopedToken(t) ? t : null;
  } catch {
    return null;
  }
}

/** The Discord identity a session belongs to (signed into the cookie with the rest of the session). */
export type SessionProfile = { id: string; name: string; avatar: string | null; roles: string[]; admin: boolean };

function encodeProfile(p: SessionProfile): string {
  return Buffer.from(JSON.stringify(p), "utf8").toString("base64url");
}
function decodeProfile(s: string): SessionProfile | null {
  try {
    const p = JSON.parse(Buffer.from(s, "base64url").toString("utf8"));
    if (!p || typeof p.id !== "string" || typeof p.name !== "string") return null;
    return { id: p.id, name: p.name, avatar: typeof p.avatar === "string" ? p.avatar : null,
      roles: Array.isArray(p.roles) ? p.roles.filter((r: unknown) => typeof r === "string") : [], admin: !!p.admin };
  } catch {
    return null;
  }
}

/** A Discord session: master (admin role) when token is null, otherwise acting with that scoped token.
 *  Format v3: v3.<exp>.<nonce>.<sealed token | "-">.<profile>.<hmac> */
export function createSession(profile: SessionProfile, token: string | null = null): string {
  const config = authConfig();
  if (!config) throw new Error("Dashboard authentication is not configured");
  const expires = Math.floor(Date.now() / 1000) + SESSION_MAX_AGE;
  const payload = `v3.${expires}.${randomBytes(16).toString("hex")}.${token ? sealToken(token, config) : "-"}.${encodeProfile(profile)}`;
  return `${payload}.${signature(payload, config)}`;
}

export type SessionCredential = { kind: "master" | "scoped"; key: string; profile: SessionProfile; expires: number };

/** The credential a session acts with, or null when the cookie is missing, forged, expired or from an older format
 *  (key-login sessions v1 / v2 are no longer accepted: the dashboard signs in with Discord only). */
export function sessionCredential(value: string | undefined): SessionCredential | null {
  const config = authConfig();
  if (!config || !value || value.length > 4096) return null;
  const match = /^(v3\.(\d{10})\.[a-f0-9]{32}\.(-|[A-Za-z0-9_-]{40,200})\.([A-Za-z0-9_-]{2,2000}))\.([a-f0-9]{64})$/.exec(value);
  if (!match) return null;
  const [, payload, exp, sealed, prof, sig] = match;
  const expires = Number(exp);
  const now = Math.floor(Date.now() / 1000);
  if (expires <= now || expires > now + SESSION_MAX_AGE) return null;
  if (!timingSafeEqual(Buffer.from(sig, "hex"), Buffer.from(signature(payload, config), "hex"))) return null;
  const profile = decodeProfile(prof);
  if (!profile) return null;
  if (sealed === "-") return profile.admin ? { kind: "master", key: config.adminKey, profile, expires } : null;
  const token = openToken(sealed, config);
  return token ? { kind: "scoped", key: token, profile, expires } : null;
}

export function validSession(value: string | undefined): boolean {
  return sessionCredential(value) !== null;
}

/** Solara API keys: a scoped dashboard token sent by a program (a Discord bot...) instead of a browser session, as
 *  `Authorization: Bearer sdt1_...` or `x-api-key: sdt1_...`. Only scoped tokens: the master admin key is never
 *  accepted from a header, so a leaked bot key carries only the permissions and stations an admin gave it. Only the
 *  shape is checked here; the backend checks the token itself (unknown, revoked, expired -> 401) and its scope. */
export function headerScopedToken(headers: Headers): string | null {
  const auth = headers.get("authorization");
  const bearer = auth && /^Bearer\s+(\S+)$/i.exec(auth.trim());
  const key = bearer ? bearer[1] : headers.get("x-api-key")?.trim();
  return looksLikeScopedToken(key) ? key : null;
}
