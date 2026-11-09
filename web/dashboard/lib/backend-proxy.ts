import { NextRequest, NextResponse } from "next/server";
import { authConfig, headerScopedToken, SESSION_COOKIE, sessionCredential } from "./session";

export async function proxyBackend(req: NextRequest, path: string[], service: "dashboard" | "station") {
  // Check again at the privileged operation, even if the outer Proxy matcher changes.
  const config = authConfig();
  const session = config ? sessionCredential(req.cookies.get(SESSION_COOKIE)?.value) : null;
  // Solara API keys: no browser session, but a scoped token in Authorization: Bearer / x-api-key (a Discord bot...).
  // Dashboard API only -- the station API (:78) does not accept scoped tokens.
  const apiKey = !session && service === "dashboard" ? headerScopedToken(req.headers) : null;
  const cred = session ?? (apiKey ? { key: apiKey } : null);
  if (!config || !cred) {
    return NextResponse.json({ error: "unauthorized" }, { status: 401 });
  }

  const host = process.env.BACKEND_HOST ?? "127.0.0.1";
  const port = service === "dashboard"
    ? process.env.DASHBOARD_API_PORT ?? "8080"
    : process.env.STATION_API_PORT ?? "78";
  const prefix = service === "dashboard" ? "/api/" : "/";
  const target = `http://${host}:${port}${prefix}${path.map(encodeURIComponent).join("/")}${req.nextUrl.search}`;
  const headers = new Headers();
  const contentType = req.headers.get("content-type");
  if (contentType) headers.set("content-type", contentType);
  // Solara: the station API (:78) also guards its routes (KNOWN_ISSUES SEC-F2) and accepts the dashboard
  // admin key in x-dashboard-key as an admin credential, so /game needs it too -- without it every
  // stations/create, deployments and event call came back 401 from the Go server.
  // Solara scoped tokens: a session opened with a scoped dashboard token forwards THAT token, so the backend
  // enforces its permissions and station / fleet scope (the station API on :78 does not accept it at all).
  headers.set("x-dashboard-key", cred.key);

  const method = req.method.toUpperCase();
  const body = method === "GET" || method === "HEAD" ? undefined : Buffer.from(await req.arrayBuffer());
  try {
    const upstream = await fetch(target, { method, headers, body, cache: "no-store", redirect: "manual" });
    const responseBody = [204, 205, 304].includes(upstream.status) ? null : Buffer.from(await upstream.arrayBuffer());
    // Audit log: record every successful mutating action by a signed-in admin (not bot API keys,
    // not the audit route itself). The actor comes from the verified session, never the browser.
    const mutating = method === "POST" || method === "PUT" || method === "PATCH" || method === "DELETE";
    if (mutating && upstream.status >= 200 && upstream.status < 300 && session && path[0] !== "audit") {
      await recordAudit(config, host, {
        actor_id: session.profile.id,
        actor_name: session.profile.name,
        scoped: session.kind === "scoped",
        method,
        service,
        path: "/" + path.join("/") + (req.nextUrl.search ?? ""),
        status: upstream.status,
        detail: auditDetail(contentType, body),
      });
    }
    return new NextResponse(responseBody, {
      status: upstream.status,
      headers: {
        "content-type": upstream.headers.get("content-type") ?? "application/json",
        "cache-control": "no-store",
      },
    });
  } catch {
    return NextResponse.json({ error: "backend unreachable" }, { status: 502 });
  }
}

// A short, safe snapshot of the request body for context (ban reason, perms set). JSON only,
// small bodies only, truncated; never a file upload or a large payload.
function auditDetail(contentType: string | null, body?: Buffer): string {
  if (!body || body.length === 0 || body.length > 2000) return "";
  if (!contentType || !contentType.includes("json")) return "";
  const s = body.toString("utf8");
  return s.length > 500 ? s.slice(0, 500) + "…" : s;
}

type AuditFields = {
  actor_id: string; actor_name: string; scoped: boolean;
  method: string; service: string; path: string; status: number; detail: string;
};

// Fire the audit entry to the dashboard backend (:8080) with the admin key. Best-effort: a failure
// here must never break the action the admin just performed, so it is swallowed with a short timeout.
async function recordAudit(config: NonNullable<ReturnType<typeof authConfig>>, host: string, fields: AuditFields) {
  const port = process.env.DASHBOARD_API_PORT ?? "8080";
  try {
    const ctrl = new AbortController();
    const t = setTimeout(() => ctrl.abort(), 3000);
    await fetch(`http://${host}:${port}/api/audit`, {
      method: "POST",
      headers: { "content-type": "application/json", "x-dashboard-key": config.adminKey },
      body: JSON.stringify(fields),
      cache: "no-store",
      signal: ctrl.signal,
    }).finally(() => clearTimeout(t));
  } catch {
    /* audit is best-effort; never block the admin action on it */
  }
}
