"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type GameBan } from "../../lib/api";
import { fmt, ago } from "../../lib/utils";

// Bans issued in game (the in-game moderation panel) or by a game server. They live in Mothership, not in the
// station ban list above, so they get their own table and their own unban.
export default function GameBans() {
  const [bans, setBans] = useState<GameBan[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(() => {
    api.gameBans().then(b => { setBans(b); setErr(null); }).catch(e => setErr(e.message));
  }, []);

  useEffect(() => {
    load();
    const t = setInterval(load, 15000);
    return () => clearInterval(t);
  }, [load]);

  async function lift(b: GameBan) {
    const who = b.username || b.player_id;
    if (!confirm(`Unban ${who}? They can rejoin right away.`)) return;
    setBusy(b.player_id);
    try {
      const res = await api.liftGameBans(b.player_id);
      if (!res.success) setErr(res.error ?? "Unban failed");
      load();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setBusy(null);
    }
  }

  return (
    <div className="mb-8">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold" style={{ color: "var(--text)" }}>In-game bans</h2>
        {bans && (
          <span className="text-xs px-2 py-1 rounded-full"
            style={{ background: "var(--surface2)", color: "var(--muted)" }}>
            {bans.length} active
          </span>
        )}
      </div>

      {err && (
        <div className="mb-3 px-4 py-3 rounded-md text-sm"
          style={{ background: "#2d1a1a", color: "var(--red)", border: "1px solid #5c2020" }}>
          {err}
        </div>
      )}

      <div className="rounded-lg overflow-x-auto" style={{ border: "1px solid var(--border)" }}>
        <table className="w-full text-sm">
          <thead>
            <tr style={{ background: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {["Player", "Reason", "Banned", "Expires", ""].map(h => (
                <th key={h} className="px-4 py-3 text-left font-medium text-xs"
                  style={{ color: "var(--muted)" }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {!bans ? (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-sm" style={{ color: "var(--muted)" }}>Loading…</td></tr>
            ) : bans.length === 0 ? (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-sm" style={{ color: "var(--muted)" }}>No active in-game bans</td></tr>
            ) : bans.map((b, i) => (
              <tr key={b.ban_id || `ban-${i}`}
                style={{ background: i % 2 === 0 ? "var(--surface)" : "var(--surface2)", borderBottom: "1px solid var(--border)" }}>
                <td className="px-4 py-3">
                  <div className="font-medium">{b.username || "(unknown)"}</div>
                  <div className="font-mono text-xs" style={{ color: "var(--muted)" }}>
                    {b.meta_id || (b.player_id ? `${b.player_id.slice(0, 8)}…` : "—")}
                  </div>
                </td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>
                  {b.reason || "—"}
                  {b.source && <div className="mt-0.5 opacity-70">{b.source}</div>}
                </td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{ago(b.created_at)}</td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{fmt(b.expires_at)}</td>
                <td className="px-4 py-3">
                  <button onClick={() => lift(b)} disabled={busy === b.player_id}
                    className="text-xs px-2 py-1 rounded disabled:opacity-50"
                    style={{ color: "var(--green)", border: "1px solid var(--border)" }}>
                    {busy === b.player_id ? "Unbanning…" : "Unban"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
