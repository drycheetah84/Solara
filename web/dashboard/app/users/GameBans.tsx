"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type GameBan } from "../../lib/api";
import { fmt, ago } from "../../lib/utils";

const BAN_DURATIONS: { label: string; minutes: number }[] = [
  { label: "1 hour", minutes: 60 },
  { label: "1 day", minutes: 1440 },
  { label: "7 days", minutes: 10080 },
  { label: "30 days", minutes: 43200 },
  { label: "Permanent", minutes: 0 },
];

// Game bans live in Mothership (not in the station ban list above). Every one is GLOBAL: the player cannot log in
// and their current session stops working (Solara mothership/globalban.go). Issued here, by the in-game moderation
// panel, or by a game server; lifted here.
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

  const [banId, setBanId] = useState("");
  const [banMinutes, setBanMinutes] = useState(1440);
  const [banReason, setBanReason] = useState("");

  async function ban() {
    const id = banId.trim();
    if (!id) { setErr("Paste a player id (Meta id or Mothership id)"); return; }
    const howLong = BAN_DURATIONS.find(d => d.minutes === banMinutes)?.label.toLowerCase() ?? `${banMinutes} minutes`;
    if (!confirm(`Ban ${id} from the game (${howLong})? They are disconnected from Mothership and cannot log in.`)) return;
    setBusy("add");
    try {
      const res = await api.addGameBan(id, banMinutes, banReason.trim());
      if (!res.success) setErr(res.error ?? "Ban failed");
      else { setBanId(""); setBanReason(""); setErr(null); }
      load();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setBusy(null);
    }
  }

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

      <div className="mb-3 flex flex-wrap gap-2 items-center">
        <input value={banId} onChange={e => setBanId(e.target.value)} placeholder="Player id (Meta id or Mothership id)"
          className="field text-sm min-w-0 flex-1" style={{ minWidth: 220 }} />
        <select value={banMinutes} onChange={e => setBanMinutes(Number(e.target.value))} className="field text-sm">
          {BAN_DURATIONS.map(d => <option key={d.minutes} value={d.minutes}>{d.label}</option>)}
        </select>
        <input value={banReason} onChange={e => setBanReason(e.target.value)} maxLength={200}
          placeholder="Reason (shown to the player)" className="field text-sm min-w-0 flex-1" style={{ minWidth: 200 }} />
        <button onClick={ban} disabled={busy === "add"} className="btn-light text-sm disabled:opacity-50"
          style={{ color: "var(--red)" }}>
          {busy === "add" ? "Banning…" : "Ban"}
        </button>
      </div>

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
