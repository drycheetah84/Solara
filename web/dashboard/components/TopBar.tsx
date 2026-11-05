"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import Image from "next/image";
import { useEffect, useState } from "react";
import { LogOut } from "lucide-react";
import { Avatar } from "./ui";

const nav = [
  { href: "/",            label: "Stations"    },
  { href: "/users",       label: "Users"       },
  { href: "/deployments", label: "Deployments" },
  { href: "/events",      label: "Events"      },
];

// Global-admin extras (master session). Everything is still enforced server-side; hiding is cosmetic.
const adminNav = [
  { href: "/titledata",       label: "Title data"      },
  { href: "/anonymous-names", label: "Anonymous names" },
];

export type Me = {
  id: string; name: string; avatar: string | null; roles: string[]; admin: boolean;
  permissions: string[]; stations: string[]; expires_at: string;
};

export function TopBar() {
  const path = usePathname();
  const [me, setMe] = useState<Me | null>(null);

  useEffect(() => {
    if (path === "/login") return;
    let alive = true;
    fetch("/me", { cache: "no-store" })
      .then(r => (r.ok ? r.json() : null))
      .then(m => { if (alive) setMe(m); })
      .catch(() => {});
    return () => { alive = false; };
  }, [path]);

  async function logout() {
    await fetch("/logout", { method: "POST" }).catch(() => null);
    window.location.assign("/login");
  }

  if (path === "/login") return null;

  const items = [...nav, ...(me?.admin ? adminNav : [])];

  return (
    <header style={{ background: "#20242c", borderBottom: "1px solid var(--line)" }}>
      <div className="flex items-center gap-3 px-6 py-3.5">
        {/* Lead: logo + wordmark */}
        <Link href="/" className="flex items-center gap-2.5 shrink-0 hover:opacity-85 active:opacity-60">
          <Image src="/logo.webp" alt="Halcyon logo" width={30} height={30} style={{ borderRadius: 6 }} />
          <strong className="text-[15px] font-bold tracking-wide hidden min-[480px]:block">
            HALCYON STATION DASHBOARD
          </strong>
        </Link>

        {/* Nav */}
        <nav className="flex items-center gap-1 flex-1 overflow-x-auto ml-2">
          {items.map(({ href, label }) => {
            const active = href === "/"
              ? (path === "/" || path.startsWith("/stations"))
              : path.startsWith(href);
            return (
              <Link key={href} href={href}
                className="px-3 py-1.5 rounded-md text-sm whitespace-nowrap transition-colors"
                style={{
                  color: active ? "var(--text)" : "var(--muted)",
                  background: active ? "rgba(255,255,255,0.07)" : "transparent",
                  fontWeight: active ? 500 : 400,
                }}>
                {label}
              </Link>
            );
          })}
        </nav>

        {/* Trail: the signed-in Discord user */}
        <div className="flex items-center gap-3 shrink-0">
          {me ? (
            <>
              {me.admin && (
                <span className="px-1.5 py-0.5 rounded text-xs font-medium hidden md:inline"
                  style={{ background: "#1a3a2a", color: "var(--green)" }}>
                  ADMIN
                </span>
              )}
              <button onClick={logout} className="btn-outline">
                <LogOut size={13} /> <span className="hidden sm:inline">Logout</span>
              </button>
              <Link href="/profile" className="flex items-center gap-2.5 hover:opacity-85">
                <span className="text-[15px] hidden sm:block max-w-40 truncate">{me.name}</span>
                <span className="rounded-full overflow-hidden inline-flex">
                  <Avatar url={me.avatar} name={me.name} size={40} />
                </span>
              </Link>
            </>
          ) : (
            <Link href="/login" className="btn-outline">Login</Link>
          )}
        </div>
      </div>
    </header>
  );
}
