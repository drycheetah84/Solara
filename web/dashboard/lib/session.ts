import { createHash, createHmac, randomBytes, timingSafeEqual } from "node:crypto";

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

function signature(payload: string, config: NonNullable<ReturnType<typeof authConfig>>) {
  // Rotating either credential invalidates existing sessions.
  return createHmac("sha256", config.sessionSecret)
    .update(JSON.stringify(["halcyon-dashboard", config.adminKey, payload]))
    .digest("hex");
}

export function createSession(): string {
  const config = authConfig();
  if (!config) throw new Error("Dashboard authentication is not configured");
  const expires = Math.floor(Date.now() / 1000) + SESSION_MAX_AGE;
  const payload = `v1.${expires}.${randomBytes(16).toString("hex")}`;
  return `${payload}.${signature(payload, config)}`;
}

export function validSession(value: string | undefined): boolean {
  const config = authConfig();
  if (!config || !value) return false;
  const match = /^(v1\.(\d{10})\.[a-f0-9]{32})\.([a-f0-9]{64})$/.exec(value);
  if (!match) return false;
  const expires = Number(match[2]);
  const now = Math.floor(Date.now() / 1000);
  if (expires <= now || expires > now + SESSION_MAX_AGE) return false;
  return timingSafeEqual(
    Buffer.from(match[3], "hex"),
    Buffer.from(signature(match[1], config), "hex"),
  );
}
