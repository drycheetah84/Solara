"use client";

// Solara: station boards -- the in-game billboard / plaza sign images. Each slot (board key) holds an
// image URL in the station config; uploading stores the file on the backend and points the slot at it.
// Game servers read the station config once at startup, so a change shows in-game after the next restart.
import { useCallback, useEffect, useState } from "react";
import { api } from "../../../lib/api";

export default function BoardsTab({ stationId }: { stationId: string }) {
  const [keys, setKeys] = useState<string[] | null>(null);
  const [board, setBoard] = useState<Record<string, string | null>>({});
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);

  const load = useCallback(() => {
    Promise.all([api.boardKeys(), api.stationBoard(stationId)])
      .then(([k, b]) => { setKeys(k); setBoard(b); setErr(null); })
      .catch(e => setErr(e.message));
  }, [stationId]);

  useEffect(() => { load(); }, [load]);

  async function upload(key: string, file: File | undefined) {
    if (!file) return;
    if (file.size > 8 * 1024 * 1024) { setErr("image too large (max 8 MB)"); return; }
    setBusy(key);
    try {
      await api.uploadStationBoard(stationId, key, file);
      load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  async function clear(key: string) {
    if (!confirm(`Clear ${key}?`)) return;
    setBusy(key);
    try {
      await api.clearStationBoard(stationId, key);
      load();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(null);
    }
  }

  return (
    <div>
      <p className="text-xs mb-4" style={{ color: "var(--muted)" }}>
        In-game boards and signs for this station. PNG, JPG, GIF or WebP, up to 8 MB. Game servers read these when
        they start, so a new image shows up in-game after this fleet&apos;s server next restarts.
      </p>
      {err && (
        <div className="mb-4 px-4 py-3 rounded-md text-sm"
          style={{ background: "#2d1a1a", color: "var(--red)", border: "1px solid #5c2020" }}>
          {err}
        </div>
      )}
      {!keys ? (
        <div className="text-sm" style={{ color: "var(--muted)" }}>Loading…</div>
      ) : (
        <div className="grid gap-3" style={{ gridTemplateColumns: "repeat(auto-fill, minmax(240px, 1fr))" }}>
          {keys.map(k => {
            const url = board[k];
            return (
              <div key={k} className="rounded-lg p-3"
                style={{ background: "var(--surface)", border: "1px solid var(--border)" }}>
                <div className="font-mono text-xs mb-2">{k}</div>
                <div className="rounded-md mb-3 flex items-center justify-center overflow-hidden"
                  style={{ background: "var(--surface2)", height: 120 }}>
                  {url ? (
                    // eslint-disable-next-line @next/next/no-img-element -- arbitrary backend URL, no optimisation wanted
                    <img src={url} alt={k} style={{ maxHeight: 120, maxWidth: "100%", objectFit: "contain" }} />
                  ) : (
                    <span className="text-xs" style={{ color: "var(--muted)" }}>empty</span>
                  )}
                </div>
                <div className="flex gap-2">
                  <label className="px-2.5 py-1 text-xs rounded-md font-medium cursor-pointer"
                    style={{ background: "var(--accent)", color: "#fff", opacity: busy === k ? 0.5 : 1 }}>
                    {busy === k ? "Working…" : "Upload"}
                    <input type="file" accept="image/png,image/jpeg,image/gif,image/webp" hidden
                      disabled={busy === k}
                      onChange={e => { upload(k, e.target.files?.[0]); e.target.value = ""; }} />
                  </label>
                  {url && (
                    <button onClick={() => clear(k)} disabled={busy === k}
                      className="px-2.5 py-1 text-xs rounded-md"
                      style={{ background: "transparent", color: "var(--muted)", border: "1px solid var(--border)" }}>
                      Clear
                    </button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
