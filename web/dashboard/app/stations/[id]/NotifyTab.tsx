"use client";

// Notify: put a notification board with your own text in front of chosen players (or everyone) -- fleet-wide: every
// live game server, whatever station this page is on.
// The game server reports who is connected (every ~10 s) and polls for queued notifications; it then calls the
// game's own UA2NotificationComponent::Client_ShowNotification RPC, so the board appears within ~2 s.

import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type NotifyInfo } from "../../../lib/api";

const MAX = 400;

export default function NotifyTab({ stationId }: { stationId: string }) {
  const [info, setInfo] = useState<NotifyInfo | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [everyone, setEveryone] = useState(false);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    api.notify(stationId).then(i => { setInfo(i); setErr(null); }).catch(e => setErr(e instanceof Error ? e.message : String(e)));
  }, [stationId]);

  useEffect(() => {
    let live = true;
    const tick = () => api.notify(stationId).then(i => { if (live) { setInfo(i); setErr(null); } })
      .catch(e => { if (live) setErr(e instanceof Error ? e.message : String(e)); });
    tick();
    const t = setInterval(tick, 10000);             // the server re-reports its players every ~10 s
    return () => { live = false; clearInterval(t); };
  }, [stationId]);

  const players = useMemo(() => {
    const q = query.trim().toLowerCase();
    return [...(info?.players ?? [])]
      .sort((a, b) => (a.name || a.id).localeCompare(b.name || b.id))
      .filter(p => !q || p.name.toLowerCase().includes(q) || p.id.toLowerCase().includes(q) || (p.server ?? "").toLowerCase().includes(q));
  }, [info, query]);

  const online = useMemo(() => new Set((info?.players ?? []).map(p => p.id)), [info]);
  const chosen = [...picked].filter(id => online.has(id));   // drop players who left since they were ticked

  function toggle(id: string) {
    setPicked(prev => { const n = new Set(prev); if (n.has(id)) n.delete(id); else n.add(id); return n; });
  }

  async function send() {
    const text = message.trim();
    if (!text) { setErr("Write a message first"); return; }
    if (!everyone && chosen.length === 0) { setErr("Pick at least one player, or send to everyone"); return; }
    setBusy(true); setNote(null);
    try {
      const r = await api.notifySend(stationId, everyone ? "all" : chosen, text);
      if (!r.success) setErr(r.error ?? "Send failed");
      else {
        setErr(null);
        setNote(everyone ? `Sent to everyone on ${servers} server${servers === 1 ? "" : "s"}` : `Sent to ${chosen.length} player${chosen.length === 1 ? "" : "s"}`);
        setMessage("");
      }
      load();
    } catch (e: unknown) { setErr(e instanceof Error ? e.message : "Send failed"); }
    finally { setBusy(false); }
  }

  const left = MAX - message.length;
  const servers = info?.deployments?.length ?? (info?.deployment_id ? 1 : 0);
  return (
    <div>
      <p className="text-xs mb-3" style={{ color: "var(--muted)" }}>
        Shows a notification board with your text in front of the players you pick (or everyone). Fleet-wide: covers
        every live game server{servers ? ` (${servers} online: ${(info?.deployments ?? []).map(d => d.name || d.id.slice(0, 8)).join(", ")})` : ""}.
        It appears within a couple of seconds.
      </p>

      {err && <div className="mb-3 px-3 py-2 rounded text-xs" style={{ background: "#2d1a1a", color: "var(--red)" }}>{err}</div>}
      {note && <div className="mb-3 px-3 py-2 rounded text-xs" style={{ background: "#1a2d1a", color: "var(--green)" }}>{note}</div>}

      {info && !info.deployment_id ? (
        <div className="px-4 py-6 rounded-md text-sm text-center" style={{ border: "1px solid var(--border)", color: "var(--muted)" }}>
          No live game server has reported its players yet (it does every few seconds once it&rsquo;s running).
          <div className="mt-3"><button onClick={load} className="text-xs px-3 py-1 rounded" style={{ border: "1px solid var(--border)", color: "var(--text)" }}>Refresh</button></div>
        </div>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          <div>
            <div className="flex gap-2 mb-2 items-center">
              <input className="flex-1 px-3 py-2 rounded-md text-sm outline-none"
                style={{ background: "var(--surface2)", border: "1px solid var(--border)", color: "var(--text)" }}
                placeholder={`Search ${info?.players.length ?? 0} players…`} value={query} onChange={e => setQuery(e.target.value)} />
              <label className="text-xs flex items-center gap-1 shrink-0" style={{ color: "var(--text)" }}>
                <input type="checkbox" checked={everyone} onChange={e => setEveryone(e.target.checked)} /> Everyone
              </label>
            </div>
            <div className="rounded-md overflow-y-auto" style={{ border: "1px solid var(--border)", maxHeight: 420, opacity: everyone ? 0.5 : 1 }}>
              {!info ? (
                <div className="px-3 py-4 text-xs" style={{ color: "var(--muted)" }}>Loading…</div>
              ) : players.length === 0 ? (
                <div className="px-3 py-4 text-xs" style={{ color: "var(--muted)" }}>{info.players.length ? "No player matches." : "Nobody is on any server right now."}</div>
              ) : players.map((p, i) => (
                <label key={p.id} className="px-3 py-1.5 flex items-center gap-2 text-xs cursor-pointer"
                  style={{ background: i % 2 ? "var(--surface2)" : "transparent" }}>
                  <input type="checkbox" disabled={everyone} checked={picked.has(p.id)} onChange={() => toggle(p.id)} />
                  <span className="truncate">{p.name || "(no name)"}</span>
                  {p.server && servers > 1 && <span className="shrink-0 px-1.5 rounded" style={{ border: "1px solid var(--border)", color: "var(--muted)" }}>{p.server}</span>}
                  <span className="font-mono ml-auto truncate" style={{ color: "var(--muted)", maxWidth: "55%" }}>{p.id}</span>
                </label>
              ))}
            </div>
            <p className="text-xs mt-2" style={{ color: "var(--muted)" }}>
              {everyone ? `All ${info?.players.length ?? 0} players` : `${chosen.length} selected`}
              {chosen.length > 0 && !everyone && <button onClick={() => setPicked(new Set())} className="ml-2 underline">clear</button>}
            </p>
          </div>

          <div>
            <textarea className="w-full px-3 py-2 rounded-md text-sm outline-none" rows={8} maxLength={MAX}
              style={{ background: "var(--surface2)", border: "1px solid var(--border)", color: "var(--text)", resize: "vertical" }}
              placeholder="Message shown on the board (new lines are kept)" value={message} onChange={e => setMessage(e.target.value)} />
            <div className="flex items-center justify-between mt-2">
              <span className="text-xs" style={{ color: left < 40 ? "var(--red)" : "var(--muted)" }}>{left} characters left</span>
              <button disabled={busy} onClick={send} className="text-xs px-4 py-2 rounded disabled:opacity-50"
                style={{ border: "1px solid var(--border)", color: "var(--green)" }}>
                {busy ? "Sending…" : everyone ? "Send to everyone" : `Send to ${chosen.length || "…"}`}
              </button>
            </div>
            {info && info.queued > 0 && <p className="text-xs mt-2" style={{ color: "var(--muted)" }}>{info.queued} waiting to be delivered</p>}
          </div>
        </div>
      )}
    </div>
  );
}
