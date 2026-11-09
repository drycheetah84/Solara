"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { api, type AuditEntry } from "../lib/api";
import { ago } from "../lib/utils";

// The dashboard audit log: every mutating action a signed-in admin performed, newest first.
// Master key only (the backend refuses a scoped token), so the page is reachable from the admin nav.
export default function AuditLog() {
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [total, setTotal] = useState(0);
  const [err, setErr] = useState<string | null>(null);
  const [actor, setActor] = useState("");
  const [q, setQ] = useState("");
  const seq = useRef(0);

  const load = useCallback(() => {
    const mine = ++seq.current;
    api.auditLog({ limit: 500, actor: actor.trim(), q: q.trim() })
      .then(r => { if (mine === seq.current) { setEntries(r.entries); setTotal(r.total); setErr(null); } })
      .catch(e => { if (mine === seq.current) setErr(e instanceof Error ? e.message : "Failed to load"); });
  }, [actor, q]);

  // debounce filter changes; also poll every 20s
  useEffect(() => {
    const t = setTimeout(load, 250);
    return () => clearTimeout(t);
  }, [load]);
  useEffect(() => {
    const t = setInterval(load, 20000);
    return () => clearInterval(t);
  }, [load]);

  return (
    <div className="max-w-6xl mx-auto">
      <div className="flex items-center justify-between mb-4 flex-wrap gap-2">
        <div>
          <h1 className="text-lg font-semibold" style={{ color: "var(--text)" }}>Audit log</h1>
          <p className="text-sm" style={{ color: "var(--muted)" }}>
            Every change made through the dashboard, newest first.
          </p>
        </div>
        {entries && (
          <span className="text-xs px-2 py-1 rounded-full"
            style={{ background: "var(--surface2)", color: "var(--muted)" }}>
            {total.toLocaleString()} recorded
          </span>
        )}
      </div>

      <div className="mb-3 flex flex-wrap gap-2">
        <input value={actor} onChange={e => setActor(e.target.value)}
          placeholder="Filter by admin (name or id)" className="field text-sm" style={{ minWidth: 220 }} />
        <input value={q} onChange={e => setQ(e.target.value)}
          placeholder="Search action, path or detail" className="field text-sm flex-1" style={{ minWidth: 220 }} />
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
              {["When", "Admin", "Action", "Status", "Detail"].map(h => (
                <th key={h} className="px-4 py-3 text-left font-medium text-xs"
                  style={{ color: "var(--muted)" }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {!entries ? (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-sm" style={{ color: "var(--muted)" }}>Loading…</td></tr>
            ) : entries.length === 0 ? (
              <tr><td colSpan={5} className="px-4 py-6 text-center text-sm" style={{ color: "var(--muted)" }}>No matching actions</td></tr>
            ) : entries.map((e, i) => {
              const ok = e.status >= 200 && e.status < 300;
              return (
                <tr key={e.ts_ms + "-" + i}
                  style={{ background: i % 2 === 0 ? "var(--surface)" : "var(--surface2)", borderBottom: "1px solid var(--border)" }}>
                  <td className="px-4 py-3 text-xs whitespace-nowrap" style={{ color: "var(--muted)" }} title={e.ts}>
                    {ago(e.ts)}
                  </td>
                  <td className="px-4 py-3">
                    <div className="font-medium">{e.actor_name || "(unknown)"}</div>
                    <div className="font-mono text-xs" style={{ color: "var(--muted)" }}>
                      {e.actor_id}{e.scoped ? " · scoped" : ""}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <span className="font-mono text-xs px-1.5 py-0.5 rounded mr-2"
                      style={{ background: "var(--surface2)", color: "var(--text)" }}>{e.method}</span>
                    <span className="font-mono text-xs break-all" style={{ color: "var(--muted)" }}>{e.path}</span>
                  </td>
                  <td className="px-4 py-3 text-xs" style={{ color: ok ? "var(--green)" : "var(--red)" }}>{e.status}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)", maxWidth: 320 }}>
                    <span className="break-all line-clamp-2" title={e.detail}>{e.detail || "—"}</span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </div>
  );
}
