package ares

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"solara/internal/hosting"
)

func resetThrottle() {
	dashFailMu.Lock()
	dashFails = map[string]*dashFailState{}
	dashFailMu.Unlock()
}

// SEC-DASH-1: repeated bad credentials lock the client out, even with forged
// forwarding headers; other clients are unaffected; the lockout expires.
func TestDashboardBruteForceLockout(t *testing.T) {
	resetDashAuth(t, nil)
	resetThrottle()
	oldLock := DashboardLockout
	DashboardLockout = 300 * time.Millisecond
	defer func() { DashboardLockout = oldLock; resetThrottle() }()
	s := newAresDashboardServer()

	for i := 0; i < DashboardMaxFailedAttempts; i++ {
		// rotating spoofed client IPs must not reset the counter (keyed by socket peer)
		w := do(t, s, "GET", "/api/stats", "", map[string]string{"x-dashboard-key": "guess", "Cf-Connecting-Ip": "10.0.0." + itoaTest(i+1)})
		if w.Code != 401 {
			t.Fatalf("attempt %d: %d", i, w.Code)
		}
	}
	if w := do(t, s, "GET", "/api/stats", "", adminHdr); w.Code != 429 {
		t.Fatalf("locked-out client with the right key: %d, want 429", w.Code)
	}
	// a different socket peer is not affected
	r := newReq("GET", "/api/stats", adminHdr)
	r.RemoteAddr = net.JoinHostPort("198.51.100.7", "4000")
	if w := serve(s, r); w.Code != 200 {
		t.Errorf("other client: %d", w.Code)
	}
	// unauthenticated probes without any credential do not count towards lockout
	resetThrottle()
	for i := 0; i < 3*DashboardMaxFailedAttempts; i++ {
		do(t, s, "GET", "/api/stats", "", nil)
	}
	if w := do(t, s, "GET", "/api/stats", "", adminHdr); w.Code != 200 {
		t.Errorf("credential-less probes caused a lockout: %d", w.Code)
	}
	// lockout expiry
	for i := 0; i < DashboardMaxFailedAttempts; i++ {
		do(t, s, "GET", "/api/stats", "", map[string]string{"x-dashboard-key": "guess"})
	}
	time.Sleep(400 * time.Millisecond)
	if w := do(t, s, "GET", "/api/stats", "", adminHdr); w.Code != 200 {
		t.Errorf("after lockout expiry: %d", w.Code)
	}
}

// SEC-DASH-3: a generated admin key goes to a 0600 file, not the console.
func TestDashboardGeneratedKeyWrittenToFile(t *testing.T) {
	dir := t.TempDir()
	old := DashboardSecretsDir
	DashboardSecretsDir = filepath.Join(dir, "config")
	defer func() { DashboardSecretsDir = old }()
	t.Setenv("DASHBOARD_ADMIN_KEY", "")
	t.Setenv("DASHBOARD_SESSION_SECRET", "s")
	dashboardAuthOnce = sync.Once{}
	key := DashboardAuthPreprocessorAdminKeyForDisplay()
	raw, err := os.ReadFile(filepath.Join(DashboardSecretsDir, "generated-dashboard_admin_key.txt"))
	if err != nil || strings.TrimSpace(string(raw)) != key || len(key) != 64 {
		t.Fatalf("generated key file: %v %q", err, raw)
	}
	dashboardAuthOnce = sync.Once{}
}

func TestDashboardSecurityHeadersAndLogRedaction(t *testing.T) {
	resetDashAuth(t, nil)
	resetThrottle()
	s := newAresDashboardServer()
	w := do(t, s, "GET", "/api/stats", "", adminHdr)
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("api headers: %v", w.Header())
	}
	// blocked responses still carry no CORS headers (original behaviour HOST-007)
	w = do(t, s, "GET", "/api/stats", "", nil)
	if w.Code != 401 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("blocked: %d %v", w.Code, w.Header())
	}
	if got := dashboardSanitizeForLog("code", "secret-oauth-code"); got != "<redacted>" {
		t.Errorf("code not redacted: %s", got)
	}
	if got := dashboardSanitizeForLog("x", "a\r\nFAKE LOG LINE"); strings.ContainsAny(got, "\r\n") {
		t.Errorf("line break not neutralised: %q", got)
	}
}

// SEC-DASH-5: DASHBOARD_BIND_ADDRESS restricts the listening interface.
func TestDashboardBindAddress(t *testing.T) {
	t.Setenv("DASHBOARD_BIND_ADDRESS", "127.0.0.1")
	s := newAresDashboardServer()
	if s.HTTP.BindAddress != "127.0.0.1" {
		t.Fatalf("bind address %q", s.HTTP.BindAddress)
	}
	hs := hosting.NewServer("bindtest", "localhost", 0)
	hs.BindAddress = "127.0.0.1"
	old := hosting.DisableListen
	hosting.DisableListen = false
	defer func() { hosting.DisableListen = old }()
	if err := hs.Start(); err != nil {
		t.Fatal(err)
	}
	defer hs.Stop()
	if host, _, _ := net.SplitHostPort(hs.Addr()); host != "127.0.0.1" {
		t.Errorf("listening on %s", hs.Addr())
	}
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func newReq(method, target string, hdr map[string]string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return r
}

func serve(s *AresDashboardServer, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.HTTP.ServeHTTP(w, r)
	return w
}
