"use client";

import { useEffect, useState, useCallback } from "react";
import { api, type TelemetryEvent } from "../../lib/api";
import { EmptyRow } from "../../components/ui";
import { Search, X, Pause, Play } from "lucide-react";

function ago(ts: string) {
  const diff = Date.now() - new Date(ts).getTime();
  if (diff < 1000) return "just now";
  if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`;
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
  return new Date(ts).toLocaleTimeString();
}

const COLS = "0.7fr 1.2fr 2fr";

export default function TelemetryPage() {
  const [events, setEvents]   = useState<TelemetryEvent[] | null>(null);
  const [err, setErr]         = useState<string | null>(null);
  const [selected, setSelected] = useState<TelemetryEvent | null>(null);
  const [filter, setFilter]   = useState("");
  const [live, setLive]       = useState(true);

  const load = useCallback(() => {
    api.telemetry(500)
      .then(setEvents)
      .catch(e => setErr(e.message));
  }, []);

  useEffect(() => {
    load();
    if (!live) return;
    const t = setInterval(load, 2000);
    return () => clearInterval(t);
  }, [load, live]);

  const filtered = events?.filter(e =>
    !filter || e.eventName.toLowerCase().includes(filter.toLowerCase()) || (e.payload ?? "").toLowerCase().includes(filter.toLowerCase())
  ) ?? [];

  let prettyPayload = "";
  if (selected) {
    try { prettyPayload = JSON.stringify(JSON.parse(selected.payload ?? ""), null, 2); }
    catch { prettyPayload = selected.payload ?? ""; }
  }

  return (
    <div className="mx-auto max-w-[1200px] pt-8 pb-14">
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6">
        <div>
          <p className="eyebrow mb-1.5">Diagnostics</p>
          <h1 className="text-[44px] leading-tight font-medium tracking-tight">Telemetry</h1>
          <p className="text-[15px] mt-1.5 tabular" style={{ color: "var(--muted)" }}>
            {events ? <><span className="font-medium" style={{ color: "var(--text)" }}>{filtered.length}</span> event{filtered.length === 1 ? "" : "s"}{filter ? " match" : ""}</> : "Loading…"}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            role="switch"
            aria-checked={live}
            onClick={() => setLive(l => !l)}
            className="flex items-center gap-2.5 text-[14px] font-medium px-1 py-1"
            style={{ color: live ? "var(--text)" : "var(--muted)" }}
          >
            <span
              className="relative w-10 h-[22px] rounded-full transition-colors"
              style={{ background: live ? "var(--green)" : "#1a2334", boxShadow: "inset 0 0 0 1px var(--line)" }}
            >
              <span className="absolute top-[3px] h-4 w-4 rounded-full bg-white transition-all"
                style={{ left: live ? "calc(100% - 19px)" : "3px" }} />
            </span>
            {live ? "Live" : "Paused"}
          </button>
          <button onClick={load} className="btn-dark !py-2 text-sm">Refresh</button>
        </div>
      </div>

      {err && <div className="card mb-4 px-5 py-3.5 text-[15px]" style={{ color: "var(--red)" }}>{err}</div>}

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_380px] items-start">
        {/* Event list */}
        <div className="min-w-0">
          <label className="relative block mb-4">
            <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" style={{ color: "var(--muted)" }} />
            <input className="field !pl-9" placeholder="Filter by event name or payload…"
              value={filter} onChange={e => setFilter(e.target.value)} aria-label="Filter telemetry" />
          </label>

          <div className="tbl-head grid items-center gap-2 px-5 py-3.5" style={{ gridTemplateColumns: COLS }}>
            {["Time", "Event", "Preview"].map(h => (
              <span key={h} className="text-[14px] font-bold">{h}</span>
            ))}
          </div>
          <div className="max-h-[60vh] overflow-y-auto">
            {!events ? (
              <div className="tbl-row px-5 py-8"><div className="skeleton h-5 w-1/3" /></div>
            ) : filtered.length === 0 ? (
              <EmptyRow>{filter ? "Nothing matches this filter." : "No events yet."}</EmptyRow>
            ) : filtered.map(ev => {
              const active = selected?.id === ev.id;
              return (
                <button
                  key={ev.id}
                  onClick={() => setSelected(active ? null : ev)}
                  className="tbl-row grid items-center gap-2 px-5 py-3 w-full text-left"
                  style={{
                    gridTemplateColumns: COLS,
                    background: active ? "rgba(124,199,255,0.06)" : undefined,
                    boxShadow: active ? "inset 2px 0 0 var(--link)" : undefined,
                  }}
                >
                  <span className="text-[13px] tabular whitespace-nowrap" style={{ color: "var(--muted)" }}>{ago(ev.timestamp)}</span>
                  <span className="font-mono text-[13px] truncate" style={{ color: "var(--link)" }}>{ev.eventName}</span>
                  <span className="text-[13px] truncate" style={{ color: "var(--muted)" }}>{(ev.payload ?? "").slice(0, 120)}</span>
                </button>
              );
            })}
          </div>
        </div>

        {/* Payload inspector */}
        <div className="card overflow-hidden lg:sticky lg:top-6">
          {!selected ? (
            <EmptyRow>Pick an event to inspect its payload.</EmptyRow>
          ) : (
            <div>
              <div className="flex items-center justify-between gap-2 px-5 py-3.5" style={{ borderBottom: "1px solid var(--line)" }}>
                <span className="font-mono text-[13px] font-medium truncate" style={{ color: "var(--link)" }}>{selected.eventName}</span>
                <button onClick={() => setSelected(null)} className="p-1 rounded hover:opacity-80 shrink-0" style={{ color: "var(--muted)" }} aria-label="Close inspector">
                  <X size={15} />
                </button>
              </div>
              <div className="px-5 py-2.5 text-[13px] tabular" style={{ color: "var(--muted)", borderBottom: "1px solid var(--line)" }}>
                {new Date(selected.timestamp).toLocaleString()}
              </div>
              <pre className="p-5 text-[12px] font-mono overflow-auto whitespace-pre-wrap break-all" style={{ maxHeight: "50vh", color: "var(--text)" }}>
                {prettyPayload}
              </pre>
            </div>
          )}
        </div>
      </div>

      <p className="mt-4 text-[13px] flex items-center gap-2" style={{ color: "var(--muted)" }}>
        {live ? <Pause size={13} /> : <Play size={13} />}
        {live ? "Updating every 2 seconds" : "Paused — refresh manually"}
      </p>
    </div>
  );
}
