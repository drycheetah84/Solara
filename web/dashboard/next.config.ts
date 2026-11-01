import type { NextConfig } from "next";

// Where the backend is running. Set BACKEND_HOST to point the UI somewhere else; on the VPS
// everything is loopback.
//   /api   -> handled by app/api/[...path]/route.ts, NOT a rewrite. The backend enforces auth on
//             /api/*, so the request needs the admin key attached SERVER-SIDE; a plain rewrite
//             cannot add a header, and putting the key in the browser would defeat the point.
//   /auth  -> AresDashboardServer :8080 (Meta SSO round-trip; no key needed)
//   /game  -> handled by app/game/[...path]/route.ts with session verification.
const BACKEND_HOST = process.env.BACKEND_HOST ?? "127.0.0.1";
const DASHBOARD_PORT = process.env.DASHBOARD_API_PORT ?? "8080";

// Solara: browser hardening for an admin panel. No third-party origins are used, so the CSP is
// self-only ('unsafe-inline' is required by Next's inline bootstrap scripts and the inline styles
// the pages use; images also allow data:/blob: for board previews).
const securityHeaders = [
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Referrer-Policy", value: "same-origin" },
  { key: "Permissions-Policy", value: "camera=(), microphone=(), geolocation=(), payment=()" },
  { key: "Strict-Transport-Security", value: "max-age=31536000" },
  {
    key: "Content-Security-Policy",
    value: [
      "default-src 'self'",
      "script-src 'self' 'unsafe-inline'",
      "style-src 'self' 'unsafe-inline'",
      "img-src 'self' data: blob: https: http:",
      "font-src 'self' data:",
      "connect-src 'self'",
      "frame-ancestors 'none'",
      "base-uri 'self'",
      "form-action 'self'",
      "object-src 'none'",
    ].join("; "),
  },
];

const nextConfig: NextConfig = {
  poweredByHeader: false,
  output: "standalone",
  async headers() {
    return [{ source: "/:path*", headers: securityHeaders }];
  },
  async rewrites() {
    return [
      { source: "/auth/:path*", destination: `http://${BACKEND_HOST}:${DASHBOARD_PORT}/auth/:path*` },
    ];
  },
};

export default nextConfig;
