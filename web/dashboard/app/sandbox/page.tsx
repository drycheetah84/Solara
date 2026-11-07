"use client";

// Sandbox content (Developers only; the backend refuses every scoped token on /api/sandbox/objects).
// Game servers load ScrapRun trap bundles and cloud gamemodes from Axiom's S3 bucket. Their own HTTPS client cannot
// verify Amazon's certificate, so the DLL routes those fetches through the station backend, which caches each object
// on the hub. This lists what has been fetched: one row per bundle / gamemode with its files.
import { useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { api, type SandboxObject, type SandboxObjects } from "../../lib/api";
import { ago, fmt } from "../../lib/utils";
import { EmptyRow } from "../../components/ui";

const COLS = "1.6fr 0.8fr 1.6fr 0.6fr 0.55fr 0.9fr 0.9fr";

const BOXES: Record<string, string> = { "10.10.1.10": "game box 1", "10.20.1.10": "game box 2" };
const server = (ip: string) => (ip ? BOXES[ip] ?? ip : "—");

function size(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

/** Human label for one file of a bundle. */
function fileLabel(o: SandboxObject): string {
  switch (o.kind) {
    case "projects/v1": return "info";
    case "configs/v1": return "trap bundle";
    case "releases/v1": return `release ${o.version}`;
    case "gamemodes/v2": return `gamemode ${o.version.split("-")[0].replace(/^0+/, "") || "0"}`;
    case "quests/v2": return `quests ${o.version.split("-")[0].replace(/^0+/, "") || "0"}`;
    default: return o.kind;
  }
}

type Group = {
  id: string; name: string; type: string; files: SandboxObject[]; bytes: number; hits: number;
  last_at: string; last_client: string; failed: boolean;
};

function group(objs: SandboxObject[]): Group[] {
  const by = new Map<string, Group>();
  for (const o of objs) {
    const key = o.id || o.path;
    let g = by.get(key);
    if (!g) {
      g = { id: o.id, name: o.name, type: o.type, files: [], bytes: 0, hits: 0, last_at: "", last_client: "", failed: false };
      by.set(key, g);
    }
    g.files.push(o);
    g.bytes += o.bytes;
    g.hits += o.hits;
    g.name ||= o.name;
    g.type ||= o.type;
    g.failed ||= o.failed_fetch;
    if (o.last_at && o.last_at > g.last_at) { g.last_at = o.last_at; g.last_client = o.last_client; }
  }
  const out = [...by.values()];
  for (const g of out) g.files.sort((a, b) => a.kind.localeCompare(b.kind));
  return out.sort((a, b) =>
    Number(b.failed) - Number(a.failed) || (a.name || "~").localeCompare(b.name || "~") || a.id.localeCompare(b.id));
}

export default function SandboxPage() {
  const [admin, setAdmin] = useState<boolean | null>(null);
  const [data, setData] = useState<SandboxObjects | null>(null);
  const [err, setErr] = useState("");
  const [loading, setLoading] = useState(true);
  const [q, setQ] = useState("");

  function load() {
    setLoading(true);
    api.sandboxObjects()
      .then(d => { setData(d); setErr(""); })
      .catch(e => setErr((e as Error).message))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    fetch("/me", { cache: "no-store" }).then(r => (r.ok ? r.json() : null)).then(m => {
      setAdmin(!!m?.admin);
      if (m?.admin) load(); else setLoading(false);
    }).catch(() => { setAdmin(false); setLoading(false); });
  }, []);

  const groups = useMemo(() => group(data?.objects ?? []), [data]);
  const shown = useMemo(() => {
    const s = q.trim().toLowerCase();
    if (!s) return groups;
    return groups.filter(g => g.name.toLowerCase().includes(s) || g.id.toLowerCase().includes(s) || g.type.toLowerCase().includes(s));
  }, [groups, q]);
  const failed = groups.filter(g => g.failed).length;

  if (admin === false) {
    return (
      <div className="mx-auto max-w-[1200px] pt-8 pb-14">
        <div className="card px-5 py-4 text-[15px]" style={{ color: "var(--muted)" }}>Developers only.</div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-[1200px] pt-8 pb-14">
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6">
        <div>
          <p className="eyebrow mb-1.5">Game content</p>
          <h1 className="text-[44px] leading-tight font-medium tracking-tight">Sandbox bundles</h1>
          <p className="text-[15px] mt-1.5 tabular" style={{ color: "var(--muted)" }}>
            {data ? <>
              <span className="font-medium" style={{ color: "var(--text)" }}>{groups.length}</span> bundles
              {" · "}{data.objects.length} files · {size(data.total_bytes)} cached on the hub
              {failed > 0 && <>{" · "}<span className="font-medium" style={{ color: "var(--red)" }}>{failed} failing</span></>}
            </> : "Loading…"}
          </p>
          <p className="text-[13px] mt-1" style={{ color: "var(--muted)" }}>
            ScrapRun trap bundles and cloud gamemodes the game servers load from Axiom&apos;s S3 bucket, fetched through the
            station backend and kept on the hub. Fetch counts are since the backend last restarted.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <input value={q} onChange={e => setQ(e.target.value)} placeholder="Search name or id"
            className="rounded-md px-3 py-2 text-sm outline-none"
            style={{ background: "var(--bg)", border: "1px solid var(--border)", color: "var(--text)" }} />
          <button onClick={load} disabled={loading} className="btn-dark" aria-label="Refresh">
            <RefreshCw size={16} className={loading ? "animate-spin" : ""} />
          </button>
        </div>
      </div>

      {err && <div className="card mb-4 px-5 py-3.5 text-[15px]" style={{ color: "var(--red)" }}>{err}</div>}

      <div className="overflow-x-auto">
        <div className="min-w-[1000px]">
          <div className="tbl-head grid items-center gap-2 px-5 py-3.5" style={{ gridTemplateColumns: COLS }}>
            {["Bundle", "Type", "Files", "Size", "Fetches", "Last fetch", "Requested by"].map(h => (
              <span key={h} className="text-[14px] font-bold">{h}</span>
            ))}
          </div>
          <div>
            {!data ? (
              <div className="tbl-row px-5 py-8"><div className="skeleton h-5 w-1/3" /></div>
            ) : shown.length === 0 ? (
              <EmptyRow>{groups.length === 0 ? "Nothing fetched yet." : "No bundle matches that search."}</EmptyRow>
            ) : shown.map(g => (
              <div key={g.id || g.files[0].path} className="tbl-row grid items-start gap-2 px-5 py-3.5 text-[15px]"
                style={{ gridTemplateColumns: COLS, ...(g.failed ? { boxShadow: "inset 3px 0 0 var(--red)" } : {}) }}>
                <span className="min-w-0">
                  <span className="block font-medium truncate">{g.name || "Unnamed"}</span>
                  <span className="block font-mono text-[12px] truncate tabular" style={{ color: "var(--muted)" }} title={g.id}>{g.id}</span>
                </span>
                <span className="text-sm" style={{ color: "var(--muted)" }}>{g.type || "—"}</span>
                <span className="flex flex-wrap gap-1.5">
                  {g.files.map(f => (
                    <span key={f.path} title={`${f.path}${f.cached_at ? `\ncached ${fmt(f.cached_at)}` : ""}${f.last_error ? `\n${f.last_error}` : ""}`}
                      className="text-[11px] font-semibold px-2 py-0.5 tabular"
                      style={f.failed_fetch
                        ? { color: "var(--red)", background: "rgba(255,90,90,0.08)", boxShadow: "inset 0 0 0 1px rgba(255,90,90,0.35)", borderRadius: 3 }
                        : { color: "var(--muted)", background: "rgba(255,255,255,0.04)", boxShadow: "inset 0 0 0 1px var(--line)", borderRadius: 3 }}>
                      {fileLabel(f)}{f.failed_fetch ? " · failed" : ""}
                    </span>
                  ))}
                </span>
                <span className="text-[13px] tabular" style={{ color: "var(--muted)" }}>{g.bytes ? size(g.bytes) : "—"}</span>
                <span className="tabular" style={{ color: "var(--link)" }}>{g.hits}</span>
                <span className="text-[13px] tabular" style={{ color: "var(--muted)" }}>{g.last_at ? ago(g.last_at) : "—"}</span>
                <span className="text-[13px]" style={{ color: "var(--muted)" }}>{server(g.last_client)}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
