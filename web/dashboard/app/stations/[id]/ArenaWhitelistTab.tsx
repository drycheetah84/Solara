"use client";

// Arena whitelists for one station (fleet). Each arena's TicketManager only lets the listed players join a team when
// that team's whitelist is on (TicketManager::CheckWhitelist: comma-separated player names). The values are module
// netvars, stored as "nv.module.<arena slot>.<variable>" station-config keys and pushed to the running servers by the
// injected DLL within ~10 s; the DLL then calls TicketManager::OnConfigChanged so the arena picks them up.
//   Soccer / jakeball arenas: bUseTeam0Whitelist + Team0Whitelist (red), bUseTeam1Whitelist + Team1Whitelist (blue)
//   Z-Drift arenas:           bUseWhitelist (both teams) + Team0Whitelist / Team1Whitelist
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type User } from "../../../lib/api";

type Mode = "teams" | "single";
type Arena = { slot: string; label: string; mode: Mode };

// Arena slots reported by the live servers' slot map (team-changer doors), 2026-11-03.
const GROUPS: { title: string; arenas: Arena[] }[] = [
  { title: "Soccer", arenas: [
    { slot: "DriftplexSoccerEast", label: "Driftplex Soccer East", mode: "teams" },
    { slot: "DriftplexSoccerWestFront", label: "Driftplex Soccer West (front)", mode: "teams" },
    { slot: "DriftplexSoccerWestBack", label: "Driftplex Soccer West (back)", mode: "teams" },
  ] },
  { title: "Jakeball / Tackleball", arenas: [
    { slot: "TKB_Prime", label: "Prime", mode: "teams" },
    { slot: "TKB_PlazaEast", label: "Plaza East", mode: "teams" },
    { slot: "TKB_PlazaWest", label: "Plaza West", mode: "teams" },
    { slot: "TKB_Bunker", label: "Bunker", mode: "teams" },
    { slot: "TKB_FieldhouseEast", label: "Fieldhouse East", mode: "teams" },
    { slot: "TKB_FieldhouseWest", label: "Fieldhouse West", mode: "teams" },
    ...[1, 2, 3, 4, 5, 6].map(n => ({ slot: `TKB_Driftplex0${n}`, label: `Driftplex ${n}`, mode: "teams" as Mode })),
  ] },
  { title: "Z-Drift", arenas: [1, 2, 3].map(n => ({ slot: `ZDrift_0${n}`, label: `Z-Drift ${n}`, mode: "single" as Mode })) },
];

const NV = "nv.module.";
const key = (slot: string, v: string) => `${NV}${slot}.${v}`;
const card: React.CSSProperties = { background: "var(--surface)", border: "1px solid var(--border)" };
const inputStyle: React.CSSProperties = { background: "var(--surface2)", border: "1px solid var(--border)", color: "var(--text)" };

function names(v: string | undefined): string[] {
  return (v ?? "").split(/[,\n]/).map(s => s.trim()).filter(Boolean);
}

type TeamState = { on: boolean; list: string };
type ArenaState = { red: TeamState; blue: TeamState };

function readArena(cfg: Record<string, string>, a: Arena): ArenaState {
  const truthy = (v?: string) => (v ?? "").toLowerCase() === "true";
  if (a.mode === "single") {
    const on = truthy(cfg[key(a.slot, "bUseWhitelist")]);
    return { red: { on, list: names(cfg[key(a.slot, "Team0Whitelist")]).join(", ") },
             blue: { on, list: names(cfg[key(a.slot, "Team1Whitelist")]).join(", ") } };
  }
  return { red: { on: truthy(cfg[key(a.slot, "bUseTeam0Whitelist")]), list: names(cfg[key(a.slot, "Team0Whitelist")]).join(", ") },
           blue: { on: truthy(cfg[key(a.slot, "bUseTeam1Whitelist")]), list: names(cfg[key(a.slot, "Team1Whitelist")]).join(", ") } };
}

