"use client";

// Title data editor (Developers only; the backend refuses every scoped token on /api/titledata).
// Players download title data once, when their game starts, so a change shows on their next launch.
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, type TitleDataKnown, type TitleDataList } from "../../lib/api";
import { fmt } from "../../lib/utils";

const card: React.CSSProperties = { background: "var(--surface)", border: "1px solid var(--border)" };
const inputCls = "w-full rounded-md px-3 py-2 text-sm font-mono outline-none";
const inputStyle: React.CSSProperties = { background: "var(--bg)", border: "1px solid var(--border)", color: "var(--text)" };

/** A JSON object whose values are all booleans gets toggles instead of a text box (StationBrowserFlair). */
function boolObject(v: string): Record<string, boolean> | null {
  try {
    const o = JSON.parse(v);
    if (!o || typeof o !== "object" || Array.isArray(o)) return null;
    const vals = Object.values(o);
    return vals.length > 0 && vals.every(x => typeof x === "boolean") ? (o as Record<string, boolean>) : null;
  } catch {
    return null;
  }
}

function KeyEditor({ k, known, stored, generated, onChanged }: {
  k: string; known?: TitleDataKnown; stored?: { value: string; updated_at: string }; generated?: string;
  onChanged: () => void;
}) {
  const initial = stored?.value ?? generated ?? (known?.format === "json" ? known.template : "");
  const [draft, setDraft] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const format = known?.format ?? "text";
  const flags = format === "json" ? boolObject(draft) : null;
  const jsonErr = useMemo(() => {
    if (format !== "json" || draft === "") return "";
    try { JSON.parse(draft); return ""; } catch (e) { return (e as Error).message; }
  }, [draft, format]);
  const dirty = draft !== (stored?.value ?? generated ?? "");

  async function save() {
    setBusy(true); setMsg(null);
    try {
      await api.setTitleData(k, draft);
      setMsg({ ok: true, text: "Saved. Players see it on their next game start." });
      onChanged();
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message });
    } finally { setBusy(false); }
  }
  async function reset() {
    if (!confirm(generated !== undefined
      ? `Remove the stored value for ${k}? Players will get the main fleet's board again.`
      : `Remove ${k} from title data?`)) return;
    setBusy(true); setMsg(null);
    try {
      await api.deleteTitleData(k);
      onChanged();
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message });
    } finally { setBusy(false); }
  }

  const status = stored
    ? <span style={{ color: "var(--green)" }}>Set · {fmt(stored.updated_at)}</span>
    : generated !== undefined
      ? <span style={{ color: "var(--muted)" }}>From the main fleet&apos;s boards</span>
      : <span style={{ color: "#d4a017" }}>Not set: the game shows nothing</span>;

  return (
    <div className="rounded-lg p-4" style={card}>
      <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 mb-1">
        <span className="font-mono text-sm font-semibold" style={{ color: "var(--text)" }}>{k}</span>
        <span className="text-xs px-1.5 py-0.5 rounded" style={{ background: "var(--surface2)", color: "var(--muted)" }}>{format}</span>
        <span className="text-xs ml-auto">{status}</span>
      </div>
      {known && (
        <div className="text-xs mb-3" style={{ color: "var(--muted)" }}>
          {known.purpose}. Read by: {known.reader}
        </div>
      )}

      {flags ? (
        <div className="flex flex-wrap gap-2 mb-2">
          {Object.entries(flags).map(([name, on]) => (
            <label key={name} className="flex items-center gap-2 text-sm px-3 py-1.5 rounded-md cursor-pointer"
              style={{ background: "var(--surface2)", color: "var(--text)" }}>
              <input type="checkbox" checked={on}
                onChange={e => setDraft(JSON.stringify({ ...flags, [name]: e.target.checked }))} />
              {name}
            </label>
          ))}
        </div>
      ) : null}

      {format === "url" ? (
        <input className={inputCls} style={inputStyle} value={draft} placeholder="https://…" onChange={e => setDraft(e.target.value)} />
      ) : (
        <textarea className={inputCls} style={{ ...inputStyle, minHeight: format === "json" ? 70 : 90 }}
          value={draft} onChange={e => setDraft(e.target.value)} />
      )}
      {format === "url" && /^https?:\/\//.test(draft) && (
        // eslint-disable-next-line @next/next/no-img-element -- preview of an arbitrary image URL
        <img src={draft} alt="" className="mt-2 rounded" style={{ maxHeight: 90 }} />
      )}
      {jsonErr && <div className="text-xs mt-1" style={{ color: "var(--red)" }}>Invalid JSON: {jsonErr}</div>}

      <div className="flex items-center gap-2 mt-3">
        <button onClick={save} disabled={busy || !dirty || !!jsonErr}
          className="px-3 py-1.5 rounded-md text-sm font-medium disabled:opacity-40"
          style={{ background: "var(--accent)", color: "white" }}>
          {busy ? "Saving…" : "Save"}
        </button>
        {stored && (
          <button onClick={reset} disabled={busy} className="px-3 py-1.5 rounded-md text-sm"
            style={{ border: "1px solid var(--border)", color: "var(--muted)" }}>
            {generated !== undefined ? "Use main fleet's" : "Remove"}
          </button>
        )}
        {msg && <span className="text-xs" style={{ color: msg.ok ? "var(--green)" : "var(--red)" }}>{msg.text}</span>}
      </div>
    </div>
  );
}

