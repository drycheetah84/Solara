"use client";

import { useEffect, useState, useCallback } from "react";
import { api, type WsClient, type WsMessage } from "../../lib/api";
import { EmptyRow } from "../../components/ui";
import { Pause, Play } from "lucide-react";

function ago(ts: string) {
  const diff = Date.now() - new Date(ts).getTime();
  if (diff < 1000) return "just now";
  if (diff < 60000) return `${Math.floor(diff / 1000)}s ago`;
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
  return new Date(ts).toLocaleTimeString();
}

function shortPath(path: string) {
  return path.length > 48 ? "…" + path.slice(-47) : path;
}

export default function WsPage() {
  const [clients, setClients]   = useState<WsClient[] | null>(null);
  const [err, setErr]           = useState<string | null>(null);
  const [selected, setSelected] = useState<WsClient | null>(null);
  const [messages, setMessages] = useState<WsMessage[] | null>(null);
  const [msgErr, setMsgErr]     = useState<string | null>(null);
  const [live, setLive]         = useState(true);

  const loadClients = useCallback(() => {
    api.wsClients()
      .then(list => {
        setClients(list);
        setErr(null);
      })
      .catch(e => setErr(e.message));
  }, []);

  useEffect(() => {
    loadClients();
    if (!live) return;
    const t = setInterval(loadClients, 2000);
    return () => clearInterval(t);
  }, [loadClients, live]);

  const loadMessages = useCallback((sessionId: string) => {
    api.wsMessages(sessionId)
      .then(msgs => { setMessages(msgs); setMsgErr(null); })
      .catch(e => setMsgErr(e.message));
  }, []);

  useEffect(() => {
    if (!selected) return;
    loadMessages(selected.session_id);
    if (!live) return;
    const t = setInterval(() => loadMessages(selected.session_id), 2000);
    return () => clearInterval(t);
  }, [selected, live, loadMessages]);

  const online = clients?.filter(c => !c.disconnected_at) ?? [];
  const offline = clients?.filter(c => c.disconnected_at) ?? [];
  const sorted = [...online, ...offline];

  return (
    <div className="mx-auto max-w-[1200px] pt-8 pb-14">
      <div className="flex flex-wrap items-end justify-between gap-4 mb-6">
        <div>
          <p className="eyebrow mb-1.5">Sockets</p>
          <h1 className="text-[44px] leading-tight font-medium tracking-tight">WS Monitor</h1>
          <p className="text-[15px] mt-1.5 tabular" style={{ color: "var(--muted)" }}>
            {clients ? <><span className="font-medium" style={{ color: "var(--green)" }}>{online.length} online</span>{" · "}{offline.length} offline</> : "Loading…"}
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
          <button onClick={loadClients} className="btn-dark !py-2 text-sm">Refresh</button>
        </div>
      </div>

      {err && <div className="card mb-4 px-5 py-3.5 text-[15px]" style={{ color: "var(--red)" }}>{err}</div>}

      <div className="grid gap-4 lg:grid-cols-[320px_minmax(0,1fr)] items-start">
        {/* Client list */}
        <div className="min-w-0">
          {!clients ? (
            <div className="card p-5"><div className="skeleton h-5 w-2/3" /></div>
          ) : sorted.length === 0 ? (
            <div className="card"><EmptyRow>No clients connected.</EmptyRow></div>
          ) : (
            <div className="space-y-2">
              {sorted.map(c => {
                const isOnline  = !c.disconnected_at;
                const isSelected = selected?.session_id === c.session_id;
                return (
                  <button
                    key={c.session_id}
                    onClick={() => setSelected(isSelected ? null : c)}
                    className="card card-hover w-full text-left p-4"
                    style={isSelected ? { boxShadow: "inset 0 0 0 1px var(--link)" } : undefined}
                  >
                    <div className="flex items-center gap-2 mb-1.5">
                      <span className={isOnline ? "live-dot" : "live-dot off"} style={isOnline ? undefined : { width: 6, height: 6 }} aria-hidden />
                      <span className="font-mono text-[13px] truncate tabular">{c.session_id}</span>
                      <span className="ml-auto text-[12px] tabular shrink-0" style={{ color: "var(--muted)" }}>
                        {c.message_count} msg
                      </span>
                    </div>
                    <div className="font-mono text-[12px] truncate" style={{ color: "var(--muted)" }} title={c.path}>
                      {shortPath(c.path)}
                    </div>
                    <div className="text-[12px] mt-1 tabular" style={{ color: "var(--muted)" }}>
                      {isOnline ? `connected ${ago(c.connected_at)}` : `gone ${ago(c.disconnected_at!)}`}
                    </div>
                  </button>
                );
              })}
            </div>
          )}
        </div>

        {/* Message log */}
        <div className="min-w-0">
          {!selected ? (
            <div className="card"><EmptyRow>Pick a client to read its messages.</EmptyRow></div>
          ) : (
            <div>
              <div className="flex items-center gap-3 mb-4">
                <div className="flex-1 min-w-0">
                  <span className="font-mono text-[13px] font-medium" style={{ color: "var(--link)" }}>
                    {selected.session_id}
                  </span>
                  <span className="ml-2 font-mono text-[12px] truncate" style={{ color: "var(--muted)" }} title={selected.path}>
                    {shortPath(selected.path)}
                  </span>
                </div>
                <button onClick={() => loadMessages(selected.session_id)} className="btn-dark !py-2 text-sm shrink-0">
                  Refresh
                </button>
              </div>

              {msgErr && <div className="card mb-4 px-5 py-3.5 text-[15px]" style={{ color: "var(--red)" }}>{msgErr}</div>}

              {!messages ? (
                <div className="card p-5"><div className="skeleton h-5 w-1/3" /></div>
              ) : messages.length === 0 ? (
                <div className="card"><EmptyRow>No messages on this connection yet.</EmptyRow></div>
              ) : (
                <div className="space-y-2">
                  {messages.map((m, i) => {
                    const isIn = m.direction === "in";
                    return (
                      <div key={i} className="card p-4">
                        <div className="flex items-center gap-2 mb-1.5 flex-wrap">
                          <span className="font-mono text-[12px] font-semibold px-1.5 py-0.5 rounded"
                            style={isIn
                              ? { background: "rgba(124,199,255,0.1)", color: "var(--link)" }
                              : { background: "rgba(139,212,80,0.1)", color: "var(--green)" }}>
                            {isIn ? "↓ IN" : "↑ OUT"}
                          </span>
                          <span className="font-mono text-[13px] font-medium truncate">{m.command}</span>
                          <span className="ml-auto text-[12px] tabular shrink-0" style={{ color: "var(--muted)" }}>
                            {ago(m.timestamp)}
                          </span>
                        </div>
                        {m.headers && (
                          <div className="font-mono text-[12px] mb-1 truncate" style={{ color: "var(--muted)" }}>
                            {m.headers}
                          </div>
                        )}
                        {m.body && (
                          <pre className="font-mono text-[12px] whitespace-pre-wrap break-all overflow-hidden"
                            style={{ color: "var(--text)", maxHeight: "6rem" }}>
                            {m.body}
                          </pre>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}

              <p className="mt-3 text-[13px] tabular flex items-center gap-2" style={{ color: "var(--muted)" }}>
                {live ? <Pause size={13} /> : <Play size={13} />}
                {messages?.length ?? 0} messages · {live ? "updating every 2 seconds" : "paused"}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
