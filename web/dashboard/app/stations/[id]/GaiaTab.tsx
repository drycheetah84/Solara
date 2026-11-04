"use client";

// GAIA: make the station announcer play any Wwise event for everyone, right now -- fleet-wide: every live game server.
// The game server reports which events it has loaded; Play queues one, and the server broadcasts it through the
// GAIA speakers within ~2 s. Players who turned "Play GAIA" off in their settings won't hear it.

import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type GaiaInfo } from "../../../lib/api";

type Filter = "announcer" | "music" | "all";
const ANNOUNCER = /gaia|announc|_vo_|^vo_|^play_vo|voice|welcome/i;
const MUSIC = /music|mus_|_mus|track|song|theme|stinger/i;

export default function GaiaTab({ stationId }: { stationId: string }) {
  const [info, setInfo] = useState<GaiaInfo | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("announcer");
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(() => {
    api.gaia(stationId).then(i => { setInfo(i); setErr(null); }).catch(e => setErr(e instanceof Error ? e.message : String(e)));
  }, [stationId]);

  useEffect(() => {
    let live = true;
    api.gaia(stationId).then(i => { if (live) { setInfo(i); setErr(null); } })
      .catch(e => { if (live) setErr(e instanceof Error ? e.message : String(e)); });
    return () => { live = false; };
  }, [stationId]);

  const shown = useMemo(() => {
    const ev = info?.events ?? [];
    const q = query.trim().toLowerCase();
    return ev.filter(n =>
      (filter === "all" || (filter === "announcer" ? ANNOUNCER.test(n) : MUSIC.test(n))) &&
      (!q || n.toLowerCase().includes(q))).slice(0, 300);
  }, [info, query, filter]);

  const servers = info?.deployments?.length ?? (info?.deployment_id ? 1 : 0);

  async function play(ev: string) {
    setBusy(ev); setNote(null);
    try {
      const r = await api.gaiaPlay(stationId, ev);
      if (!r.success) setErr(r.error ?? "Play failed");
      else { setErr(null); setNote(`Playing ${ev} on ${servers} server${servers === 1 ? "" : "s"}`); }
      load();
    } catch (e: unknown) { setErr(e instanceof Error ? e.message : "Play failed"); }
    finally { setBusy(null); }
  }

  const count = info?.events.length ?? 0;
  return (
    <div>
      <p className="text-xs mb-3" style={{ color: "var(--muted)" }}>
        Plays a sound through the GAIA announcer speakers for every player on every live game server
        {servers ? ` (${servers} online: ${(info?.deployments ?? []).map(d => d.name || d.id.slice(0, 8)).join(", ")})` : ""}. Players who turned
        &ldquo;Play GAIA&rdquo; off in their settings won&rsquo;t hear it.
      </p>

      {err && <div className="mb-3 px-3 py-2 rounded text-xs" style={{ background: "#2d1a1a", color: "var(--red)" }}>{err}</div>}
      {note && <div className="mb-3 px-3 py-2 rounded text-xs" style={{ background: "#1a2d1a", color: "var(--green)" }}>{note}</div>}

      {info && !info.deployment_id ? (
        <div className="px-4 py-6 rounded-md text-sm text-center" style={{ border: "1px solid var(--border)", color: "var(--muted)" }}>
          The live game server hasn&rsquo;t reported its sounds yet (it does about a minute after it starts).
          <div className="mt-3"><button onClick={load} className="text-xs px-3 py-1 rounded" style={{ border: "1px solid var(--border)", color: "var(--text)" }}>Refresh</button></div>
        </div>
      ) : (
        <>
          <div className="flex flex-wrap gap-2 mb-3 items-center">
            <input className="flex-1 min-w-[180px] px-3 py-2 rounded-md text-sm outline-none"
              style={{ background: "var(--surface2)", border: "1px solid var(--border)", color: "var(--text)" }}
              placeholder={`Search ${count} sounds…`} value={query} onChange={e => setQuery(e.target.value)} />
            {(["announcer", "music", "all"] as Filter[]).map(f => (
              <button key={f} onClick={() => setFilter(f)} className="text-xs px-3 py-2 rounded-md capitalize"
                style={{ border: "1px solid var(--border)", background: filter === f ? "var(--surface2)" : "transparent", color: "var(--text)" }}>
                {f}
              </button>
            ))}
            {info && info.queued > 0 && <span className="text-xs" style={{ color: "var(--muted)" }}>{info.queued} queued</span>}
          </div>
          <div className="rounded-md overflow-y-auto" style={{ border: "1px solid var(--border)", maxHeight: 520 }}>
            {!info ? (
              <div className="px-3 py-4 text-xs" style={{ color: "var(--muted)" }}>Loading…</div>
            ) : shown.length === 0 ? (
              <div className="px-3 py-4 text-xs" style={{ color: "var(--muted)" }}>No sound matches — try &ldquo;All&rdquo;.</div>
            ) : shown.map((n, i) => (
              <div key={n} className="px-3 py-1.5 flex items-center justify-between gap-3 text-xs"
                style={{ background: i % 2 ? "var(--surface2)" : "transparent" }}>
                <span className="font-mono break-all">{n}</span>
                <button disabled={busy === n} onClick={() => play(n)} className="px-3 py-1 rounded shrink-0 disabled:opacity-50"
                  style={{ border: "1px solid var(--border)", color: "var(--green)" }}>
                  {busy === n ? "…" : "Play"}
                </button>
              </div>
            ))}
          </div>
          {shown.length === 300 && <p className="text-xs mt-2" style={{ color: "var(--muted)" }}>Showing the first 300 — search to narrow it down.</p>}
        </>
      )}
    </div>
  );
}
