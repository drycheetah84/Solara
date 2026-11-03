import { createCipheriv, createDecipheriv, createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";

export const SESSION_COOKIE = "halcyon_dash";
export const SESSION_MAX_AGE = 60 * 60 * 12;

export function authConfig() {
  const adminKey = process.env.DASHBOARD_ADMIN_KEY;
  const sessionSecret = process.env.DASHBOARD_SESSION_SECRET;
  if (!adminKey || !sessionSecret) return null;
  return { adminKey, sessionSecret };
}

export function validAdminKey(key: unknown): boolean {
  const config = authConfig();
  if (!config || typeof key !== "string" || !key || key.length > 4096) return false;
  // Equal-length hashes keep arbitrary input lengths away from timingSafeEqual.
  return timingSafeEqual(
    createHash("sha256").update(key).digest(),
    createHash("sha256").update(config.adminKey).digest(),
  );
}

/** Solara scoped dashboard tokens (backend /api/scoped-tokens) look like sdt1_<64 hex>. */
export function looksLikeScopedToken(key: unknown): key is string {
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

/** A master session (the admin key). */
export function createSession(): string {
  const config = authConfig();
  if (!config) throw new Error("Dashboard authentication is not configured");
  const expires = Math.floor(Date.now() / 1000) + SESSION_MAX_AGE;
  const payload = `v1.${expires}.${randomBytes(16).toString("hex")}`;
  return `${payload}.${signature(payload, config)}`;
}

/** A session that acts with a scoped dashboard token (already checked against the backend). */
export function createScopedSession(token: string): string {
  const config = authConfig();
  if (!config) throw new Error("Dashboard authentication is not configured");
  const expires = Math.floor(Date.now() / 1000) + SESSION_MAX_AGE;
  const payload = `v2.${expires}.${randomBytes(16).toString("hex")}.${sealToken(token, config)}`;
  return `${payload}.${signature(payload, config)}`;
}

export type SessionCredential = { kind: "master"; key: string } | { kind: "scoped"; key: string };

/** The credential a session acts with, or null when the cookie is missing, forged or expired. */
export function sessionCredential(value: string | undefined): SessionCredential | null {
  const config = authConfig();
  if (!config || !value) return null;
  const match = /^(v([12])\.(\d{10})\.[a-f0-9]{32}(?:\.([A-Za-z0-9_-]{40,200}))?)\.([a-f0-9]{64})$/.exec(value);
  if (!match) return null;
  const [, payload, version, exp, sealed, sig] = match;
  if ((version === "1") !== (sealed === undefined)) return null;
  const expires = Number(exp);
  const now = Math.floor(Date.now() / 1000);
  if (expires <= now || expires > now + SESSION_MAX_AGE) return null;
  if (!timingSafeEqual(Buffer.from(sig, "hex"), Buffer.from(signature(payload, config), "hex"))) return null;
  if (version === "1") return { kind: "master", key: config.adminKey };
  const token = openToken(sealed as string, config);
  return token ? { kind: "scoped", key: token } : null;
}

export function validSession(value: string | undefined): boolean {
  return sessionCredential(value) !== null;
}