function ArenaRow({ arena, cfg, playerNames, onSaved }: {
  arena: Arena; cfg: Record<string, string>; playerNames: string[]; onSaved: () => void;
}) {
  const initial = useMemo(() => readArena(cfg, arena), [cfg, arena]);
  const [st, setSt] = useState<ArenaState>(initial);   // the parent remounts this row when its saved values change
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  const [adding, setAdding] = useState<{ red: string; blue: string }>({ red: "", blue: "" });
  const dirty = JSON.stringify(st) !== JSON.stringify(initial);
  const active = initial.red.on || initial.blue.on;

  async function save() {
    setBusy(true); setMsg("");
    try {
      const set: Record<string, string> = {};
      const drop: string[] = [];
      const csv = (t: TeamState) => names(t.list).join(",");
      const put = (v: string, value: string, keep: boolean) => { if (keep) set[key(arena.slot, v)] = value; else if (cfg[key(arena.slot, v)] !== undefined) drop.push(key(arena.slot, v)); };
      if (arena.mode === "single") {
        const on = st.red.on || st.blue.on;
        put("bUseWhitelist", on ? "true" : "false", on);
      } else {
        put("bUseTeam0Whitelist", st.red.on ? "true" : "false", st.red.on);
        put("bUseTeam1Whitelist", st.blue.on ? "true" : "false", st.blue.on);
      }
      // a list is kept while its team is on or it still has names (so turning a team off does not lose the list)
      put("Team0Whitelist", csv(st.red), st.red.on || csv(st.red) !== "");
      put("Team1Whitelist", csv(st.blue), st.blue.on || csv(st.blue) !== "");
      if (Object.keys(set).length) await api.patchStationConfig(stationIdOf(), set);
      for (const k of drop) await api.deleteStationConfigKey(stationIdOf(), k);
      setMsg("Saved -- live within ~10 s");
      onSaved();
    } catch (e) {
      setMsg((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  function stationIdOf() { return (window.location.pathname.split("/").filter(Boolean)[1]) ?? ""; }

  const team = (side: "red" | "blue", title: string, color: string) => (
    <div className="flex-1 min-w-[260px]">
      <label className="flex items-center gap-2 text-xs mb-1.5" style={{ color }}>
        <input type="checkbox" checked={st[side].on}
          onChange={e => setSt(s => arena.mode === "single"
            ? { red: { ...s.red, on: e.target.checked }, blue: { ...s.blue, on: e.target.checked } }
            : { ...s, [side]: { ...s[side], on: e.target.checked } })} />
        {title} whitelist {arena.mode === "single" && side === "red" ? "(both teams)" : ""}
      </label>
      <textarea rows={2} value={st[side].list}
        placeholder="player names, comma separated"
        onChange={e => setSt(s => ({ ...s, [side]: { ...s[side], list: e.target.value } }))}
        className="w-full px-2.5 py-1.5 rounded-md text-xs font-mono outline-none" style={inputStyle} />
      <input value={adding[side]} list={`players-${arena.slot}`} placeholder="add player…"
        onChange={e => setAdding(a => ({ ...a, [side]: e.target.value }))}
        onKeyDown={e => {
          if (e.key !== "Enter") return;
          const n = adding[side].trim();
          if (n) setSt(s => ({ ...s, [side]: { ...s[side], list: [...names(s[side].list), n].filter((v, i, all) => all.indexOf(v) === i).join(", ") } }));
          setAdding(a => ({ ...a, [side]: "" }));
        }}
        className="w-full mt-1 px-2.5 py-1 rounded-md text-xs outline-none" style={inputStyle} />
    </div>
  );

  return (
    <div className="rounded-lg p-3 mb-2" style={card}>
      <div className="flex items-center justify-between mb-2">
        <div className="text-sm" style={{ color: "var(--text)" }}>
          {arena.label} <span className="font-mono text-xs" style={{ color: "var(--muted)" }}>{arena.slot}</span>
          {active && <span className="ml-2 text-xs" style={{ color: "var(--green)" }}>whitelisted</span>}
        </div>
        <div className="flex items-center gap-3">
          {msg && <span className="text-xs" style={{ color: "var(--muted)" }}>{msg}</span>}
          <button onClick={save} disabled={!dirty || busy}
            className="px-3 py-1 text-xs rounded-md font-medium"
            style={{ background: "var(--accent)", color: "#fff", opacity: !dirty || busy ? 0.4 : 1 }}>Save</button>
        </div>
      </div>
      <div className="flex flex-wrap gap-3">
        {team("red", "Red team", "var(--red)")}
        {team("blue", "Blue team", "var(--accent)")}
      </div>
      <datalist id={`players-${arena.slot}`}>{playerNames.map(n => <option key={n} value={n} />)}</datalist>
    </div>
  );
}

export default function ArenaWhitelistTab({ stationId }: { stationId: string }) {
  const [cfg, setCfg] = useState<Record<string, string> | null>(null);
  const [players, setPlayers] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const [custom, setCustom] = useState("");
  const [extra, setExtra] = useState<Arena[]>([]);

  const load = useCallback(() => {
    api.stationConfig(stationId).then(c => { setCfg(c); setErr(""); }).catch(e => setErr(e.message));
  }, [stationId]);
  useEffect(() => {
    load();
    api.users().then((u: User[]) => setPlayers([...new Set(u.map(x => x.username).filter(Boolean))].sort())).catch(() => setPlayers([]));
  }, [load]);

  // arenas that already have whitelist keys but are not in the built-in list (added via "custom slot")
  const known = new Set(GROUPS.flatMap(g => g.arenas.map(a => a.slot)));
  const fromCfg: Arena[] = [...new Set(Object.keys(cfg ?? {})
    .filter(k => k.startsWith(NV) && /Whitelist$/.test(k))
    .map(k => k.slice(NV.length, k.lastIndexOf("."))))]
    .filter(s => s !== "*" && !known.has(s))
    .map(s => ({ slot: s, label: s, mode: (cfg ?? {})[key(s, "bUseWhitelist")] !== undefined ? "single" : "teams" as Mode }));
  const customArenas = [...fromCfg, ...extra.filter(e => !fromCfg.some(f => f.slot === e.slot))];

  return (
    <div>
      <p className="text-xs mb-4" style={{ color: "var(--muted)" }}>
        Only the listed players can join that team in that arena while its whitelist is on. Names must match the
        players&apos; in-game names. Changes reach the running servers within about 10 seconds.
      </p>
      {err && <div className="mb-3 text-sm" style={{ color: "var(--red)" }}>{err}</div>}
      {!cfg ? <div className="text-sm" style={{ color: "var(--muted)" }}>Loading…</div> : (
        <>
          {GROUPS.map(g => (
            <div key={g.title} className="mb-5">
              <div className="text-xs uppercase tracking-wide mb-2" style={{ color: "var(--muted)" }}>{g.title}</div>
              {g.arenas.map(a => <ArenaRow key={a.slot + JSON.stringify(readArena(cfg, a))} arena={a} cfg={cfg} playerNames={players} onSaved={load} />)}
            </div>
          ))}
          <div className="mb-5">
            <div className="text-xs uppercase tracking-wide mb-2" style={{ color: "var(--muted)" }}>Other arenas</div>
            {customArenas.map(a => <ArenaRow key={a.slot + JSON.stringify(readArena(cfg, a))} arena={a} cfg={cfg} playerNames={players} onSaved={load} />)}
            <div className="flex gap-2 items-center">
              <input value={custom} onChange={e => setCustom(e.target.value)} placeholder="arena slot id (e.g. TKB_Prime)"
                className="px-2.5 py-1.5 rounded-md text-xs font-mono outline-none" style={{ ...inputStyle, minWidth: 260 }} />
              <button onClick={() => { const s = custom.trim(); if (s && !known.has(s)) setExtra(x => [...x, { slot: s, label: s, mode: "teams" }]); setCustom(""); }}
                className="px-3 py-1 text-xs rounded-md" style={{ border: "1px solid var(--border)", color: "var(--text)" }}>Add arena</button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
