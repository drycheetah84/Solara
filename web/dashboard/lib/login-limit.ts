// Brute-force protection for POST /dash-login (Solara). Mirrors the Go dashboard's own lockout
// (internal/ares/dashboard_hardening.go): 10 failed attempts per client in 5 minutes -> 429 for 15 minutes.
// In-memory and per-process, which matches a single `next start` behind the tunnel.
const WINDOW_MS = 5 * 60 * 1000;
const LOCK_MS = 15 * 60 * 1000;
const MAX_FAILS = 10;
const MAX_TRACKED = 10_000;

type Entry = { fails: number[]; lockedUntil: number };
const clients = new Map<string, Entry>();

/** Client identity: Cloudflare's connecting IP when behind the tunnel, else the first X-Forwarded-For hop. */
export function clientKey(headers: Headers): string {
  const cf = headers.get("cf-connecting-ip");
  if (cf) return cf.trim();
  const xff = headers.get("x-forwarded-for");
  if (xff) return xff.split(",")[0].trim();
  return "local";
}

/** Seconds until the client may try again, or 0 when it is not locked out. */
export function lockedFor(key: string, now = Date.now()): number {
  const e = clients.get(key);
  if (!e || e.lockedUntil <= now) return 0;
  return Math.ceil((e.lockedUntil - now) / 1000);
}

export function recordFailure(key: string, now = Date.now()) {
  if (!clients.has(key) && clients.size >= MAX_TRACKED) {
    // Drop the oldest entry so a flood of spoofed keys cannot grow the map without bound.
    const oldest = clients.keys().next().value;
    if (oldest !== undefined) clients.delete(oldest);
  }
  const e = clients.get(key) ?? { fails: [], lockedUntil: 0 };
  e.fails = e.fails.filter(t => now - t < WINDOW_MS);
  e.fails.push(now);
  if (e.fails.length >= MAX_FAILS) {
    e.lockedUntil = now + LOCK_MS;
    e.fails = [];
  }
  clients.set(key, e);
}

export function recordSuccess(key: string) {
  clients.delete(key);
}
