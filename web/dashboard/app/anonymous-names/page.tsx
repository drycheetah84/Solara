"use client";

// Anonymous-mode name generator (Developers only; the backend refuses every scoped token on /api/anonymous-names).
// A name is Adjective + Noun + an optional number, picked by the random seed the game sends each time a player turns
// anonymous mode on. Changes apply to the next name a player gets.
import { useCallback, useEffect, useState } from "react";
import { api, type AnonNamePreview, type AnonNameSettings, type AnonNameState } from "../../lib/api";
import { fmt } from "../../lib/utils";

const card: React.CSSProperties = { background: "var(--surface)", border: "1px solid var(--border)" };
const inputCls = "w-full rounded-md px-3 py-2 text-sm font-mono outline-none";
const inputStyle: React.CSSProperties = { background: "var(--bg)", border: "1px solid var(--border)", color: "var(--text)" };

/** One word per line (commas also split); blanks dropped. The backend validates and de-duplicates. */
const words = (s: string) => s.split(/[\n,]/).map(w => w.trim()).filter(Boolean);
const lines = (ws: string[]) => ws.join("\n");

export default function AnonymousNamesPage() {
  const [admin, setAdmin] = useState<boolean | null>(null);
  const [state, setState] = useState<AnonNameState | null>(null);
  const [adj, setAdj] = useState("");
  const [nouns, setNouns] = useState("");
  const [lo, setLo] = useState("10");
  const [hi, setHi] = useState("99");
  const [preview, setPreview] = useState<AnonNamePreview[]>([]);
  const [previewErr, setPreviewErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null);
  const [err, setErr] = useState("");

  const apply = useCallback((s: AnonNameState) => {
    setState(s);
    setAdj(lines(s.settings.adjectives));
    setNouns(lines(s.settings.nouns));
    setLo(String(s.settings.number_min));
    setHi(String(s.settings.number_max));
    setPreview(s.preview);
    setPreviewErr("");
  }, []);

  useEffect(() => {
    fetch("/me", { cache: "no-store" }).then(r => (r.ok ? r.json() : null)).then(m => {
      setAdmin(!!m?.admin);
      if (m?.admin) api.anonymousNames().then(apply).catch(e => setErr((e as Error).message));
    }).catch(() => setAdmin(false));
  }, [apply]);

  const draft = useCallback((): AnonNameSettings => ({
    adjectives: words(adj), nouns: words(nouns), number_min: Number(lo), number_max: Number(hi),
  }), [adj, nouns, lo, hi]);

  // live preview of the draft (debounced; nothing is saved)
  useEffect(() => {
    if (!state) return;
    const t = setTimeout(() => {
      api.previewAnonymousNames(draft())
        .then(r => { setPreview(r.preview); setPreviewErr(""); })
        .catch(e => setPreviewErr((e as Error).message));
    }, 350);
    return () => clearTimeout(t);
  }, [state, draft]);

  const saved = state?.settings;
  const dirty = !!saved && (
    lines(words(adj)) !== lines(saved.adjectives) || lines(words(nouns)) !== lines(saved.nouns) ||
    Number(lo) !== saved.number_min || Number(hi) !== saved.number_max);

  async function save() {
    setBusy(true); setMsg(null);
    try {
      apply(await api.saveAnonymousNames(draft()));
      setMsg({ ok: true, text: "Saved. Players get the new style the next time they turn anonymous mode on." });
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message });
    } finally { setBusy(false); }
  }
  async function reset() {
    if (!confirm("Go back to the built-in word lists and number range?")) return;
    setBusy(true); setMsg(null);
    try {
      apply(await api.resetAnonymousNames());
      setMsg({ ok: true, text: "Back to the defaults." });
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message });
    } finally { setBusy(false); }
  }

  if (admin === null) return <div className="text-sm" style={{ color: "var(--muted)" }}>Loading…</div>;
  if (!admin) return <div className="text-sm" style={{ color: "var(--muted)" }}>Anonymous names are for Developers only.</div>;
  if (err) return <div className="text-sm" style={{ color: "var(--red)" }}>{err}</div>;
  if (!state) return <div className="text-sm" style={{ color: "var(--muted)" }}>Loading…</div>;

  const nAdj = words(adj).length, nNoun = words(nouns).length;
  return (
    <div className="mx-auto max-w-[1100px] pt-8 pb-14">
      <p className="eyebrow mb-1.5">Admin</p>
      <h1 className="text-[44px] leading-tight font-medium tracking-tight mb-1">Anonymous names</h1>
      <p className="text-xs mb-5" style={{ color: "var(--muted)" }}>
        When a player turns anonymous mode on, their name tag becomes <b>Adjective + Noun + number</b>, picked from
        these lists by a random seed the game sends. Words: letters and digits only, up to 24 characters, one per line.
        {" "}{state.is_default
          ? <span style={{ color: "var(--muted)" }}>Using the built-in defaults.</span>
          : <span style={{ color: "var(--green)" }}>Custom · saved {fmt(state.updated_at)}</span>}
      </p>

      <div className="grid gap-3 md:grid-cols-2 mb-3">
        <div className="rounded-lg p-4" style={card}>
          <div className="text-sm font-semibold mb-2" style={{ color: "var(--text)" }}>Adjectives <span className="text-xs font-normal" style={{ color: "var(--muted)" }}>({nAdj})</span></div>
          <textarea className={inputCls} style={{ ...inputStyle, minHeight: 260 }} value={adj} onChange={e => setAdj(e.target.value)} />
        </div>
        <div className="rounded-lg p-4" style={card}>
          <div className="text-sm font-semibold mb-2" style={{ color: "var(--text)" }}>Nouns <span className="text-xs font-normal" style={{ color: "var(--muted)" }}>({nNoun})</span></div>
          <textarea className={inputCls} style={{ ...inputStyle, minHeight: 260 }} value={nouns} onChange={e => setNouns(e.target.value)} />
        </div>
      </div>

      <div className="rounded-lg p-4 mb-3" style={card}>
        <div className="text-sm font-semibold mb-2" style={{ color: "var(--text)" }}>Number</div>
        <div className="flex flex-wrap items-center gap-2 text-sm" style={{ color: "var(--muted)" }}>
          from <input type="number" min={0} max={99999} className={inputCls} style={{ ...inputStyle, width: 110 }} value={lo} onChange={e => setLo(e.target.value)} />
          to <input type="number" min={0} max={99999} className={inputCls} style={{ ...inputStyle, width: 110 }} value={hi} onChange={e => setHi(e.target.value)} />
          <span className="text-xs">(both 0 = no number)</span>
        </div>
      </div>

      <div className="rounded-lg p-4 mb-4" style={card}>
        <div className="text-sm font-semibold mb-2" style={{ color: "var(--text)" }}>Preview {dirty && <span className="text-xs font-normal" style={{ color: "#d4a017" }}>(unsaved changes)</span>}</div>
        {previewErr
          ? <div className="text-xs" style={{ color: "var(--red)" }}>{previewErr}</div>
          : <div className="flex flex-wrap gap-2">
              {preview.map(p => (
                <span key={p.seed} className="text-sm font-mono px-2 py-1 rounded" style={{ background: "var(--surface2)", color: "var(--text)" }}>{p.name}</span>
              ))}
            </div>}
      </div>

      <div className="flex items-center gap-2">
        <button onClick={save} disabled={busy || !dirty || !!previewErr}
          className="px-3 py-1.5 rounded-md text-sm font-medium disabled:opacity-40"
          style={{ background: "var(--accent)", color: "white" }}>
          {busy ? "Saving…" : "Save"}
        </button>
        {!state.is_default && (
          <button onClick={reset} disabled={busy} className="px-3 py-1.5 rounded-md text-sm"
            style={{ border: "1px solid var(--border)", color: "var(--muted)" }}>Reset to defaults</button>
        )}
        {msg && <span className="text-xs" style={{ color: msg.ok ? "var(--green)" : "var(--red)" }}>{msg.text}</span>}
      </div>
    </div>
  );
}
