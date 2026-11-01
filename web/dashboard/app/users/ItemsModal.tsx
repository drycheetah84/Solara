"use client";

import { useEffect, useMemo, useState } from "react";
import { X } from "lucide-react";
import { api, type CatalogItem, type UserInventory } from "../../lib/api";

const inputCls = "w-full px-3 py-2 rounded-md text-sm outline-none";
const inputStyle = { background: "var(--surface2)", border: "1px solid var(--border)", color: "var(--text)" };

// Give / take cosmetics and currency for one player. Changes reach the player the next time their game starts
// (the game reads its inventory at login).
export default function ItemsModal({ userId, username, onClose }: { userId: string; username: string; onClose: () => void }) {
  const [inv, setInv] = useState<UserInventory | null>(null);
  const [catalog, setCatalog] = useState<CatalogItem[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<CatalogItem | null>(null);
  const [qty, setQty] = useState("1000");
  const [busy, setBusy] = useState(false);

  function applyInventory(r: UserInventory | { success: boolean; error?: string }) {
    if ("success" in r && !r.success) { setErr(r.error ?? "Could not load inventory"); return; }
    setInv(r as UserInventory);
    setErr(null);
  }
  async function load() {
    try { applyInventory(await api.userInventory(userId)); }
    catch (e: unknown) { setErr(e instanceof Error ? e.message : "Could not load inventory"); }
  }

  useEffect(() => {
    let live = true;
    api.userInventory(userId)
      .then(r => { if (live) applyInventory(r); })
      .catch((e: unknown) => { if (live) setErr(e instanceof Error ? e.message : "Could not load inventory"); });
    api.catalogItems().then(c => { if (live) setCatalog(c); }).catch(() => {});
    return () => { live = false; };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [userId]);

  const matches = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q ? catalog.filter(i => i.id.toLowerCase().includes(q)) : catalog;
    return list.slice(0, 60);
  }, [catalog, query]);

  async function give(item: CatalogItem, quantity: number) {
    setBusy(true); setNote(null);
    try {
      const r = await api.grantItem(userId, item.id, quantity);
      if (!r.success) setErr(r.error ?? "Grant failed");
      else { setNote(item.currency ? `Set ${item.id} to ${quantity.toLocaleString()}` : `Gave ${item.id}`); setErr(null); }
      await load();
    } catch (e: unknown) { setErr(e instanceof Error ? e.message : "Grant failed"); }
    finally { setBusy(false); }
  }

  async function take(id: string) {
    if (!confirm(`Remove ${id} from ${username}?`)) return;
    setBusy(true); setNote(null);
    try {
      const r = await api.revokeItem(userId, id);
      if (!r.success) setErr(r.error ?? "Remove failed");
      else { setNote(`Removed ${id}`); setErr(null); }
      await load();
    } catch (e: unknown) { setErr(e instanceof Error ? e.message : "Remove failed"); }
    finally { setBusy(false); }
  }

  const qtyNum = Math.max(0, Math.min(2000000000, parseInt(qty, 10) || 0));

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center px-4"
      style={{ background: "rgba(0,0,0,0.6)" }}
      onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="w-full max-w-2xl rounded-lg p-6 max-h-[90vh] overflow-y-auto"
        style={{ background: "var(--surface)", border: "1px solid var(--border)" }}>
        <div className="flex items-center justify-between mb-1">
          <h2 className="font-semibold text-sm">Items for {username}</h2>
          <button onClick={onClose} style={{ color: "var(--muted)" }} aria-label="Close"><X size={16} /></button>
        </div>
        <p className="text-xs mb-4" style={{ color: "var(--muted)" }}>
          Changes show up the next time this player starts the game.
          {inv?.full_catalogue && " This account already has every cosmetic; granting here only matters for currency balances."}
        </p>

        {err && <div className="mb-3 px-3 py-2 rounded text-xs" style={{ background: "#2d1a1a", color: "var(--red)" }}>{err}</div>}
        {note && <div className="mb-3 px-3 py-2 rounded text-xs" style={{ background: "#1a2d1a", color: "var(--green)" }}>{note}</div>}

        {/* Give */}
        <div className="mb-5">
          <label className="block text-xs mb-1.5" style={{ color: "var(--muted)" }}>Give an item ({catalog.length} in the catalogue)</label>
          <input className={inputCls} style={inputStyle} placeholder="Search, e.g. Hat, Chassis, Currency…"
            value={query} onChange={e => { setQuery(e.target.value); setPicked(null); }} />
          <div className="mt-2 rounded-md overflow-y-auto" style={{ maxHeight: 180, border: "1px solid var(--border)" }}>
            {matches.map(i => (
              <button key={i.id} onClick={() => setPicked(i)}
                className="w-full text-left px-3 py-1.5 text-xs flex justify-between"
                style={{ background: picked?.id === i.id ? "var(--surface2)" : "transparent", color: "var(--text)" }}>
                <span className="font-mono">{i.id}</span>
                {i.currency && <span style={{ color: "#f0883e" }}>currency</span>}
              </button>
            ))}
            {matches.length === 0 && <div className="px-3 py-2 text-xs" style={{ color: "var(--muted)" }}>No match</div>}
          </div>
          {picked && (
            <div className="mt-3 flex gap-2 items-end">
              {picked.currency && (
                <div className="flex-1">
                  <label className="block text-xs mb-1.5" style={{ color: "var(--muted)" }}>Balance (replaces the current one)</label>
                  <input className={inputCls} style={inputStyle} inputMode="numeric" value={qty} onChange={e => setQty(e.target.value)} />
                </div>
              )}
              <button disabled={busy} onClick={() => give(picked, picked.currency ? qtyNum : 1)}
                className="px-4 py-2 rounded-md text-sm font-medium disabled:opacity-50"
                style={{ background: "var(--accent, #2f81f7)", color: "#fff" }}>
                {picked.currency ? "Set balance" : `Give ${picked.id}`}
              </button>
            </div>
          )}
        </div>

        {/* Owned */}
        <div>
          <div className="text-xs mb-1.5" style={{ color: "var(--muted)" }}>Given items ({inv?.items.length ?? 0})</div>
          <div className="rounded-md overflow-hidden" style={{ border: "1px solid var(--border)" }}>
            {!inv ? (
              <div className="px-3 py-3 text-xs" style={{ color: "var(--muted)" }}>{err ? "—" : "Loading…"}</div>
            ) : inv.items.length === 0 ? (
              <div className="px-3 py-3 text-xs" style={{ color: "var(--muted)" }}>Nothing given yet</div>
            ) : inv.items.map((it, i) => (
              <div key={it.id} className="px-3 py-2 text-xs flex items-center justify-between"
                style={{ background: i % 2 ? "var(--surface2)" : "transparent" }}>
                <span className="font-mono">{it.id}{it.currency && <span style={{ color: "#f0883e" }}> · {it.quantity.toLocaleString()}</span>}</span>
                <button disabled={busy} onClick={() => take(it.id)} className="px-2 py-0.5 rounded disabled:opacity-50"
                  style={{ color: "var(--red)", border: "1px solid var(--border)" }}>Remove</button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
