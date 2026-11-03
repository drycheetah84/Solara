"use client";

import { Suspense } from "react";
import { useSearchParams } from "next/navigation";

// Dashboard sign-in: Discord only (/auth/discord). Members of the Halcyon Discord server get the access their roles
// map to; the session is an httpOnly cookie and /api/* proxying happens server-side.

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
  const urlErr = useSearchParams().get("err");
  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-950 p-6">
      <div className="w-full max-w-md rounded-xl border border-neutral-800 bg-neutral-900 p-7 shadow-xl">
        <h1 className="mb-1 text-lg font-semibold text-neutral-100">Halcyon dashboard</h1>
        <p className="mb-5 text-sm leading-relaxed text-neutral-400">
          Sign in with your Discord account. What you can see and do comes from your roles in the Halcyon server.
        </p>
        <a href="/auth/discord"
          className="block w-full rounded-lg py-2.5 text-center text-sm font-medium text-white transition hover:opacity-90"
          style={{ background: "#5865F2" }}>
          Log in with Discord
        </a>
        {urlErr && <div className="mt-3 text-xs text-red-400">{ERRORS[urlErr] ?? "Sign-in failed."}</div>}
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
