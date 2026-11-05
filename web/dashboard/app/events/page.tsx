"use client";

import { useEffect, useMemo, useState } from "react";
import { api, type ServerEvent, type Deployment, type Station } from "../../lib/api";
import { ago } from "../../lib/utils";
import { EmptyRow, FleetIcon } from "../../components/ui";
import { Search } from "lucide-react";

type Enriched = ServerEvent & {
  deploymentName: string | null;
  stationId: string | null;
  stationName: string | null;
};

function EventRow({ e }: { e: Enriched }) {
  return (
    <div className="tbl-row px-5 py-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="font-mono text-[13px] px-2 py-0.5 rounded"
          style={{ background: "var(--surface2)", color: "var(--link)" }}>
          {e.event_name}
        </span>
        <span className="text-[13px] truncate" style={{ color: "var(--muted)" }}>
          {e.deploymentName ?? `${e.deployment_id.slice(0, 8)}…`}
        </span>
        <span className="flex-1" />
        <span className="text-[13px] tabular" style={{ color: "var(--muted)" }} title={e.created_at}>
          {ago(e.created_at)}
        </span>
      </div>
    </div>
  );
}

export default function EventsPage() {
  const [events, setEvents] = useState<ServerEvent[] | null>(null);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [stations, setStations] = useState<Station[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [stationFilter, setStationFilter] = useState("");

  useEffect(() => {
    let mounted = true;
    function load() {
      Promise.all([
        api.events(),
        // Joins only: a session without these permissions still sees the raw event list.
        api.deployments().catch(() => [] as Deployment[]),
        api.stations().catch(() => [] as Station[]),
      ]).then(([ev, dep, st]) => {
        if (!mounted) return;
        setEvents(ev);
        setDeployments(dep);
        setStations(st);
        setErr(null);
      }).catch(e => { if (mounted) setErr(e.message); });
    }
    load();
    const id = setInterval(load, 5000);
    return () => { mounted = false; clearInterval(id); };
  }, []);

  // event -> deployment -> station joins, grouped per station (newest group first).
  const { groups, ungrouped } = useMemo(() => {
    if (!events) return { groups: null, ungrouped: null } as const;
    const depById = new Map(deployments.map(d => [d.deployment_id, d]));
    const stById = new Map(stations.map(s => [s.station_id, s]));
    const q = query.trim().toLowerCase();

    const enriched: Enriched[] = [];
    for (const e of events) {
      if (q && !e.event_name.toLowerCase().includes(q)) continue;
      const dep = depById.get(e.deployment_id);
      const st = dep ? stById.get(dep.station_id) : undefined;
      enriched.push({
        ...e,
        deploymentName: dep?.deployment_name ?? null,
        stationId: st?.station_id ?? null,
        stationName: st?.station_name ?? null,
      });
    }
    enriched.sort((a, b) => b.created_at.localeCompare(a.created_at));

    const byStation = new Map<string, { name: string; items: Enriched[] }>();
    const lone: Enriched[] = [];
    for (const e of enriched) {
      if (stationFilter && e.stationId !== stationFilter) continue;
      if (e.stationId) {
        const g = byStation.get(e.stationId) ?? { name: e.stationName ?? e.stationId, items: [] };
        g.items.push(e);
        byStation.set(e.stationId, g);
      } else {
        lone.push(e);
      }
    }
    const groups = [...byStation.values()].sort((a, b) =>
      b.items[0].created_at.localeCompare(a.items[0].created_at));
    return { groups, ungrouped: lone };
  }, [events, deployments, stations, query, stationFilter]);

  const stationOptions = useMemo(
    () => [...stations].sort((a, b) => a.station_name.localeCompare(b.station_name)),
    [stations]);

  return (
    <div className="mx-auto max-w-[1200px] pt-8 pb-14">
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6">
        <div>
          <p className="eyebrow mb-1.5">Live feed</p>
          <h1 className="text-[44px] leading-tight font-medium tracking-tight">Events</h1>
          <p className="text-[15px] mt-1.5 tabular" style={{ color: "var(--muted)" }}>
            {events ? <><span className="font-medium" style={{ color: "var(--text)" }}>{events.length}</span> recent</> : "Loading…"}
            {" · auto-refreshes every 5 seconds"}
          </p>
        </div>
      </div>

      {err && <div className="card mb-4 px-5 py-3.5 text-[15px]" style={{ color: "var(--red)" }}>{err}</div>}

      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-3 mb-5">
        <label className="relative flex-1 min-w-52">
          <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" style={{ color: "var(--muted)" }} />
          <input className="field pl-9!" value={query} onChange={e => setQuery(e.target.value)}
            placeholder="Search event names…" aria-label="Search events" />
        </label>
        <select className="field w-auto!" value={stationFilter} onChange={e => setStationFilter(e.target.value)} aria-label="Filter by station">
          <option value="">All stations</option>
          {stationOptions.map(s => <option key={s.station_id} value={s.station_id}>{s.station_name}</option>)}
        </select>
      </div>

      {!groups ? (
        <div className="card p-5"><div className="skeleton h-5 w-1/3" /></div>
      ) : groups.length === 0 && ungrouped!.length === 0 ? (
        <div className="card">
          <EmptyRow>{query || stationFilter ? "No events match these filters." : "No events yet."}</EmptyRow>
        </div>
      ) : (
        <div className="space-y-5">
          {groups.map(g => (
            <section key={g.name}>
              <div className="flex items-center gap-3 px-1 mb-2">
                <FleetIcon name={g.name} />
                <h2 className="text-[20px] font-medium tracking-tight">{g.name}</h2>
                <span className="text-[13px] tabular" style={{ color: "var(--muted)" }}>
                  {g.items.length} event{g.items.length === 1 ? "" : "s"}
                </span>
              </div>
              <div className="card p-0! overflow-hidden">
                {g.items.map(e => <EventRow key={e.event_id} e={e} />)}
              </div>
            </section>
          ))}

          {ungrouped!.length > 0 && (
            <section>
              <div className="flex items-center gap-3 px-1 mb-2">
                <h2 className="text-[20px] font-medium tracking-tight" style={{ color: "var(--muted)" }}>Other</h2>
                <span className="text-[13px] tabular" style={{ color: "var(--muted)" }}>
                  {ungrouped!.length} event{ungrouped!.length === 1 ? "" : "s"}
                </span>
              </div>
              <div className="card p-0! overflow-hidden">
                {ungrouped!.map(e => <EventRow key={e.event_id} e={e} />)}
              </div>
            </section>
          )}
        </div>
      )}
    </div>
  );
}
