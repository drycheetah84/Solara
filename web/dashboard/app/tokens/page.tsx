"use client";

// Solara: per-server identity tokens (docs/SERVER_IDENTITY.md). Spin-ups mint a server token per game
// server; agents may hold an agent token. Secrets are never shown -- only the display prefix. New tokens
// are issued on the backend CLI (`solara server-token issue` / `agent-token issue`).
import { useCallback, useEffect, useState } from "react";
import { api, type ServerToken } from "../../lib/api";
import { fmt, ago } from "../../lib/utils";

function status(t: ServerToken): { label: string; color: string } {
  if (t.revoked_at) return { label: "Revoked", color: "var(--red)" };
  const now = Date.now();
  const exp = [t.expires_at, t.absolute_expiry].filter(Boolean).map(s => Date.parse(s as string));
  if (exp.some(e => !Number.isNaN(e) && e <= now)) return { label: "Expired", color: "var(--muted)" };
  return { label: "Active", color: "var(--green)" };
}

export default function TokensPage() {
  const [tokens, setTokens] = useState<ServerToken[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [showAll, setShowAll] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(() => {
    api.serverTokens()
      .then(d => { setTokens(d.tokens); setErr(null); })
      .catch(e => setErr(e.message));
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, 10000);
    return () => clearInterval(id);
  }, [load]);

  async function revoke(t: ServerToken) {
    if (!confirm(`Revoke ${t.prefix}? The game server or agent using it loses access immediately.`)) return;
    setBusy(t.id);
    try {
      const r = await api.revokeServerToken(t.id);
      if (!r.success) setErr(r.error ?? "revoke failed");
      load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  const shown = (tokens ?? []).filter(t => showAll || status(t).label === "Active");
  const cols = ["Prefix", "Kind", "Box", "Station", "Deployment", "Issued", "Last seen", "Expires", "Status", ""];

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <h1 className="text-lg font-semibold" style={{ color: "var(--text)" }}>Server Tokens</h1>
        <label className="flex items-center gap-2 text-xs" style={{ color: "var(--muted)" }}>
          <input type="checkbox" checked={showAll} onChange={e => setShowAll(e.target.checked)} />
          show revoked / expired
        </label>
      </div>
      <p className="text-xs mb-5" style={{ color: "var(--muted)" }}>
        Identity tokens held by game servers and allocator agents. Secrets are never displayed; issue new
        tokens with <span className="font-mono">solara server-token issue</span> on the backend.
      </p>

      {err && (
        <div className="mb-4 px-4 py-3 rounded-md text-sm"
          style={{ background: "#2d1a1a", color: "var(--red)", border: "1px solid #5c2020" }}>
          {err}
        </div>
      )}

      <div className="rounded-lg overflow-x-auto" style={{ border: "1px solid var(--border)" }}>
        <table className="w-full text-sm">
          <thead>
            <tr style={{ background: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {cols.map(h => (
                <th key={h} className="px-4 py-3 text-left font-medium text-xs" style={{ color: "var(--muted)" }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {!tokens ? (
              <tr><td colSpan={cols.length} className="px-4 py-8 text-center text-sm" style={{ color: "var(--muted)" }}>Loading…</td></tr>
            ) : shown.length === 0 ? (
              <tr><td colSpan={cols.length} className="px-4 py-8 text-center text-sm" style={{ color: "var(--muted)" }}>No tokens</td></tr>
            ) : shown.map((t, i) => {
              const st = status(t);
              return (
                <tr key={t.id} style={{ background: i % 2 === 0 ? "var(--surface)" : "var(--surface2)", borderBottom: "1px solid var(--border)" }}>
                  <td className="px-4 py-3 font-mono text-xs">{t.prefix}</td>
                  <td className="px-4 py-3 text-xs">{t.kind}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{t.box ?? "—"}</td>
                  <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--muted)" }}>{t.station_id ?? "—"}</td>
                  <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--muted)" }}>
                    {t.deployment_id ? t.deployment_id.slice(0, 8) + "…" : "—"}
                  </td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{t.issued_at ? fmt(t.issued_at) : "—"}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{t.last_seen ? ago(t.last_seen) : "never"}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{t.expires_at ? fmt(t.expires_at) : "—"}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: st.color }}>
                    {st.label}{t.revoke_reason ? ` (${t.revoke_reason})` : ""}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {st.label === "Active" && (
                      <button onClick={() => revoke(t)} disabled={busy === t.id}
                        className="px-2.5 py-1 text-xs rounded-md font-medium"
                        style={{ background: "var(--red)", color: "#fff", opacity: busy === t.id ? 0.5 : 1 }}>
                        Revoke
                      </button>
                    )}
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
