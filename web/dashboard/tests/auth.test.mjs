import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHmac, randomBytes } from "node:crypto";
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

function login(request, body, headers = {}) {
  return request("/dash-login", {
    method: "POST",
    headers: { "content-type": "application/json", ...headers },
    body: JSON.stringify(body),
  });
}

function signedCookie(expires) {
  const payload = `v1.${expires}.${randomBytes(16).toString("hex")}`;
  const signature = createHmac("sha256", sessionSecret)
    .update(JSON.stringify(["halcyon-dashboard", adminKey, payload])).digest("hex");
  return `halcyon_dash=${payload}.${signature}`;
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

  await t.test("wrong keys and malformed login bodies are rejected cleanly", async () => {
    for (const body of [null, [], {}, { key: null }, { key: 123 }, { key: [] }, { key: {} }, { key: "wrong" }]) {
      const response = await login(request, body);
      assert.equal(response.status, 401);
      assert.equal(response.headers.get("set-cookie"), null);
    }
    assert.equal((await request("/dash-login", { method: "POST", body: "{" })).status, 400);
  });

  await t.test("valid login unlocks pages and authenticated API/game forwarding", async () => {
    const response = await login(request, { key: adminKey });
    assert.equal(response.status, 200);
    const setCookie = response.headers.get("set-cookie");
    assert.match(setCookie, /HttpOnly/i);
    assert.match(setCookie, /SameSite=lax/i);
    assert.match(setCookie, /Max-Age=43200/i);
    assert.ok(!setCookie.includes(adminKey));
    assert.ok(!setCookie.includes(sessionSecret));
    cookie = setCookie.split(";")[0];
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
  });

  await t.test("a scoped token logs in and the proxy forwards that token, not the admin key", async () => {
    const unknown = await login(request, { key: "sdt1_" + "0".repeat(64) });
    assert.equal(unknown.status, 401);
    assert.equal(unknown.headers.get("set-cookie"), null);
    const response = await login(request, { key: scopedToken });
    assert.equal(response.status, 200);
    const setCookie = response.headers.get("set-cookie");
    assert.ok(!setCookie.includes(scopedToken), "the token must not sit in the cookie in the clear");
    const scopedCookie = setCookie.split(";")[0];
    assert.equal((await request("/", { headers: { cookie: scopedCookie } })).status, 200);
    const api = await request("/api/stats", { headers: { cookie: scopedCookie } });
    assert.deepEqual(await api.json(), {
      path: "/api/stats", method: "GET", body: "", correctKey: false, scopedKey: true, forwardedCookie: false,
    });
  });

  await t.test("expired, future-dated, tampered and legacy sessions are rejected", async () => {
    const before = upstreamRequests;
    const now = Math.floor(Date.now() / 1000);
    const legacy = createHmac("sha256", sessionSecret).update("halcyon-dashboard-v1").digest("hex");
    const tampered = cookie.slice(0, -1) + (cookie.endsWith("a") ? "b" : "a");
    for (const invalid of [
      signedCookie(now - 1), signedCookie(now + 86400), tampered, `halcyon_dash=${legacy}`,
    ]) {
      for (const url of ["/api/stats", "/game/stations"]) {
        assert.equal((await request(url, { headers: { cookie: invalid } })).status, 401);
      }
      assert.equal((await request("/", { headers: { cookie: invalid } })).status, 307);
    }
    assert.equal(upstreamRequests, before);
  });

  await t.test("HTTPS forwarding sets a secure cookie", async () => {
    const response = await login(request, { key: adminKey }, { "x-forwarded-proto": "https" });
    assert.equal(response.status, 200);
    assert.match(response.headers.get("set-cookie"), /; Secure/i);
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

  await t.test("login is locked out after 10 failed attempts", async () => {
    const who = { "cf-connecting-ip": "203.0.113.9" };
    for (let i = 0; i < 10; i++) assert.equal((await login(request, { key: "wrong" }, who)).status, 401);
    const locked = await login(request, { key: adminKey }, who);
    assert.equal(locked.status, 429);
    assert.ok(Number(locked.headers.get("retry-after")) > 0);
    assert.equal(locked.headers.get("set-cookie"), null);
    // other clients are unaffected
    assert.equal((await login(request, { key: adminKey }, { "cf-connecting-ip": "203.0.113.10" })).status, 200);
  });

  await dashboard.stop();
  for (const missing of ["DASHBOARD_ADMIN_KEY", "DASHBOARD_SESSION_SECRET"]) {
    await t.test(`missing ${missing} fails closed`, async () => {
      const unconfigured = await startDashboard(upstreamPort, { [missing]: "" });
      try {
        const response = await login(unconfigured.request, { key: adminKey });
        assert.equal(response.status, 503);
        assert.equal(response.headers.get("set-cookie"), null);
        for (const url of ["/api/stats", "/game/stations"]) {
          assert.equal((await unconfigured.request(url, { headers: { cookie } })).status, 401);
        }
      } finally {
        await unconfigured.stop();
      }
    });
  }
});
