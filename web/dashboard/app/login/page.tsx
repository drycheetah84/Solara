"use client";

import { Suspense } from "react";
import Image from "next/image";
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
    <div className="mx-auto max-w-[1200px] px-6">
      <div className="max-w-sm mx-auto pt-24 pb-20">
        <div className="card p-8">
          <div className="flex flex-col items-center gap-3 mb-6 text-center">
            <Image src="/logo.webp" alt="Halcyon logo" width={56} height={56} style={{ borderRadius: 12 }} />
            <div>
              <p className="eyebrow mb-1.5">Halcyon</p>
              <h1 className="text-[28px] font-medium tracking-tight">Log in</h1>
              <p className="text-sm mt-1" style={{ color: "var(--muted)" }}>
                Sign in with your Discord account. What you can see and do comes from your roles in the Halcyon server.
              </p>
            </div>
          </div>

          {urlErr && (
            <div className="card mb-4 px-4 py-3 text-sm" style={{ color: "var(--red)" }}>
              {ERRORS[urlErr] ?? "Sign-in failed."}
            </div>
          )}

          <a href="/auth/discord" className="btn-light w-full py-3! text-sm justify-center"
            style={{ background: "#5865F2", color: "#fff" }}>
            Log in with Discord
          </a>
        </div>
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