export default function TitleDataPage() {
  const [admin, setAdmin] = useState<boolean | null>(null);
  const [data, setData] = useState<TitleDataList | null>(null);
  const [err, setErr] = useState("");
  const [newKey, setNewKey] = useState("");
  const [extraKeys, setExtraKeys] = useState<string[]>([]);
  const [rev, setRev] = useState(0);

  const load = useCallback(() => {
    api.titleData().then(d => { setData(d); setErr(""); setRev(r => r + 1); }).catch(e => setErr((e as Error).message));
  }, []);

  useEffect(() => {
    fetch("/me", { cache: "no-store" }).then(r => (r.ok ? r.json() : null)).then(m => {
      setAdmin(!!m?.admin);
      if (m?.admin) load();
    }).catch(() => setAdmin(false));
  }, [load]);

  if (admin === null) return <div className="text-sm" style={{ color: "var(--muted)" }}>Loading…</div>;
  if (!admin) return <div className="text-sm" style={{ color: "var(--muted)" }}>Title data is for Developers only.</div>;
  if (err) return <div className="text-sm" style={{ color: "var(--red)" }}>{err}</div>;
  if (!data) return <div className="text-sm" style={{ color: "var(--muted)" }}>Loading…</div>;

  const stored = Object.fromEntries(data.stored.map(r => [r.key, r]));
  const knownKeys = new Set(data.known.map(k => k.key));
  const otherStored = data.stored.map(r => r.key).filter(k => !knownKeys.has(k));
  const otherGenerated = Object.keys(data.generated).filter(k => !knownKeys.has(k) && !stored[k]);
  const extras = extraKeys.filter(k => !knownKeys.has(k) && !stored[k]);

  return (
    <div className="mx-auto max-w-[1100px] pt-8 pb-14">
      <p className="eyebrow mb-1.5">Admin</p>
      <h1 className="text-[44px] leading-tight font-medium tracking-tight mb-1">Title data</h1>
      <p className="text-xs mb-5" style={{ color: "var(--muted)" }}>
        Shared by every player and fleet. Players download it when their game starts, so changes show on their next
        launch. A saved value overrides the main fleet&apos;s board images.
      </p>

      <h2 className="text-sm font-semibold mb-2" style={{ color: "var(--text)" }}>Keys the game reads</h2>
      <div className="grid gap-3 mb-8">
        {data.known.map(k => (
          <KeyEditor key={`${k.key}:${rev}`} k={k.key} known={k} stored={stored[k.key]}
            generated={data.generated[k.key]} onChanged={load} />
        ))}
      </div>

      <h2 className="text-sm font-semibold mb-2" style={{ color: "var(--text)" }}>Other keys</h2>
      <p className="text-xs mb-3" style={{ color: "var(--muted)" }}>
        Keys the client is not known to read. Saved keys and new ones go here.
      </p>
      <div className="grid gap-3 mb-4">
        {[...otherStored, ...extras].map(k => (
          <KeyEditor key={`${k}:${rev}`} k={k} stored={stored[k]} generated={data.generated[k]} onChanged={load} />
        ))}
      </div>
      <form className="flex gap-2 mb-8" onSubmit={e => {
        e.preventDefault();
        const k = newKey.trim();
        if (/^[A-Za-z0-9_.-]{1,100}$/.test(k)) { setExtraKeys(x => [...x, k]); setNewKey(""); }
      }}>
        <input className={inputCls} style={{ ...inputStyle, maxWidth: 320 }} value={newKey}
          placeholder="new key name" onChange={e => setNewKey(e.target.value)} />
        <button type="submit" disabled={!/^[A-Za-z0-9_.-]{1,100}$/.test(newKey.trim())}
          className="px-3 py-1.5 rounded-md text-sm disabled:opacity-40"
          style={{ border: "1px solid var(--border)", color: "var(--text)" }}>Add key</button>
      </form>

      {otherGenerated.length > 0 && (
        <>
          <h2 className="text-sm font-semibold mb-1" style={{ color: "var(--text)" }}>Also sent from the main fleet&apos;s boards</h2>
          <p className="text-xs mb-2" style={{ color: "var(--muted)" }}>
            Nothing in the client reads these from title data (they reach each fleet&apos;s server instead): {otherGenerated.join(", ")}
          </p>
        </>
      )}
    </div>
  );
}
