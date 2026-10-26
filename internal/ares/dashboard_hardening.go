// Security hardening of the dashboard server (:8080) — deliberate deviations
// from the original, documented in KNOWN_ISSUES (SEC-DASH-*):
//
//   - SEC-DASH-1 brute force: per-client throttling of failed credential
//     attempts on /api/* (keyed by the socket peer address, which, unlike
//     Cf-Connecting-Ip / X-Real-Ip, cannot be spoofed by the client).
//   - SEC-DASH-2 browser hardening headers on every response; a
//     Content-Security-Policy on the embedded page; no-store on /api/*.
//   - SEC-DASH-3 generated secrets are written to a 0600 file instead of the
//     console (they used to land in journald / log files).
//   - SEC-DASH-4 startup warnings for disabled enforcement or weak keys.
//   - SEC-DASH-5 optional DASHBOARD_BIND_ADDRESS (e.g. 127.0.0.1 when only the
//     Next.js proxy on the same host should reach the API).
package ares

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"solara/internal/hosting"
	"solara/internal/logging"
)

// Throttling parameters.
var (
	DashboardMaxFailedAttempts = 10
	DashboardFailWindow        = 5 * time.Minute
	DashboardLockout           = 15 * time.Minute
)

type dashFailState struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

var (
	dashFailMu sync.Mutex
	dashFails  = map[string]*dashFailState{}
)

func dashPeer(req *hosting.Request) string {
	host, _, err := net.SplitHostPort(req.Raw().RemoteAddr)
	if err != nil {
		return req.Raw().RemoteAddr
	}
	return host
}

// dashboardBlocked reports whether the client is currently locked out.
func dashboardBlocked(peer string, now time.Time) bool {
	dashFailMu.Lock()
	defer dashFailMu.Unlock()
	st := dashFails[peer]
	return st != nil && now.Before(st.blockedUntil)
}

// dashboardRecordFailure counts a failed credential attempt.
func dashboardRecordFailure(peer string, now time.Time) {
	dashFailMu.Lock()
	defer dashFailMu.Unlock()
	if len(dashFails) > 10000 { // bound memory: drop idle entries
		for k, v := range dashFails {
			if now.After(v.blockedUntil) && now.Sub(v.windowStart) > DashboardFailWindow {
				delete(dashFails, k)
			}
		}
	}
	st := dashFails[peer]
	if st == nil || now.Sub(st.windowStart) > DashboardFailWindow {
		st = &dashFailState{windowStart: now}
		dashFails[peer] = st
	}
	st.count++
	if st.count >= DashboardMaxFailedAttempts {
		st.blockedUntil = now.Add(DashboardLockout)
		st.count = 0
		st.windowStart = now
		logging.Console("[DASH-AUTH] " + peer + " locked out for " + DashboardLockout.String() + " after repeated failed credentials")
	}
}

func dashboardRecordSuccess(peer string) {
	dashFailMu.Lock()
	delete(dashFails, peer)
	dashFailMu.Unlock()
}

// dashboardPresentedCredential reports whether the request carried any
// dashboard credential (only those attempts count as failures).
func dashboardPresentedCredential(req *hosting.Request) bool {
	for _, h := range []string{"x-dashboard-key", "x-api-key"} {
		if v, ok := req.Header(h); ok && v != "" {
			return true
		}
	}
	c, _ := req.Header("Cookie")
	return strings.Contains(c, DashboardAuthPreprocessorSessionCookieName+"=")
}

const dashboardIndexCSP = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' http: https: data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'; object-src 'none'"

// dashboardSecurityHeaders adds browser hardening headers to every response.
func dashboardSecurityHeaders(req *hosting.Request, resp *hosting.Response) {
	resp.SetHeader("X-Content-Type-Options", "nosniff")
	resp.SetHeader("X-Frame-Options", "DENY")
	resp.SetHeader("Referrer-Policy", "no-referrer")
	path := req.Uri()
	if path == "/" {
		resp.SetHeader("Content-Security-Policy", dashboardIndexCSP)
	}
	if dashHasPrefixFoldOrdinal(path, "/api/") {
		resp.SetHeader("Cache-Control", "no-store")
	}
}

// DashboardSecretsDir is where generated dashboard secrets are written.
var DashboardSecretsDir = "config"

// dashboardPersistGenerated writes a generated secret to a private file and
// returns its path ("" on failure, in which case the caller falls back to the
// console so the operator is never locked out).
func dashboardPersistGenerated(envVar, value string) string {
	if err := os.MkdirAll(DashboardSecretsDir, 0o700); err != nil {
		return ""
	}
	p := filepath.Join(DashboardSecretsDir, "generated-"+strings.ToLower(envVar)+".txt")
	if err := os.WriteFile(p, []byte(value+"\n"), 0o600); err != nil {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// dashboardStartupWarnings prints loud warnings for insecure configurations.
func dashboardStartupWarnings() {
	dashboardAuthTypeInit()
	if !DashboardAuthPreprocessorEnforce {
		logging.Console("[DASH-AUTH] WARNING: DASHBOARD_AUTH_ENFORCE=0 -- the dashboard API (:8080 /api/*) is OPEN to anyone who can reach it (log-only mode).")
	}
	if v := os.Getenv("DASHBOARD_ADMIN_KEY"); v != "" && len(strings.TrimSpace(v)) < 24 && !dashHasPrefixFoldOrdinal(v, "CHANGE-ME") {
		logging.Console("[DASH-AUTH] WARNING: DASHBOARD_ADMIN_KEY is short (<24 chars). Use a long random value, e.g. `openssl rand -hex 32`.")
	}
	if b := os.Getenv("DASHBOARD_BIND_ADDRESS"); b == "" {
		logging.Console("[DASH-AUTH] note: dashboard listens on all interfaces; set DASHBOARD_BIND_ADDRESS=127.0.0.1 if only the local Next.js dashboard should reach it.")
	}
}

// dashboardSanitizeForLog strips line breaks (log injection) and redacts
// OAuth values from a logged query value.
func dashboardSanitizeForLog(key, value string) string {
	switch strings.ToLower(key) {
	case "code", "token", "access_token", "id_token", "refresh_token", "state", "nonce":
		if value != "" {
			return "<redacted>"
		}
	}
	return strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(value)
}
