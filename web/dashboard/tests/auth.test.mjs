import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createCipheriv, createHash, createHmac, randomBytes } from "node:crypto";
import { once } from "node:events";
import http from "node:http";
import net from "node:net";
import path from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { setTimeout as delay } from "node:timers/promises";

const cwd = fileURLToPath(new URL("..", import.meta.url));
const adminKey = randomBytes(32).toString("hex");
const sessionSecret = randomBytes(32).toString("hex");
const scopedToken = "sdt1_" + randomBytes(32).toString("hex");

async function freePort() {
  const listener = net.createServer();
  listener.listen(0, "127.0.0.1");
  await once(listener, "listening");
  const port = listener.address().port;
  await new Promise(resolve => listener.close(resolve));
  return port;
}

async function startDashboard(upstreamPort, overrides = {}) {
  const port = await freePort();
  const child = spawn(process.execPath, [
    path.join(cwd, "node_modules/next/dist/bin/next"), "start", "--hostname", "127.0.0.1", "--port", String(port),
  ], {
    cwd,
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"],
    env: {
      ...process.env,
      NODE_ENV: "production",
      NEXT_TELEMETRY_DISABLED: "1",
      DASHBOARD_ADMIN_KEY: adminKey,
      DASHBOARD_SESSION_SECRET: sessionSecret,
      BACKEND_HOST: "127.0.0.1",
      DASHBOARD_API_PORT: String(upstreamPort),
      STATION_API_PORT: String(upstreamPort),
      ...overrides,
    },
  });
  let logs = "";
  child.stdout.on("data", chunk => { logs += chunk; });
  child.stderr.on("data", chunk => { logs += chunk; });
  const origin = `http://127.0.0.1:${port}`;
  const request = (pathname, options = {}) => fetch(origin + pathname, {
    redirect: "manual", signal: AbortSignal.timeout(5000), ...options,
  });
  const stop = async () => {
    if (child.exitCode !== null || child.signalCode !== null) return;
    const exited = once(child, "exit");
    child.kill();
    await exited;
  };
  try {
    for (let attempt = 0; attempt < 100; attempt++) {
      if (child.exitCode !== null) throw new Error(`Dashboard exited during startup: ${logs}`);
      try {
        if ((await request("/login")).status === 200) return { request, stop };
      } catch {}
      await delay(100);
    }
    throw new Error(`Dashboard did not start: ${logs}`);
  } catch (error) {
    await stop();
    throw error;
  }
}

function sign(payload) {
  return createHmac("sha256", sessionSecret).update(JSON.stringify(["halcyon-dashboard", adminKey, payload])).digest("hex");
}

// A v3 (Discord) session exactly as /auth/discord/callback mints it.
function discordCookie({ admin = true, token = null, expires = Math.floor(Date.now() / 1000) + 3600 } = {}) {
  let sealed = "-";
  if (token) {
    const key = createHash("sha256").update(JSON.stringify(["halcyon-dashboard-token", sessionSecret])).digest();
    const iv = randomBytes(12);
    const c = createCipheriv("aes-256-gcm", key, iv);
    const enc = Buffer.concat([c.update(token, "utf8"), c.final()]);
    sealed = Buffer.concat([iv, c.getAuthTag(), enc]).toString("base64url");
  }
  const profile = Buffer.from(JSON.stringify({ id: "1234", name: "Tester", avatar: null, roles: ["Developer"], admin }))
    .toString("base64url");
  const payload = `v3.${expires}.${randomBytes(16).toString("hex")}.${sealed}.${profile}`;
  return `halcyon_dash=${payload}.${sign(payload)}`;
}

function legacyKeyCookie(expires) {
  const payload = `v1.${expires}.${randomBytes(16).toString("hex")}`;
  return `halcyon_dash=${payload}.${sign(payload)}`;
}

