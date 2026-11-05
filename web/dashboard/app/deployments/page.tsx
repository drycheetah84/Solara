"use client";

import { useEffect, useState } from "react";
import { api, type Deployment } from "../../lib/api";
import { fmt, ago } from "../../lib/utils";
import { EmptyRow } from "../../components/ui";
import { RefreshCw } from "lucide-react";

const COLS = "1.4fr 1fr 0.8fr 0.7fr 0.8fr 0.6fr 0.9fr 1fr";

export default function DeploymentsPage() {
  const [deps, setDeps] = useState<Deployment[] | null>(null);
  const [err, setErr]   = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  function load() {
    api.deployments()
      .then(d  => { setDeps(d); setErr(null); })
      .catch(e => setErr(e.message))
      .finally(() => setLoading(false));
  }

  useEffect(() => { load(); }, []);

  const online = deps?.filter(d => d.online).length ?? 0;

  return (
    <div className="mx-auto max-w-[1200px] pt-8 pb-14">
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6">
        <div>
          <p className="eyebrow mb-1.5">Infrastructure</p>
          <h1 className="text-[44px] leading-tight font-medium tracking-tight">Deployments</h1>
          <p className="text-[15px] mt-1.5 tabular" style={{ color: "var(--muted)" }}>
            {deps ? <><span className="font-medium" style={{ color: "var(--text)" }}>{deps.length}</span> total
            {" · "}<span className="font-medium" style={{ color: "var(--green)" }}>{online} online</span></> : "Loading…"}
          </p>
        </div>
        <button onClick={() => { setLoading(true); load(); }} disabled={loading} className="btn-dark" aria-label="Refresh">
          <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
        </button>
      </div>

      {err && <div className="card mb-4 px-5 py-3.5 text-[15px]" style={{ color: "var(--red)" }}>{err}</div>}

      <div className="overflow-x-auto">
        <div className="min-w-[1000px]">
          <div className="tbl-head grid items-center gap-2 px-5 py-3.5" style={{ gridTemplateColumns: COLS }}>
            {["Deployment", "IP", "Region", "Version", "Status", "Players", "Last event", "Created"].map(h => (
              <span key={h} className="text-[14px] font-bold">{h}</span>
            ))}
          </div>
          <div>
            {!deps ? (
              <div className="tbl-row px-5 py-8"><div className="skeleton h-5 w-1/3" /></div>
            ) : deps.length === 0 ? (
              <EmptyRow>No deployments running.</EmptyRow>
            ) : deps.map(d => (
              <div key={d.deployment_id} className="tbl-row grid items-center gap-2 px-5 py-3.5 text-[15px]" style={{ gridTemplateColumns: COLS }}>
                <span className="min-w-0">
                  <span className="block font-medium truncate">{d.deployment_name ?? "—"}</span>
                  <span className="block font-mono text-[12px] truncate tabular" style={{ color: "var(--muted)" }}>
                    {d.deployment_id.slice(0, 8)}…
                  </span>
                </span>
                <span className="font-mono text-[13px] truncate tabular" style={{ color: "var(--muted)" }}>{d.ip_address ?? "—"}</span>
                <span className="text-sm" style={{ color: "var(--muted)" }}>{d.region ?? "—"}</span>
                <span className="text-sm tabular" style={{ color: "var(--muted)" }}>{d.version ?? "—"}</span>
                <span>
                  <span
                    className="text-[11px] font-semibold uppercase tracking-[0.14em] px-2 py-0.5"
                    style={d.online
                      ? { color: "var(--green)", background: "rgba(139,212,80,0.1)", boxShadow: "inset 0 0 0 1px rgba(139,212,80,0.35)", borderRadius: 3 }
                      : { color: "var(--muted)", background: "rgba(255,255,255,0.04)", boxShadow: "inset 0 0 0 1px var(--line)", borderRadius: 3 }}>
                    {d.online ? "Online" : "Offline"}
                  </span>
                </span>
                <span className="tabular text-center" style={{ color: "var(--link)" }}>{d.player_count}</span>
                <span className="text-[13px] tabular" style={{ color: "var(--muted)" }}>{ago(d.last_event)}</span>
                <span className="text-[13px] tabular" style={{ color: "var(--muted)" }}>{fmt(d.created_at)}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
