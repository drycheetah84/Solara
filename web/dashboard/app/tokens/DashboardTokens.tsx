"use client";

// Solara: scoped dashboard tokens. Only the master key can create, list or revoke them; a scoped token can log in to
// this dashboard (or call the API) and do only what its permissions allow, only for the stations / fleets picked here.
import { useCallback, useEffect, useState } from "react";
import { api, type ScopedPermission, type ScopedToken, type Station } from "../../lib/api";
import { fmt, ago } from "../../lib/utils";

function tokenStatus(t: ScopedToken): { label: string; color: string } {
  if (t.revoked_at) return { label: "Revoked", color: "var(--red)" };
  if (t.expires_at && Date.parse(t.expires_at) <= Date.now()) return { label: "Expired", color: "var(--muted)" };
  return { label: "Active", color: "var(--green)" };
}

const input = { background: "var(--surface2)", border: "1px solid var(--border)", color: "var(--text)" } as const;

export default function DashboardTokens() {
  const [master, setMaster] = useState<boolean | null>(null);
  const [perms, setPerms] = useState<ScopedPermission[]>([]);
  const [stations, setStations] = useState<Station[]>([]);
  const [tokens, setTokens] = useState<ScopedToken[] | null>(null);
  const [name, setName] = useState("");
  const [days, setDays] = useState("");
  const [pick, setPick] = useState<Set<string>>(new Set());
  const [scope, setScope] = useState<Set<string>>(new Set());
  const [created, setCreated] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [showAll, setShowAll] = useState(false);

  const load = useCallback(() => {
    api.scopedTokens().then(d => setTokens(d.tokens)).catch(e => setErr(e.message));
  }, []);

  useEffect(() => {
    api.scopedSelf()
      .then(s => {
        setMaster(s.master);
        if (!s.master) return;
        api.scopedTokenPerms().then(d => setPerms(d.permissions)).catch(e => setErr(e.message));
        api.stations().then(setStations).catch(() => setStations([]));
        load();
      })
      .catch(e => setErr(e.message));
  }, [load]);

  const toggle = (set: Set<string>, setter: (s: Set<string>) => void, v: string) => {
    const n = new Set(set);
    if (n.has(v)) n.delete(v); else n.add(v);
    setter(n);
  };

  async function create() {
    setErr(null);
    setCreated(null);
    setBusy(true);
    try {
      const d = parseInt(days, 10);
      const out = await api.createScopedToken({
        name, permissions: [...pick], stations: [...scope], ...(d > 0 ? { expires_in_days: d } : {}),
      });
      setCreated(out.token);
      setName(""); setDays(""); setPick(new Set()); setScope(new Set());
      load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  async function revoke(t: ScopedToken) {
    if (!confirm(`Revoke ${t.name || t.prefix}? Anyone using it loses access immediately.`)) return;
    try {
      const r = await api.revokeScopedToken(t.id);
      if (!r.success) setErr(r.error ?? "revoke failed");
      load();
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  if (master === false) {
    return (
      <p className="text-xs mb-8" style={{ color: "var(--muted)" }}>
        You are signed in with a scoped token. Only the master key can manage dashboard tokens.
      </p>
    );
  }

  const shown = (tokens ?? []).filter(t => showAll || tokenStatus(t).label === "Active");
  const cols = ["Name", "Prefix", "Permissions", "Stations / fleets", "Created", "Last used", "Expires", "Status", ""];

  return (
    <div className="mb-10">
      <div className="flex items-center justify-between mb-2">
        <h1 className="text-lg font-semibold" style={{ color: "var(--text)" }}>Dashboard Tokens</h1>
        <label className="flex items-center gap-2 text-xs" style={{ color: "var(--muted)" }}>
          <input type="checkbox" checked={showAll} onChange={e => setShowAll(e.target.checked)} />
          show revoked / expired
        </label>
      </div>
      <p className="text-xs mb-4" style={{ color: "var(--muted)" }}>
        A token can sign in here (or call the API as <span className="font-mono">x-dashboard-key</span>) and can only do
        what its permissions cover, only for the stations / fleets you pick. Routes that are not about one station need
        &ldquo;All stations&rdquo;. The token is shown once.
      </p>

      {err && (
        <div className="mb-4 px-4 py-3 rounded-md text-sm"
          style={{ background: "#2d1a1a", color: "var(--red)", border: "1px solid #5c2020" }}>{err}</div>
      )}
      {created && (
        <div className="mb-4 px-4 py-3 rounded-md text-sm break-all"
          style={{ background: "#16281b", color: "var(--green)", border: "1px solid #24502f" }}>
          Token created &mdash; copy it now, it will not be shown again:
          <div className="font-mono mt-1 select-all" style={{ color: "var(--text)" }}>{created}</div>
        </div>
      )}

      <div className="rounded-lg p-4 mb-5 flex flex-col gap-4" style={{ background: "var(--surface)", border: "1px solid var(--border)" }}>
        <div className="flex flex-wrap gap-3 items-center">
          <input value={name} onChange={e => setName(e.target.value)} placeholder="Name (who / what it is for)" maxLength={80}
            className="px-3 py-1.5 rounded-md text-sm" style={{ ...input, minWidth: 260 }} />
          <label className="text-xs" style={{ color: "var(--muted)" }}>Expires in days (blank = never)</label>
          <input value={days} onChange={e => setDays(e.target.value)} type="number" min={1} max={3650}
            className="px-3 py-1.5 rounded-md text-sm" style={{ ...input, width: 90 }} />
        </div>
        <div>
          <div className="text-xs mb-2" style={{ color: "var(--muted)" }}>Permissions</div>
          <div className="grid gap-x-6 gap-y-1 text-xs" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(320px, 1fr))" }}>
            {perms.map(p => (
              <label key={p.name} className="flex items-start gap-2">
                <input type="checkbox" checked={pick.has(p.name)} onChange={() => toggle(pick, setPick, p.name)} />
                <span><span className="font-mono" style={{ color: "var(--text)" }}>{p.name}</span>{" "}
                  <span style={{ color: "var(--muted)" }}>{p.description}</span></span>
              </label>
            ))}
          </div>
        </div>
        <div>
          <div className="text-xs mb-2" style={{ color: "var(--muted)" }}>Stations / fleets</div>
          <div className="flex flex-wrap gap-x-6 gap-y-1 text-xs">
            <label className="flex items-center gap-2">
              <input type="checkbox" checked={scope.has("*")} onChange={() => toggle(scope, setScope, "*")} />
              <span style={{ color: "var(--text)" }}>All stations (*)</span>
            </label>
            {stations.map(s => (
              <label key={s.station_id} className="flex items-center gap-2">
                <input type="checkbox" checked={scope.has(s.station_id)} onChange={() => toggle(scope, setScope, s.station_id)} />
                <span style={{ color: "var(--text)" }}>{s.station_name || s.station_id}</span>
                <span className="font-mono" style={{ color: "var(--muted)" }}>{s.station_id}</span>
              </label>
            ))}
          </div>
        </div>
        <div>
          <button onClick={create} disabled={busy || pick.size === 0 || scope.size === 0}
            className="px-3 py-1.5 text-xs rounded-md font-medium"
            style={{ background: "var(--accent, #2f81f7)", color: "#fff", opacity: busy || pick.size === 0 || scope.size === 0 ? 0.5 : 1 }}>
            Create token
          </button>
        </div>
      </div>

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
              <tr><td colSpan={cols.length} className="px-4 py-8 text-center text-sm" style={{ color: "var(--muted)" }}>No dashboard tokens</td></tr>
            ) : shown.map((t, i) => {
              const st = tokenStatus(t);
              return (
                <tr key={t.id} style={{ background: i % 2 === 0 ? "var(--surface)" : "var(--surface2)", borderBottom: "1px solid var(--border)" }}>
                  <td className="px-4 py-3 text-xs">{t.name || "—"}</td>
                  <td className="px-4 py-3 font-mono text-xs">{t.prefix}</td>
                  <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--muted)" }}>{t.permissions.join(", ")}</td>
                  <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--muted)" }}>{t.stations.join(", ")}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{t.created_at ? fmt(t.created_at) : "—"}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{t.last_used ? ago(t.last_used) : "never"}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{t.expires_at ? fmt(t.expires_at) : "never"}</td>
                  <td className="px-4 py-3 text-xs" style={{ color: st.color }}>{st.label}</td>
                  <td className="px-4 py-3 text-right">
                    {st.label === "Active" && (
                      <button onClick={() => revoke(t)} className="px-2.5 py-1 text-xs rounded-md font-medium"
                        style={{ background: "var(--red)", color: "#fff" }}>Revoke</button>
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
