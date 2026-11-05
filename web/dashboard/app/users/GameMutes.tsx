"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type GameMute, type MutesResponse } from "../../lib/api";
import { fmt, ago } from "../../lib/utils";

const DURATIONS: { label: string; minutes: number }[] = [
  { label: "10 minutes", minutes: 10 },
  { label: "1 hour", minutes: 60 },
  { label: "1 day", minutes: 1440 },
  { label: "7 days", minutes: 10080 },
  { label: "Permanent", minutes: 0 },
];

// Voice mutes. Fleet-wide: the player is silenced for everyone on whichever game server they are on (now or later),
// sees the game's "MUTED FOR N MINUTES" board, and every server picks the change up within a few seconds.
export default function GameMutes() {
  const [data, setData] = useState<MutesResponse | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [hidden, setHidden] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [player, setPlayer] = useState("");
  const [minutes, setMinutes] = useState(60);
  const [reason, setReason] = useState("");

  const load = useCallback(() => {
    api.mutes().then(d => { setData(d); setErr(null); }).catch((e: Error) => {
      if (/403|forbidden/i.test(e.message)) setHidden(true);   // this login may not mute: hide the section
      else setErr(e.message);
    });
  }, []);

  useEffect(() => {
    load();
    const t = setInterval(load, 10000);
    return () => clearInterval(t);
  }, [load]);

  if (hidden) return null;

  const online = data?.players ?? [];
  const nameOf = (id: string) => online.find(p => p.id === id)?.name;

  async function mute() {
    const id = player.trim();
    if (!id) { setErr("Pick a player or paste a player id"); return; }
    setBusy("add");
    try {
      const res = await api.mute(id, minutes, reason.trim());
      if (!res.success) setErr(res.error ?? "Mute failed");
      else { setPlayer(""); setReason(""); setErr(null); }
      load();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setBusy(null);
    }
  }

  async function lift(m: GameMute) {
    const who = m.username || nameOf(m.player_id) || m.player_id;
    if (!confirm(`Unmute ${who}?`)) return;
    setBusy(m.player_id);
    try {
      const res = await api.unmute(m.player_id);
      if (!res.success) setErr(res.error ?? "Unmute failed");
      load();
    } catch (e: unknown) {
      setErr(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setBusy(null);
    }
  }

  const mutes = data?.mutes;
  return (
    <div className="mb-8">
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold" style={{ color: "var(--text)" }}>Voice mutes</h2>
        {mutes && (
          <span className="text-xs px-2 py-1 rounded-full" style={{ background: "var(--surface2)", color: "var(--muted)" }}>
            {mutes.length} active
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
        <input list="mute-online" value={player} onChange={e => setPlayer(e.target.value)}
          placeholder={online.length ? "Player id (pick someone online)" : "Player id"}
          className="field text-sm min-w-0 flex-1" style={{ minWidth: 220 }} />
        <datalist id="mute-online">
          {online.map(p => <option key={p.id} value={p.id}>{`${p.name || "(no name)"} on ${p.server || "?"}`}</option>)}
        </datalist>
        <select value={minutes} onChange={e => setMinutes(Number(e.target.value))} className="field text-sm">
          {DURATIONS.map(d => <option key={d.minutes} value={d.minutes}>{d.label}</option>)}
        </select>
        <input value={reason} onChange={e => setReason(e.target.value)} maxLength={120}
          placeholder="Reason (shown to the player)" className="field text-sm min-w-0 flex-1" style={{ minWidth: 200 }} />
        <button onClick={mute} disabled={busy === "add"} className="btn-light text-sm disabled:opacity-50">
          {busy === "add" ? "Muting…" : "Mute"}
        </button>
      </div>

      <div className="rounded-lg overflow-x-auto" style={{ border: "1px solid var(--border)" }}>
        <table className="w-full text-sm">
          <thead>
            <tr style={{ background: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {["Player", "Reason", "Muted", "Ends", ""].map(h => (
                <th key={h} className="px-4 py-3 text-left font-medium text-xs" style={{ color: "var(--muted)" }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {!mutes ? (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-sm" style={{ color: "var(--muted)" }}>Loading…</td></tr>
            ) : mutes.length === 0 ? (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-sm" style={{ color: "var(--muted)" }}>No active mutes</td></tr>
            ) : mutes.map((m, i) => (
              <tr key={m.mute_id || `mute-${i}`}
                style={{ background: i % 2 === 0 ? "var(--surface)" : "var(--surface2)", borderBottom: "1px solid var(--border)" }}>
                <td className="px-4 py-3">
                  <div className="font-medium">{m.username || nameOf(m.player_id) || "(unknown)"}</div>
                  <div className="font-mono text-xs" style={{ color: "var(--muted)" }}>
                    {m.meta_id || `${m.player_id.slice(0, 8)}…`}
                    {nameOf(m.player_id) && <span className="ml-2" style={{ color: "var(--green)" }}>online</span>}
                  </div>
                </td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{m.reason || "—"}</td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{ago(m.created_at)}</td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{m.expires_at ? fmt(m.expires_at) : "Permanent"}</td>
                <td className="px-4 py-3">
                  <button onClick={() => lift(m)} disabled={busy === m.player_id}
                    className="text-xs px-2 py-1 rounded disabled:opacity-50"
                    style={{ color: "var(--green)", border: "1px solid var(--border)" }}>
                    {busy === m.player_id ? "Unmuting…" : "Unmute"}
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
