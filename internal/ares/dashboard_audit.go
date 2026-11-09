// Solara: dashboard audit log. Every mutating action a signed-in admin performs through the web
// dashboard is proxied through the Next.js server (web/dashboard/lib/backend-proxy.ts), which knows
// the Discord identity behind the session. After a successful mutation the proxy POSTs one entry
// here (/api/audit, admin-key authenticated); the audit page reads them back (GET /api/audit).
//
// The entries are durable (docdb collection), so they survive dashboard rebuilds and backend
// restarts. Only the backend ever assigns the timestamp.
package ares

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/stj"
)

// DashboardAuditEntry is one recorded admin action. Stored in collection "DashboardAuditEntry".
type DashboardAuditEntry struct {
	docdb.AstraDbObject
	TsMs      int64  `json:"ts_ms"`      // unix millis, assigned by the backend; used for ordering
	Ts        string `json:"ts"`         // RFC3339 UTC, for display
	ActorId   string `json:"actor_id"`   // Discord id of the admin (or scoped-token id)
	ActorName string `json:"actor_name"` // Discord username at the time of the action
	Scoped    bool   `json:"scoped"`     // true when acting through a scoped dashboard token
	Method    string `json:"method"`     // POST / PUT / PATCH / DELETE
	Service   string `json:"service"`    // "dashboard" (:8080) or "station" (:78)
	Path      string `json:"path"`       // request path (+ query) of the action
	Status    int    `json:"status"`     // upstream HTTP status
	Detail    string `json:"detail"`     // short, truncated request body for context (ban reason, etc.)
	IP        string `json:"ip"`         // caller IP as the backend saw it
}

func NewDashboardAuditEntry() *DashboardAuditEntry { return &DashboardAuditEntry{} }

func dashAudit() *docdb.Collection[DashboardAuditEntry] {
	return docdb.GetCollection[DashboardAuditEntry](Database, true, NewDashboardAuditEntry)
}

const dashAuditDetailMax = 500

// PostAudit ingests one audit entry from the dashboard proxy. Admin-key only: the proxy always
// calls it with the dashboard admin key, and the actor identity in the body comes from the
// server-verified session cookie, never from the browser.
func (s *AresDashboardServer) PostAudit(c *hosting.Context) hosting.Result {
	if !DashboardAuthPreprocessorHasValidAdminKey(c.Request) {
		return hosting.Status(403, stj.O("error", "Dashboard administrator required"))
	}
	method := strings.ToUpper(strings.TrimSpace(c.BodyString("method")))
	path := strings.TrimSpace(c.BodyString("path"))
	if method == "" || path == "" {
		return hosting.Status(400, stj.O("error", "method and path are required"))
	}
	status := 0
	if v, ok := c.BodyValue("status"); ok {
		status = int(v.GetInt64())
	}
	detail := c.BodyString("detail")
	if len(detail) > dashAuditDetailMax {
		detail = detail[:dashAuditDetailMax] + "…"
	}
	now := time.Now().UTC()
	e := NewDashboardAuditEntry()
	e.TsMs = now.UnixMilli()
	e.Ts = now.Format(time.RFC3339)
	e.ActorId = strings.TrimSpace(c.BodyString("actor_id"))
	e.ActorName = strings.TrimSpace(c.BodyString("actor_name"))
	e.Scoped = c.BodyString("scoped") == "true"
	e.Method = method
	e.Service = strings.TrimSpace(c.BodyString("service"))
	e.Path = path
	e.Status = status
	e.Detail = detail
	e.IP = c.Request.Remote()
	dashAudit().Insert(e)
	logging.Console("[AUDIT] " + e.ActorName + " (" + e.ActorId + ") " + method + " " + path + " -> " + strconv.Itoa(status))
	return hosting.Ok(stj.O("success", true))
}

// GetAudit lists recent audit entries, newest first. Admin only. Query: limit (default 200, max
// 2000), actor (substring of id or name), q (substring of path/detail/actor).
func (s *AresDashboardServer) GetAudit(c *hosting.Context) hosting.Result {
	if !DashboardAuthPreprocessorHasValidAdminKey(c.Request) {
		return hosting.Status(403, stj.O("error", "Dashboard administrator required"))
	}
	limit := c.QueryInt("limit")
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	actor := strings.ToLower(strings.TrimSpace(c.QueryString("actor")))
	q := strings.ToLower(strings.TrimSpace(c.QueryString("q")))

	all := dashAudit().FindAll()
	sort.Slice(all, func(i, j int) bool { return all[i].TsMs > all[j].TsMs })

	out := make([]*DashboardAuditEntry, 0, limit)
	for _, e := range all {
		if actor != "" && !strings.Contains(strings.ToLower(e.ActorId), actor) && !strings.Contains(strings.ToLower(e.ActorName), actor) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Path), q) &&
			!strings.Contains(strings.ToLower(e.Detail), q) &&
			!strings.Contains(strings.ToLower(e.ActorName), q) &&
			!strings.Contains(strings.ToLower(e.Method), q) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	return hosting.Ok(stj.O("entries", out, "total", len(all)))
}
