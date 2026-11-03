import { NextRequest, NextResponse } from "next/server";
import { SESSION_COOKIE, validSession } from "./lib/session";

// Next 16 Proxy runs in Node.js, so verify the signature and expiry here. Presence of an
// httpOnly cookie alone proves nothing: an HTTP client can send any cookie value it wants.
// Solara: state-changing requests must come from this dashboard's own pages. The session cookie is
// SameSite=Lax, which already stops most cross-site POSTs; checking Origin also covers same-site
// subdomains (anything else under the same registrable domain) and method-override tricks.
const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

function sameOrigin(req: NextRequest): boolean {
  const origin = req.headers.get("origin");
  if (!origin) {
    // Browsers always send Origin on cross-origin POST/PUT/PATCH/DELETE; a missing header means a
    // same-origin navigation or a non-browser client (which has no ambient cookie to abuse).
    const site = req.headers.get("sec-fetch-site");
    return !site || site === "same-origin" || site === "none";
  }
  const host = req.headers.get("x-forwarded-host") ?? req.headers.get("host");
  try {
    return new URL(origin).host === host;
  } catch {
    return false;
  }
}

export function proxy(req: NextRequest) {
  const { pathname } = req.nextUrl;
  if (!SAFE_METHODS.has(req.method.toUpperCase()) && !sameOrigin(req)) {
    return NextResponse.json({ error: "cross-origin request refused" }, { status: 403 });
  }
  if (
    pathname === "/login" ||
    pathname === "/dash-login" ||
    pathname === "/auth/discord" ||
    pathname === "/auth/discord/callback" ||
    pathname.startsWith("/_next/") ||
    pathname === "/favicon.ico" ||
    pathname === "/logo.webp"
  ) {
    return NextResponse.next();
  }

  if (!validSession(req.cookies.get(SESSION_COOKIE)?.value)) {
    if (pathname.startsWith("/api") || pathname.startsWith("/game")) {
      return NextResponse.json({ error: "unauthorized" }, { status: 401 });
    }
    const url = req.nextUrl.clone();
    url.pathname = "/login";
    url.search = "";
    return NextResponse.redirect(url);
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/((?!_next/static|_next/image|favicon.ico).*)"],
};
