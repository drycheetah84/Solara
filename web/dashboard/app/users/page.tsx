"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type User, type Station, type SearchUser, type UserSearchPage } from "../../lib/api";
import { fmt, ago } from "../../lib/utils";
import { X } from "lucide-react";
import GameBans from "./GameBans";
import ItemsModal from "./ItemsModal";

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ background: "rgba(0,0,0,0.6)" }}
      onClick={e => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="w-full max-w-md rounded-lg p-6"
        style={{ background: "var(--surface)", border: "1px solid var(--border)" }}>
        <div className="flex items-center justify-between mb-5">
          <h2 className="font-semibold text-sm">{title}</h2>
          <button onClick={onClose} style={{ color: "var(--muted)" }}><X size={16} /></button>
        </div>
        {children}
      </div>
    </div>
  );
}

const inputCls = "w-full px-3 py-2 rounded-md text-sm outline-none";
const inputStyle = { background: "var(--surface2)", border: "1px solid var(--border)", color: "var(--text)" };

const PAGE_SIZES = [25, 50, 100, 200];

export default function UsersPage() {
  const [users, setUsers]       = useState<SearchUser[] | null>(null);
  const [pageInfo, setPageInfo] = useState<UserSearchPage | null>(null);
  const [stations, setStations] = useState<Station[]>([]);
  const [err, setErr]           = useState<string | null>(null);

  // Paging + search (server-side over EVERY user; GET /api/users alone stops at the 50 newest)
  const [query, setQuery]       = useState("");
  const [q, setQ]               = useState("");   // debounced query actually sent
  const [page, setPage]         = useState(1);
  const [pageSize, setPageSize] = useState(50);
  const [busyId, setBusyId]     = useState<string | null>(null);

  // Ban modal state
  const [banTarget, setBanTarget]   = useState<User | null>(null);
  const [banStation, setBanStation] = useState("");
  const [banReason, setBanReason]   = useState("");
  const [banDuration, setBanDuration] = useState("0");
  const [saving, setSaving]         = useState(false);
  const [actionErr, setActionErr]   = useState<string | null>(null);
  const [itemsFor, setItemsFor] = useState<User | null>(null);

  const load = useCallback(() => {
    api.searchUsers(q, page, pageSize)
      .then(p => { setUsers(p.users); setPageInfo(p); if (p.page !== page) setPage(p.page); setErr(null); })
      .catch(e => setErr(e.message));
  }, [q, page, pageSize]);

  useEffect(() => { api.stations().then(setStations).catch(e => setErr(e.message)); }, []);
  useEffect(() => { load(); }, [load]);
  // typing waits 300 ms, then searches from page 1
  useEffect(() => {
    const t = setTimeout(() => { setQ(query.trim()); setPage(1); }, 300);
    return () => clearTimeout(t);
  }, [query]);

  // Global permissions modal (a per-user role on station "global" = applies on every server)
  const [gpUser, setGpUser]     = useState<SearchUser | null>(null);
  const [gpText, setGpText]     = useState("");
  const [gpCurrent, setGpCurrent] = useState<string[]>([]);
  const [gpKnown, setGpKnown]   = useState<string[]>([]);
  const [gpErr, setGpErr]       = useState<string | null>(null);

  async function openGlobalPerms(u: SearchUser) {
    setGpUser(u); setGpErr(null); setGpText(""); setGpCurrent([]);
    try {
      const g = await api.userGlobalPerms(u.user_id);
      setGpKnown(g.known); setGpCurrent(g.permissions); setGpText(g.permissions.join(", "));
    } catch (e: unknown) {
      setGpErr(e instanceof Error ? e.message : "Unknown error");
    }
  }
  const gpList = gpText.split(",").map(s => s.trim()).filter(Boolean);
  function gpToggle(p: string) {
    setGpText((gpList.includes(p) ? gpList.filter(x => x !== p) : [...gpList, p]).join(", "));
  }
  async function saveGlobalPerms(perms: string[]) {
    if (!gpUser) return;
    setBusyId(gpUser.user_id); setGpErr(null);
    try {
      await api.setUserGlobalPerms(gpUser.user_id, perms);
      setGpUser(null);
      load();
    } catch (e: unknown) {
      setGpErr(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setBusyId(null);
    }
  }

  function openBan(user: User) {
    setBanTarget(user);
    setBanStation(stations[0]?.station_id ?? "");
    setBanReason("");
    setBanDuration("0");
    setActionErr(null);
  }

  async function submitBan() {
    if (!banTarget || !banStation) return;
    setSaving(true);
    setActionErr(null);
    try {
      const res = await api.banUser(banTarget.user_id, banStation, banReason, parseInt(banDuration, 10));
      if (!res.success) { setActionErr(res.error ?? "Ban failed"); return; }
      setBanTarget(null);
      // Refresh users
      load();
    } catch (e: unknown) {
      setActionErr(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setSaving(false);
    }
  }

  async function unban(user: User) {
    const stationId = prompt("Station ID to unban from:");
    if (!stationId) return;
    await api.unbanUser(user.user_id, stationId);
    load();
  }

  async function toggleAdmin(user: User) {
    if (user.is_admin) {
      await api.revokeAdmin(user.user_id);
    } else {
      await api.grantAdmin(user.user_id);
    }
    load();
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <h1 className="text-lg font-semibold" style={{ color: "var(--text)" }}>Users</h1>
        {pageInfo && (
          <span className="text-xs px-2 py-1 rounded-full"
            style={{ background: "var(--surface2)", color: "var(--muted)" }}>
            {pageInfo.total} {q ? "matching" : "total"}
          </span>
        )}
      </div>

      {err && (
        <div className="mb-4 px-4 py-3 rounded-md text-sm"
          style={{ background: "#2d1a1a", color: "var(--red)", border: "1px solid #5c2020" }}>
          {err}
        </div>
      )}

      <GameBans />

      {/* Search + pager */}
      <div className="flex flex-wrap items-center gap-2 mb-3">
        <input className={inputCls} style={{ ...inputStyle, maxWidth: 320 }} value={query}
          placeholder="Search by username or user ID…" onChange={e => setQuery(e.target.value)} />
        <div className="flex items-center gap-2 ml-auto text-xs" style={{ color: "var(--muted)" }}>
          <select className="px-2 py-1.5 rounded-md text-xs outline-none" style={inputStyle} value={pageSize}
            onChange={e => { setPageSize(Number(e.target.value)); setPage(1); }}>
            {PAGE_SIZES.map(n => <option key={n} value={n}>{n} per page</option>)}
          </select>
          <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={!pageInfo || page <= 1}
            className="px-2.5 py-1.5 rounded-md disabled:opacity-40" style={{ border: "1px solid var(--border)", color: "var(--text)" }}>
            ← Prev
          </button>
          <span>Page {pageInfo?.page ?? page} of {pageInfo?.pages ?? 1}</span>
          <button onClick={() => setPage(p => p + 1)} disabled={!pageInfo || page >= pageInfo.pages}
            className="px-2.5 py-1.5 rounded-md disabled:opacity-40" style={{ border: "1px solid var(--border)", color: "var(--text)" }}>
            Next →
          </button>
        </div>
      </div>

      <div className="rounded-lg overflow-hidden" style={{ border: "1px solid var(--border)" }}>
        <table className="w-full text-sm">
          <thead>
            <tr style={{ background: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {["User ID", "Username", "Platform", "Roles", "Last Login", "Created", "Status", ""].map(h => (
                <th key={h} className="px-4 py-3 text-left font-medium text-xs"
                  style={{ color: "var(--muted)" }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {!users ? (
              <tr>
                <td colSpan={8} className="px-4 py-8 text-center text-sm"
                  style={{ color: "var(--muted)" }}>Loading…</td>
              </tr>
            ) : users.length === 0 ? (
              <tr>
                <td colSpan={8} className="px-4 py-8 text-center text-sm"
                  style={{ color: "var(--muted)" }}>{q ? `No users match “${q}”` : "No users"}</td>
              </tr>
            ) : users.map((u, i) => (
              <tr key={u.user_id || `row-${i}`}
                style={{
                  background: i % 2 === 0 ? "var(--surface)" : "var(--surface2)",
                  borderBottom: "1px solid var(--border)",
                }}>
                {/* Guarded even though /api/users now filters id-less rows: this table renders whatever
                    the backend sends, and one null here previously threw inside render and blanked the
                    entire tab rather than the single bad row. */}
                <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--muted)" }}>
                  {u.user_id ? `${u.user_id.slice(0, 8)}…` : "—"}
                </td>
                <td className="px-4 py-3 font-medium">
                {u.username || "(unknown)"}
                {u.is_admin && (
                  <span className="ml-2 text-xs px-1.5 py-0.5 rounded-full font-medium"
                    style={{ background: "#2d1f0e", color: "#f0883e" }}>Admin</span>
                )}
              </td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{u.platform ?? "—"}</td>
                <td className="px-4 py-3" style={{ color: "var(--muted)" }}>{u.role_count}</td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{ago(u.last_login)}</td>
                <td className="px-4 py-3 text-xs" style={{ color: "var(--muted)" }}>{fmt(u.created_at)}</td>
                <td className="px-4 py-3">
                  {u.banned ? (
                    <span className="text-xs px-2 py-0.5 rounded-full font-medium"
                      style={{ background: "#2d1a1a", color: "var(--red)" }}>Banned</span>
                  ) : (
                    <span className="text-xs px-2 py-0.5 rounded-full font-medium"
                      style={{ background: "#1a2d1a", color: "var(--green)" }}>Active</span>
                  )}
                </td>
                <td className="px-4 py-3">
                  <div className="flex gap-2">
                    {u.banned ? (
                      <button onClick={() => unban(u)}
                        className="text-xs px-2 py-1 rounded"
                        style={{ color: "var(--green)", border: "1px solid var(--border)" }}>
                        Unban
                      </button>
                    ) : (
                      <button onClick={() => openBan(u)}
                        className="text-xs px-2 py-1 rounded"
                        style={{ color: "var(--red)", border: "1px solid var(--border)" }}>
                        Ban
                      </button>
                    )}
                    <button onClick={() => setItemsFor(u)}
                      className="text-xs px-2 py-1 rounded"
                      style={{ color: "var(--text)", border: "1px solid var(--border)" }}>
                      Items
                    </button>
                    <button onClick={() => toggleAdmin(u)}
                      className="text-xs px-2 py-1 rounded"
                      style={{ color: u.is_admin ? "#f0883e" : "var(--muted)", border: "1px solid var(--border)" }}>
                      {u.is_admin ? "Revoke Admin" : "Grant Admin"}
                    </button>
                    <button onClick={() => openGlobalPerms(u)} disabled={busyId === u.user_id}
                      title="Permissions this user has on every server"
                      className="text-xs px-2 py-1 rounded whitespace-nowrap disabled:opacity-40"
                      style={{ color: u.global_permissions > 0 ? "#f0883e" : "var(--muted)", border: "1px solid var(--border)" }}>
                      {busyId === u.user_id ? "…" : `Global perms${u.global_permissions > 0 ? ` (${u.global_permissions})` : ""}`}
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {itemsFor && (
        <ItemsModal userId={itemsFor.user_id} username={itemsFor.username || itemsFor.user_id} onClose={() => setItemsFor(null)} />
      )}

      {/* Global permissions modal */}
      {gpUser && (
        <Modal title={`Global permissions — ${gpUser.username}`} onClose={() => setGpUser(null)}>
          {gpErr && (
            <div className="mb-4 px-3 py-2 rounded text-xs"
              style={{ background: "#2d1a1a", color: "var(--red)" }}>{gpErr}</div>
          )}
          <p className="text-xs mb-3" style={{ color: "var(--muted)" }}>
            These apply on <b>every server</b> (a role on station &quot;global&quot;), on top of the user&apos;s
            station roles. Players see changes the next time they log in.
          </p>
          <label className="block text-xs mb-1.5" style={{ color: "var(--muted)" }}>Permissions (comma-separated)</label>
          <textarea className={inputCls} style={{ ...inputStyle, resize: "vertical", minHeight: 80 }}
            value={gpText} onChange={e => setGpText(e.target.value)} placeholder="user_kick, user_mute, level_editor" />
          <div className="text-xs mt-1 mb-3" style={{ color: "var(--muted)" }}>
            Current: {gpCurrent.join(", ") || "none"}
          </div>
          {gpKnown.length > 0 && (
            <div className="mb-4">
              <div className="text-xs mb-1.5" style={{ color: "var(--muted)" }}>Click to add / remove:</div>
              <div className="flex flex-wrap gap-1 max-h-40 overflow-y-auto">
                {gpKnown.map(p => (
                  <button key={p} type="button" onClick={() => gpToggle(p)}
                    className="text-xs px-1.5 py-0.5 rounded font-mono"
                    style={{
                      border: "1px solid var(--border)",
                      background: gpList.includes(p) ? "var(--surface2)" : "transparent",
                      color: gpList.includes(p) ? "#f0883e" : "var(--muted)",
                    }}>{p}</button>
                ))}
              </div>
            </div>
          )}
          <div className="flex gap-2 justify-end">
            {gpCurrent.length > 0 && (
              <button onClick={() => { if (confirm(`Remove all of ${gpUser.username}'s global permissions?`)) saveGlobalPerms([]); }}
                disabled={busyId === gpUser.user_id}
                className="px-4 py-2 rounded-md text-xs mr-auto"
                style={{ color: "var(--red)", border: "1px solid var(--border)" }}>Clear all</button>
            )}
            <button onClick={() => setGpUser(null)}
              className="px-4 py-2 rounded-md text-xs"
              style={{ background: "transparent", color: "var(--muted)", border: "1px solid var(--border)" }}>
              Cancel
            </button>
            <button onClick={() => saveGlobalPerms(gpList)} disabled={busyId === gpUser.user_id}
              className="px-4 py-2 rounded-md text-xs font-medium"
              style={{ background: "var(--accent)", color: "#fff", opacity: busyId === gpUser.user_id ? 0.5 : 1 }}>
              {busyId === gpUser.user_id ? "Saving…" : "Save"}
            </button>
          </div>
        </Modal>
      )}

      {/* Ban modal */}
      {banTarget && (
        <Modal title={`Ban ${banTarget.username}`} onClose={() => setBanTarget(null)}>
          {actionErr && (
            <div className="mb-4 px-3 py-2 rounded text-xs"
              style={{ background: "#2d1a1a", color: "var(--red)" }}>{actionErr}</div>
          )}
          <div className="mb-4">
            <label className="block text-xs mb-1.5" style={{ color: "var(--muted)" }}>Station</label>
            <select className={inputCls} style={inputStyle} value={banStation} onChange={e => setBanStation(e.target.value)}>
              {stations.map(s => <option key={s.station_id} value={s.station_id}>{s.station_name}</option>)}
            </select>
          </div>
          <div className="mb-4">
            <label className="block text-xs mb-1.5" style={{ color: "var(--muted)" }}>Reason</label>
            <input className={inputCls} style={inputStyle} value={banReason}
              onChange={e => setBanReason(e.target.value)} placeholder="Optional reason" />
          </div>
          <div className="mb-5">
            <label className="block text-xs mb-1.5" style={{ color: "var(--muted)" }}>
              Duration (seconds, 0 = permanent)
            </label>
            <input className={inputCls} style={inputStyle} type="number" min="0"
              value={banDuration} onChange={e => setBanDuration(e.target.value)} />
          </div>
          <div className="flex gap-2 justify-end">
            <button onClick={() => setBanTarget(null)}
              className="px-4 py-2 rounded-md text-xs"
              style={{ background: "transparent", color: "var(--muted)", border: "1px solid var(--border)" }}>
              Cancel
            </button>
            <button onClick={submitBan} disabled={saving || !banStation}
              className="px-4 py-2 rounded-md text-xs font-medium"
              style={{ background: "var(--red)", color: "#fff", opacity: saving ? 0.5 : 1 }}>
              {saving ? "Banning…" : "Ban User"}
            </button>
          </div>
        </Modal>
      )}
    </div>
  );
}
