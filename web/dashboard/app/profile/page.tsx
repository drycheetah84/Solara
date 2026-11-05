"use client";

import { useEffect, useMemo, useState } from "react";
import { api, type Station } from "../../lib/api";
import { fmt } from "../../lib/utils";
import type { Me } from "../../components/TopBar";

// What each dashboard permission allows (mirrors Solara's dashScopedPerms).
const PERM_TEXT: Record<string, string> = {
  "fleets.view": "See the fleet list: station names and player counts",
  "stats.read": "Server stats, telemetry, events, agents, netvar dumps",
  "users.read": "List users and view inventories",
  "users.ban": "Ban and unban users, lift game bans",
  "users.mute": "Mute and unmute players' voice on every game server",
  "users.inventory": "Grant and revoke inventory items",
  "catalog.read": "Browse the item catalogue",
  "stations.read": "View station config, whitelist, members, bans, roles, boards, deployments, events",
  "stations.config": "Edit station config, arena whitelists, browser whitelist and boards",
  "stations.manage": "Create deployments, spin up servers",
  "stations.delete": "Delete stations / fleets",
  "stations.events": "Create and delete events",
  "stations.roles": "Manage roles and who has them",
  "stations.gaia": "Play GAIA announcements",
  "stations.notify": "Send in-game notifications to players",
  "spec.read": "View saved editor levels",
  "spec.write": "Edit and delete saved editor levels",
  "drafters.read": "See who can use the level editor",
  "drafters.write": "Grant and revoke level-editor access",
  "sessions.read": "View EOS sessions",
  "sessions.write": "Add and remove EOS sessions",
};

const card: React.CSSProperties = { background: "var(--surface)", border: "1px solid var(--border)" };

export default function ProfilePage() {
  const [me, setMe] = useState<Me | null>(null);
  const [stations, setStations] = useState<Station[]>([]);
  const [err, setErr] = useState("");

  useEffect(() => {
    fetch("/me", { cache: "no-store" })
      .then(r => (r.ok ? r.json() : Promise.reject(new Error(`profile ${r.status}`))))
      .then(setMe)
      .catch(e => setErr((e as Error).message));
    api.stations().then(setStations).catch(() => setStations([]));
  }, []);

  const stationName = useMemo(() => {
    const m = new Map(stations.map(s => [s.station_id, s.station_name] as const));
    return (id: string) => (id === "*" ? "All fleets" : m.get(id) ? `${m.get(id)}` : id);
  }, [stations]);

  // "perm@station" entries -> permission -> fleets
  const grants = useMemo(() => {
    const out = new Map<string, Set<string>>();
    for (const e of me?.permissions ?? []) {
      const [perm, at] = e.split("@");
      const set = out.get(perm) ?? new Set<string>();
      if (at) set.add(at); else for (const s of me?.stations ?? []) set.add(s);
      out.set(perm, set);
    }
    return [...out.entries()].sort(([a], [b]) => a.localeCompare(b));
  }, [me]);

  if (err) return <div className="text-sm" style={{ color: "var(--red)" }}>{err}</div>;
  if (!me) return <div className="text-sm" style={{ color: "var(--muted)" }}>Loading…</div>;

  return (
    <div className="mx-auto max-w-[900px] pt-8 pb-14">
      <p className="eyebrow mb-1.5">Account</p>
      <h1 className="text-[44px] leading-tight font-medium tracking-tight mb-6">Profile</h1>
      <div className="card p-5 mb-5 flex items-center gap-4">
        {me.avatar && (
          // eslint-disable-next-line @next/next/no-img-element -- Discord CDN avatar
          <img src={me.avatar} alt="" width={72} height={72} className="rounded-full" referrerPolicy="no-referrer" />
        )}
        <div className="flex-1 min-w-0">
          <div className="text-[24px] font-medium grad-name w-fit">{me.name}</div>
          <div className="text-xs font-mono" style={{ color: "var(--muted)" }}>Discord ID {me.id}</div>
          <div className="flex flex-wrap gap-1.5 mt-2">
            {me.roles.map(r => (
              <span key={r} className="text-xs px-2 py-0.5 rounded-full" style={{ background: "var(--surface2)", color: "var(--text)" }}>{r}</span>
            ))}
          </div>
        </div>
        <div className="text-xs text-right" style={{ color: "var(--muted)" }}>
          Session ends<br />{fmt(me.expires_at)}
        </div>
      </div>

      <h2 className="text-sm font-semibold mb-2" style={{ color: "var(--text)" }}>Your access</h2>
      {me.admin ? (
        <div className="rounded-lg p-4 text-sm" style={card}>
          <span style={{ color: "var(--green)" }}>Full admin.</span>{" "}
          <span style={{ color: "var(--muted)" }}>You can do everything on every fleet, including tokens and spinning up servers.</span>
        </div>
      ) : grants.length === 0 ? (
        <div className="rounded-lg p-4 text-sm" style={{ ...card, color: "var(--muted)" }}>No permissions.</div>
      ) : (
        <div className="rounded-lg overflow-hidden" style={card}>
          <table className="w-full text-sm">
            <thead>
              <tr style={{ background: "var(--surface2)" }}>
                <th className="px-4 py-2 text-left text-xs font-medium" style={{ color: "var(--muted)" }}>Permission</th>
                <th className="px-4 py-2 text-left text-xs font-medium" style={{ color: "var(--muted)" }}>Fleets</th>
              </tr>
            </thead>
            <tbody>
              {grants.map(([perm, where]) => (
                <tr key={perm} style={{ borderTop: "1px solid var(--border)" }}>
                  <td className="px-4 py-2.5">
                    <div style={{ color: "var(--text)" }}>{PERM_TEXT[perm] ?? perm}</div>
                    <div className="text-xs font-mono" style={{ color: "var(--muted)" }}>{perm}</div>
                  </td>
                  <td className="px-4 py-2.5 text-xs" style={{ color: "var(--text)" }}>
                    {[...where].map(stationName).join(", ")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="text-xs mt-3" style={{ color: "var(--muted)" }}>
        Access comes from your roles in the Halcyon Discord server. If your roles change, log out and back in.
      </p>
    </div>
  );
}
