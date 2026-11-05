"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { api, type Station } from "../lib/api";
import { EmptyRow, FleetIcon } from "../components/ui";
import { Mascot } from "../components/Mascot";
import { RefreshCw, Trash2 } from "lucide-react";

const ROW = "grid grid-cols-[52px_1fr_110px_130px_52px] items-center gap-2 px-5 py-4";

export default function HomePage() {
  const router = useRouter();
  const [me, setMe]                 = useState<{ name: string; admin: boolean } | null>(null);
  const [stations, setStations]     = useState<Station[] | null>(null);
  const [err, setErr]               = useState<string | null>(null);
  const [loading, setLoading]       = useState(false);
  const [creating, setCreating]     = useState(false);
  const [newName, setNewName]       = useState("");
  const [saving, setSaving]         = useState(false);
  const [delTarget, setDelTarget]   = useState<Station | null>(null);
  const [deleting, setDeleting]     = useState(false);
  const [delErr, setDelErr]         = useState<string | null>(null);
  const [onlineOnly, setOnlineOnly] = useState(() => {
    // UI preference only. Storage can be unavailable (private mode); fall back to "all".
    try { return typeof window !== "undefined" && window.localStorage.getItem("halcyon-online-only") === "1"; }
    catch { return false; }
  });
  function toggleOnline() {
    setOnlineOnly(v => {
      const next = !v;
      try { window.localStorage.setItem("halcyon-online-only", next ? "1" : "0"); } catch {}
      return next;
    });
  }

  function fetchStations() {
    return api.stations()
      .then(d  => { setStations(d); setErr(null); })
      .catch(e => setErr(e.message));
  }
  function load() {
    setLoading(true);
    fetchStations().finally(() => setLoading(false));
  }

  useEffect(() => {
    fetchStations();
    fetch("/me", { cache: "no-store" }).then(r => (r.ok ? r.json() : null)).then(setMe).catch(() => {});
  }, []);

  async function createStation() {
    if (!newName.trim()) return;
    setSaving(true);
    try {
      const res = await api.createStation(newName.trim());
      if (res.station_id) { setCreating(false); setNewName(""); load(); }
    } finally { setSaving(false); }
  }

  // The backend refuses a station that currently has players on it unless force=true, so a populated
  // station takes a second, deliberate click rather than being one misclick away from deletion.
  async function deleteStation(force: boolean) {
    if (!delTarget) return;
    setDeleting(true);
    setDelErr(null);
    try {
      const res = await api.deleteStation(delTarget.station_id, force);
      if (!res.success) { setDelErr(res.error ?? "Delete failed"); return; }
      setDelTarget(null);
      load();
    } catch (e: unknown) {
      setDelErr(e instanceof Error ? e.message : "Unknown error");
    } finally { setDeleting(false); }
  }

  const { rows, totals } = useMemo(() => {
    if (!stations) return { rows: null, totals: null } as const;
    const players = stations.reduce((n, s) => n + (s.online ? s.player_count : 0), 0);
    const rows = stations
      .filter(s => !onlineOnly || s.online)
      .sort((a, b) =>
        Number(b.online) - Number(a.online) ||
        b.player_count - a.player_count ||
        a.station_name.localeCompare(b.station_name));
    return { rows, totals: { players, online: stations.filter(s => s.online).length } };
  }, [stations, onlineOnly]);

  return (
    <div className="mx-auto max-w-[1200px] pt-14 pb-20">
      <div className="grid gap-8 lg:grid-cols-[minmax(0,800px)_1fr] items-start">
        {/* ── Left: station picker ─────────────────────────── */}
        <div className="min-w-0">
          <h1 className="text-[44px] leading-tight font-medium tracking-tight">
            {me ? <>Welcome <span className="grad-name">{me.name}</span>!</> : "Station Dashboard"}
          </h1>

          <p className="text-[20px] mt-9">Please choose a station:</p>
          {totals && (
            <p className="text-[15px] mt-1.5 tabular" style={{ color: "var(--muted)" }}>
              <span className="font-medium" style={{ color: "var(--text)" }}>{totals.online}</span> online
              {" · "}<span className="font-medium" style={{ color: "var(--green)" }}>{totals.players}</span> players
            </p>
          )}

          {/* Controls */}
          <div className="flex items-center gap-4 mt-7 mb-4">
            <button role="switch" aria-checked={onlineOnly} aria-label="Only show online stations"
              className="toggle" onClick={toggleOnline}>
              <span className="knob" />
            </button>
            <span className="text-[17px]">Only show online stations</span>
            <div className="flex-1" />
            <button onClick={() => setCreating(true)} className="btn-dark min-w-0! text-sm">+ New</button>
            <button onClick={load} disabled={loading} className="btn-dark" aria-label="Refresh">
              <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
            </button>
          </div>

          {/* Station table */}
          <div className={`tbl-head ${ROW}`}>
            <span />
            <span className="text-[17px] font-bold">Station Name</span>
            <span className="text-[17px] font-bold text-center">Online</span>
            <span className="text-[17px] font-bold text-center">Player Count</span>
            <span />
          </div>
          <div>
            {!rows ? (
              [...Array(6)].map((_, i) => (
                <div key={i} className={`tbl-row ${ROW}`}>
                  <div className="skeleton h-11 w-11" />
                  <div className="skeleton h-5 w-48" />
                  <div className="flex justify-center"><div className="skeleton h-9 w-9" /></div>
                  <div className="skeleton h-5 w-10 mx-auto" />
                  <span />
                </div>
              ))
            ) : rows.length === 0 ? (
              <EmptyRow>{onlineOnly ? "No online stations" : "No stations found"}</EmptyRow>
            ) : rows.map(s => (
              <div key={s.station_id} role="button" tabIndex={0}
                onClick={() => router.push(`/stations/${s.station_id}`)}
                onKeyDown={e => { if (e.key === "Enter") router.push(`/stations/${s.station_id}`); }}
                className={`tbl-row ${ROW} w-full text-left cursor-pointer hover:bg-[rgba(124,199,255,0.05)]`}>
                <FleetIcon name={s.station_name} />
                <span className="text-[17px] truncate">{s.station_name}</span>
                <span className="flex justify-center">
                  <span className={`status-sq ${s.online ? "on" : "off"}`} aria-label={s.online ? "Online" : "Offline"} />
                </span>
                <span className="text-[17px] text-center tabular">{s.player_count}</span>
                {/* stopPropagation: the row itself opens the station page. */}
                <button onClick={e => { e.stopPropagation(); setDelErr(null); setDelTarget(s); }}
                  title="Delete station" aria-label={`Delete ${s.station_name}`}
                  className="btn-dark min-w-0! p-2!" style={{ color: "var(--red)" }}>
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
          </div>

          {err && (
            <div className="card mt-4 px-5 py-4 text-[15px]" style={{ color: "var(--red)" }}>
              Couldn&apos;t reach the backend: {err}
            </div>
          )}
        </div>

        {/* ── Right: mascot art ────────────────────────────── */}
        <Mascot />
      </div>

      {/* ── Create modal ───────────────────────────────────── */}
      {creating && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4" style={{ background: "rgba(0,0,0,0.65)" }}
          onClick={e => { if (e.target === e.currentTarget) setCreating(false); }}
          role="dialog" aria-modal="true" aria-label="Create station">
          <div className="card w-full max-w-sm p-6">
            <h2 className="text-[24px] font-medium mb-5">Create Station</h2>
            <label className="block text-sm mb-1.5" style={{ color: "var(--muted)" }}>Station Name</label>
            <input autoFocus className="field mb-5" value={newName}
              onChange={e => setNewName(e.target.value)}
              onKeyDown={e => e.key === "Enter" && createStation()}
              placeholder="My Station" />
            <div className="flex gap-2 justify-end">
              <button onClick={() => setCreating(false)} className="btn-dark text-sm">Cancel</button>
              <button onClick={createStation} disabled={saving || !newName.trim()} className="btn-light py-2.5! px-5! text-sm">
                {saving ? "Creating…" : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Delete confirmation ────────────────────────────── */}
      {delTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4" style={{ background: "rgba(0,0,0,0.65)" }}
          onClick={e => { if (e.target === e.currentTarget) setDelTarget(null); }}
          role="dialog" aria-modal="true" aria-label="Delete station">
          <div className="card w-full max-w-sm p-6">
            <h2 className="text-[22px] font-medium mb-3">Delete “{delTarget.station_name}”?</h2>
            <p className="text-sm mb-4" style={{ color: "var(--muted)" }}>
              Removes the station along with its {delTarget.deployments} deployment(s), roles, events
              and EOS sessions. This cannot be undone.
            </p>
            {delTarget.player_count > 0 && (
              <div className="card mb-4 px-4 py-3 text-sm" style={{ color: "var(--red)" }}>
                {delTarget.player_count} player(s) are on this station right now.
              </div>
            )}
            {delErr && <div className="card mb-4 px-4 py-3 text-sm" style={{ color: "var(--red)" }}>{delErr}</div>}
            <div className="flex gap-2 justify-end">
              <button onClick={() => setDelTarget(null)} className="btn-dark text-sm">Cancel</button>
              <button onClick={() => deleteStation(delTarget.player_count > 0 || delErr !== null)} disabled={deleting}
                className="btn-light py-2.5! px-5! text-sm" style={{ background: "var(--red)", color: "#fff" }}>
                {deleting ? "Deleting…" : delTarget.player_count > 0 ? "Delete anyway" : "Delete"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
