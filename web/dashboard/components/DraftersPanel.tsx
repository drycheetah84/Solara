"use client";
import { useCallback, useEffect, useState } from "react";

type Account = { user_id: string; username: string };
type Grant = { user_id: string; scope: string; granted_at: string };
type Data = { accounts: Account[]; grants: Grant[] };

export default function DraftersPanel({ scope = "global" }: { scope?: string }) {
  const [data, setData] = useState<Data | null>(null);
  const [userId, setUserId] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const endpoint = `/api/drafters/${encodeURIComponent(scope)}`;
  const load = useCallback(async () => {
    const response = await fetch(endpoint, { cache: "no-store" });
    const body = await response.json();
    if (!response.ok) throw new Error(body.error || "Could not load Drafters");
    return body as Data;
  }, [endpoint]);
  useEffect(() => {
    let active = true;
    setData(null); setError(""); setUserId("");
    load().then(value => { if (active) setData(value); }).catch(e => { if (active) setError(e.message); });
    return () => { active = false; };
  }, [load]);
  async function change(id: string, grant: boolean) {
    setBusy(true); setError("");
    try {
      const response = await fetch(`${endpoint}/${encodeURIComponent(id)}`, { method: grant ? "POST" : "DELETE" });
      const result = await response.json();
      if (!response.ok || !result.success) throw new Error(result.error || "Could not update Drafter role");
      setData(await load()); setUserId("");
    } catch (e) { setError(e instanceof Error ? e.message : "Request failed"); }
    finally { setBusy(false); }
  }
  const names = new Map(data?.accounts.map(a => [a.user_id, a.username]));
  const available = data?.accounts.filter(a => !data.grants.some(g => g.user_id === a.user_id && g.scope === scope));
  return <section className="rounded-lg p-5 space-y-4" style={{ background: "var(--surface)", border: "1px solid var(--border)" }}>
    <h2 className="font-semibold">{scope === "global" ? "Global Drafters" : "Station Drafters"}</h2>
    <p className="text-sm" style={{ color: "var(--muted)" }}>
      {scope === "global" ? "Grant Level Editor access across every station." : "Grant building/editing access to this station. Global Drafters also have access here. Shared library save/load/unload requires a global grant; local import/export remains available."}
      {" "}Changes normally reach running servers within 10 seconds. The server must run with editing enabled.
    </p>
    {error && <p role="alert" className="text-sm text-red-400">{error}</p>}
    {!data ? <p>{error ? "Drafters could not be loaded." : "Loading Drafters…"}</p> : <>
      <form className="flex gap-3" onSubmit={e => { e.preventDefault(); if (userId) void change(userId, true); }}>
        <select aria-label="User to grant Drafter role" value={userId} onChange={e => setUserId(e.target.value)} disabled={busy}
          className="rounded px-3 py-2 flex-1" style={{ background: "var(--surface2)", color: "var(--text)" }}>
          <option value="">Select a user…</option>
          {available?.map(a => <option key={a.user_id} value={a.user_id}>{a.username} ({a.user_id})</option>)}
        </select>
        <button disabled={busy || !userId} className="rounded border px-3 py-2 disabled:opacity-50">Grant Drafter</button>
      </form>
      {data.grants.length === 0 && <p className="text-sm">No Drafters granted for this scope.</p>}
      <ul className="space-y-2">{data.grants.map(g => <li key={`${g.scope}:${g.user_id}`} className="flex items-center justify-between gap-3 border-t pt-3" style={{ borderColor: "var(--border)" }}>
        <div><span>{names.get(g.user_id) || g.user_id}</span><span className="text-xs ml-3" style={{ color: "var(--muted)" }}>{g.user_id} · {g.scope === "global" ? "Global Drafter" : "Station Drafter"}</span></div>
        {g.scope === scope ? <button disabled={busy} onClick={() => void change(g.user_id, false)} className="text-sm text-red-400 disabled:opacity-50">Revoke</button>
          : <a href="/drafters" className="text-sm underline">Manage global grant</a>}
      </li>)}</ul>
      {scope !== "global" && <p className="text-xs" style={{ color: "var(--muted)" }}>Revoking a station grant does not remove access provided by a global grant.</p>}
    </>}
  </section>;
}
