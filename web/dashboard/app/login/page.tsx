"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";

// Dashboard sign-in. The normal path is Discord (/auth/discord): members of the Halcyon Discord server get the access
// their roles map to. The admin key / dashboard token form stays as a break-glass fallback: the key is checked
// server-side and only an opaque httpOnly cookie is ever stored; /api/* proxying happens server-side.

const ERRORS: Record<string, string> = {
  "discord-not-configured": "Discord sign-in is not set up on this server yet.",
  "discord-cancelled": "Discord sign-in was cancelled.",
  "discord-state": "That sign-in link expired. Try again.",
  "discord-failed": "Discord did not answer. Try again in a moment.",
  "not-in-server": "Your Discord account is not in the Halcyon server.",
  "no-dashboard-role": "Your Discord roles do not include dashboard access.",
  "token-failed": "Signed in, but the backend did not issue your access. Try again.",
  "too-many-attempts": "Too many attempts. Wait a few minutes.",
};

function LoginInner() {
  const params = useSearchParams();
  const urlErr = params.get("err");
  const [key, setKey] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [showKey, setShowKey] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      const res = await fetch("/dash-login", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ key }),
      });
      if (res.ok) {
        window.location.href = "/";
        return;
      }
      setErr(res.status === 401 ? "That key was rejected." : `Login failed (${res.status}).`);
    } catch (e2) {
      setErr(String(e2));
    }
    setBusy(false);
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-950 p-6">
      <div className="w-full max-w-md rounded-xl border border-neutral-800 bg-neutral-900 p-7 shadow-xl">
        <h1 className="mb-1 text-lg font-semibold text-neutral-100">Halcyon dashboard</h1>
        <p className="mb-5 text-sm leading-relaxed text-neutral-400">
          This controls the live fleet. Sign in with the Discord account that has your staff role.
        </p>

        <a href="/auth/discord"
          className="block w-full rounded-lg py-2.5 text-center text-sm font-medium text-white transition hover:opacity-90"
          style={{ background: "#5865F2" }}>
          Log in with Discord
        </a>
        {urlErr && <div className="mt-3 text-xs text-red-400">{ERRORS[urlErr] ?? "Sign-in failed."}</div>}

        <button type="button" onClick={() => setShowKey(v => !v)}
          className="mt-6 text-xs text-neutral-500 underline-offset-2 hover:underline">
          {showKey ? "Hide" : "Use an admin key or dashboard token instead"}
        </button>
        {showKey && (
          <form onSubmit={submit} className="mt-3">
            <input
              type="password"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              placeholder="admin key or dashboard token"
              autoComplete="off"
              className="w-full rounded-lg border border-neutral-700 bg-neutral-950 px-3 py-2.5 font-mono text-sm text-neutral-100 outline-none focus:border-blue-500"
            />
            <div className="mt-2 min-h-[18px] text-xs text-red-400">{err}</div>
            <button
              type="submit"
              disabled={busy || !key.trim()}
              className="mt-1 w-full rounded-lg bg-neutral-700 py-2 text-sm font-medium text-white transition hover:bg-neutral-600 disabled:opacity-40"
            >
              {busy ? "Checking…" : "Unlock"}
            </button>
          </form>
        )}
      </div>
    </div>
  );
}

export default function Login() {
  return (
    <Suspense fallback={null}>
      <LoginInner />
    </Suspense>
  );
}