test("production dashboard authentication", async t => {
  let upstreamRequests = 0;
  const upstream = http.createServer(async (req, res) => {
    upstreamRequests++;
    const chunks = [];
    for await (const chunk of req) chunks.push(chunk);
    if (req.url === "/api/scoped-tokens/self") {
      // Solara: the backend recognises exactly one scoped token.
      const ok = req.headers["x-dashboard-key"] === scopedToken;
      res.writeHead(ok ? 200 : 401, { "content-type": "application/json" });
      res.end(JSON.stringify(ok ? { master: false, permissions: ["stations.read"], stations: ["st1"] } : { error: "unauthorized" }));
      return;
    }
    res.writeHead(200, { "content-type": "application/json" });
    res.end(JSON.stringify({
      path: req.url,
      method: req.method,
      body: Buffer.concat(chunks).toString(),
      correctKey: req.headers["x-dashboard-key"] === adminKey,
      scopedKey: req.headers["x-dashboard-key"] === scopedToken,
      forwardedCookie: Boolean(req.headers.cookie),
    }));
  });
  upstream.listen(0, "127.0.0.1");
  await once(upstream, "listening");
  t.after(() => new Promise(resolve => {
    upstream.closeAllConnections();
    upstream.close(resolve);
  }));
  const upstreamPort = upstream.address().port;
  const dashboard = await startDashboard(upstreamPort);
  t.after(dashboard.stop);
  const { request } = dashboard;
  let cookie;

  await t.test("missing or forged cookies never reach either upstream", async () => {
    for (const headers of [{}, { cookie: "halcyon_dash=anything" }]) {
      const page = await request("/", { headers });
      assert.equal(page.status, 307);
      assert.equal(new URL(page.headers.get("location"), page.url).pathname, "/login");
      for (const url of ["/api/stats", "/game/stations/create"]) {
        assert.equal((await request(url, { method: "POST", headers, body: "{}" })).status, 401);
      }
    }
    assert.equal(upstreamRequests, 0);
  });

  await t.test("the old /dash-login key login is gone (admin keys sign in at /auth/key)", async () => {
    const response = await request("/dash-login", {
      method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ key: adminKey }),
    });
    assert.notEqual(response.status, 200);
    assert.equal(response.headers.get("set-cookie"), null);
    const now = Math.floor(Date.now() / 1000);
    assert.equal((await request("/api/stats", { headers: { cookie: legacyKeyCookie(now + 3600) } })).status, 401);
  });

  await t.test("an admin Discord session unlocks pages and authenticated API/game forwarding", async () => {
    cookie = discordCookie();
    assert.equal((await request("/", { headers: { cookie } })).status, 200);
    const api = await request("/api/stats?limit=1", { headers: { cookie } });
    assert.equal(api.status, 200);
    assert.deepEqual(await api.json(), {
      path: "/api/stats?limit=1", method: "GET", body: "", correctKey: true, scopedKey: false, forwardedCookie: false,
    });
    const game = await request("/game/stations/create?test=1", {
      method: "POST", headers: { cookie, "content-type": "application/json" }, body: '{"station_name":"test"}',
    });
    assert.equal(game.status, 200);
    assert.deepEqual(await game.json(), {
      // Solara: the Go station API (:78) needs the admin key too (x-dashboard-key), so /game sends it.
      path: "/stations/create?test=1", method: "POST", body: '{"station_name":"test"}',
      correctKey: true, scopedKey: false, forwardedCookie: false,
    });
    const me = await request("/me", { headers: { cookie } });
    assert.equal(me.status, 200);
    const body = await me.json();
    assert.equal(body.name, "Tester");
    assert.equal(body.admin, true);
    assert.equal((await request("/me")).status, 401);
  });

  await t.test("a non-admin Discord session forwards its scoped token, not the admin key", async () => {
    const scopedCookie = discordCookie({ admin: false, token: scopedToken });
    assert.equal((await request("/", { headers: { cookie: scopedCookie } })).status, 200);
    const api = await request("/api/stats", { headers: { cookie: scopedCookie } });
    assert.deepEqual(await api.json(), {
      path: "/api/stats", method: "GET", body: "", correctKey: false, scopedKey: true, forwardedCookie: false,
    });
    const me = await (await request("/me", { headers: { cookie: scopedCookie } })).json();
    assert.equal(me.admin, false);
    assert.deepEqual(me.permissions, ["stations.read"]);
    // a non-admin profile without a token must never act as the master key
    assert.equal((await request("/api/stats", { headers: { cookie: discordCookie({ admin: false }) } })).status, 401);
  });

  await t.test("an API key (scoped token in a header) calls the dashboard API as itself", async () => {
    // Authorization: Bearer and x-api-key both work, and forward THAT token (the backend enforces its scope).
    for (const headers of [{ authorization: `Bearer ${scopedToken}` }, { "x-api-key": scopedToken }]) {
      const api = await request("/api/users", { headers });
      assert.equal(api.status, 200);
      assert.deepEqual(await api.json(), {
        path: "/api/users", method: "GET", body: "", correctKey: false, scopedKey: true, forwardedCookie: false,
      });
    }
    const before = upstreamRequests;
    // the master key is never accepted from a header, nor anything that is not a scoped token
    for (const value of [`Bearer ${adminKey}`, "Bearer sdt1_short", "Basic abc", `Bearer ${scopedToken}x`]) {
      assert.equal((await request("/api/users", { headers: { authorization: value } })).status, 401);
    }
    assert.equal((await request("/api/users", { headers: { "x-api-key": adminKey } })).status, 401);
    // an API key does not open the station API (:78) or the dashboard pages
    assert.equal((await request("/game/stations/create", {
      method: "POST", headers: { authorization: `Bearer ${scopedToken}` }, body: "{}" })).status, 401);
    const page = await request("/", { headers: { authorization: `Bearer ${scopedToken}` } });
    assert.equal(page.status, 307);
    assert.equal(upstreamRequests, before);
  });

  await t.test("expired, future-dated, tampered and legacy sessions are rejected", async () => {
    const before = upstreamRequests;
    const now = Math.floor(Date.now() / 1000);
    const legacy = createHmac("sha256", sessionSecret).update("halcyon-dashboard-v1").digest("hex");
    const tampered = cookie.slice(0, -1) + (cookie.endsWith("a") ? "b" : "a");
    for (const invalid of [
      discordCookie({ expires: now - 1 }), discordCookie({ expires: now + 86400 }), tampered, `halcyon_dash=${legacy}`,
      legacyKeyCookie(now + 3600),
    ]) {
      for (const url of ["/api/stats", "/game/stations"]) {
        assert.equal((await request(url, { headers: { cookie: invalid } })).status, 401);
      }
      assert.equal((await request("/", { headers: { cookie: invalid } })).status, 307);
    }
    assert.equal(upstreamRequests, before);
  });

  await t.test("security headers are set on every response", async () => {
    const response = await request("/login");
    assert.equal(response.headers.get("x-frame-options"), "DENY");
    assert.equal(response.headers.get("x-content-type-options"), "nosniff");
    assert.match(response.headers.get("content-security-policy") ?? "", /frame-ancestors 'none'/);
    assert.equal(response.headers.get("x-powered-by"), null);
  });

  await t.test("cross-origin state-changing requests are refused", async () => {
    const before = upstreamRequests;
    for (const headers of [{ origin: "https://evil.example" }, { "sec-fetch-site": "cross-site" }]) {
      const game = await request("/game/stations/create", {
        method: "POST", headers: { cookie, "content-type": "application/json", ...headers }, body: "{}",
      });
      assert.equal(game.status, 403);
      const api = await request("/api/users/x/ban", { method: "POST", headers: { cookie, ...headers } });
      assert.equal(api.status, 403);
    }
    assert.equal(upstreamRequests, before);
    // GETs are not state-changing and still work cross-site (the cookie is SameSite=Lax anyway).
    assert.equal((await request("/api/stats", { headers: { cookie, origin: "https://evil.example" } })).status, 200);
  });

  // Last against this server: it ends by tripping the shared sign-in lockout.
  await t.test("admin key sign-in: body only, constant-time, rate limited, httpOnly session", async () => {
    const form = key => ({ method: "POST", headers: { "content-type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({ key }).toString() });
    const sessionOf = res => (res.headers.get("set-cookie") ?? "").match(/halcyon_dash=([^;]+)/)?.[1] ?? null;
    // wrong key: back to /login with an error, no session
    const bad = await request("/auth/key", form("not-the-key"));
    assert.equal(bad.status, 303);
    assert.match(bad.headers.get("location") ?? "", /\/login\?err=bad-key/);
    assert.equal(sessionOf(bad), null);
    // the key in the URL is never read; GET is not a sign-in
    const inUrl = await request(`/auth/key?key=${adminKey}`, form(""));
    assert.match(inUrl.headers.get("location") ?? "", /err=bad-key/);
    assert.equal(sessionOf(inUrl), null);
    const get = await request(`/auth/key?key=${adminKey}`);
    assert.notEqual(get.status, 303);
    assert.equal(sessionOf(get), null);
    // a cross-site post is refused before the key is looked at
    assert.equal((await request("/auth/key", { ...form(adminKey),
      headers: { ...form(adminKey).headers, origin: "https://evil.example" } })).status, 403);
    // the right key: a normal signed admin session (httpOnly, SameSite=Lax), never containing the key
    const ok = await request("/auth/key", form(adminKey));
    assert.equal(ok.status, 303);
    assert.equal(new URL(ok.headers.get("location"), ok.url).pathname, "/");
    const setCookie = ok.headers.get("set-cookie") ?? "";
    assert.match(setCookie, /HttpOnly/i);
    assert.match(setCookie, /SameSite=lax/i);
    assert.equal(setCookie.includes(adminKey), false);
    const keyCookie = `halcyon_dash=${sessionOf(ok)}`;
    const api = await request("/api/stats", { headers: { cookie: keyCookie } });
    assert.equal((await api.json()).correctKey, true);
    assert.equal((await (await request("/me", { headers: { cookie: keyCookie } })).json()).admin, true);
    // ten failures lock the client out -- even the right key is refused then
    for (let i = 0; i < 10; i++) await request("/auth/key", form("wrong-" + i));
    const locked = await request("/auth/key", form(adminKey));
    assert.match(locked.headers.get("location") ?? "", /err=too-many-attempts/);
    assert.equal(sessionOf(locked), null);
  });

  await dashboard.stop();
  for (const missing of ["DASHBOARD_ADMIN_KEY", "DASHBOARD_SESSION_SECRET"]) {
    await t.test(`missing ${missing} fails closed`, async () => {
      const unconfigured = await startDashboard(upstreamPort, { [missing]: "" });
      try {
        const response = await unconfigured.request("/auth/discord/callback?code=x&state=y");
        assert.match(response.headers.get("location") ?? "", /err=discord-not-configured/);
        assert.equal(response.headers.get("set-cookie")?.includes("halcyon_dash=") ?? false, false);
        const keyLogin = await unconfigured.request("/auth/key", { method: "POST",
          headers: { "content-type": "application/x-www-form-urlencoded" }, body: `key=${adminKey}` });
        assert.match(keyLogin.headers.get("location") ?? "", /err=key-not-configured/);
        assert.equal(keyLogin.headers.get("set-cookie")?.includes("halcyon_dash=") ?? false, false);
        for (const url of ["/api/stats", "/game/stations"]) {
          assert.equal((await unconfigured.request(url, { headers: { cookie } })).status, 401);
        }
      } finally {
        await unconfigured.stop();
      }
    });
  }
});
