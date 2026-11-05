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
