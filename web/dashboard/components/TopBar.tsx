"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import Image from "next/image";
import { useEffect, useState } from "react";

const nav = [
  { href: "/",            label: "Stations"    },
  { href: "/users",       label: "Users"       },
  { href: "/deployments", label: "Deployments" },
  { href: "/events",      label: "Events"      },
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

  return (
    <header className="flex items-center gap-4 px-4 py-3"
      style={{ background: "var(--surface)", borderBottom: "1px solid var(--border)" }}>

      {/* Lead: logo + title */}
      <Link href="/" className="flex items-center gap-2 shrink-0 hover:opacity-80 active:opacity-50">
        <Image src="/logo.webp" alt="logo" width={28} height={28} className="rounded" />
        <strong className="text-sm uppercase tracking-wide hidden sm:block">
          Ares Dashboard
        </strong>
      </Link>

      {/* Nav */}
      <nav className="flex items-center gap-1 flex-1 overflow-x-auto">
        {[...nav, ...(me?.admin ? [{ href: "/titledata", label: "Title data" }, { href: "/anonymous-names", label: "Anonymous names" }] : [])].map(({ href, label }) => {
          const active = href === "/"
            ? (path === "/" || path.startsWith("/stations"))
            : path.startsWith(href);
          return (
            <Link key={href} href={href}
              className="px-3 py-1.5 rounded-md text-sm whitespace-nowrap transition-colors"
              style={{
                color:      active ? "var(--text)"    : "var(--muted)",
                background: active ? "var(--surface2)" : "transparent",
                fontWeight: active ? 500 : 400,
              }}>
              {label}
            </Link>
          );
        })}
      </nav>

      {/* Trail: the signed-in Discord user */}
      {me && (
        <div className="flex items-center gap-2 shrink-0">
          <Link href="/profile"
            className="flex items-center gap-2 px-2 py-1 rounded-md transition-colors hover:opacity-90"
            style={{ background: path.startsWith("/profile") ? "var(--surface2)" : "transparent" }}>
            {me.avatar && (
              // eslint-disable-next-line @next/next/no-img-element -- Discord CDN avatar, no image optimisation needed
              <img src={me.avatar} alt="" width={26} height={26} className="rounded-full" referrerPolicy="no-referrer" />
            )}
            <span className="text-sm hidden sm:inline" style={{ color: "var(--text)" }}>{me.name}</span>
          </Link>
          <button onClick={logout} className="px-2.5 py-1 rounded-md text-xs"
            style={{ border: "1px solid var(--border)", color: "var(--muted)" }}>
            Log out
          </button>
        </div>
      )}
    </header>
  );
}
