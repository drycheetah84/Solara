// Ports AUnrealFeatures.Ares/Servers/DashboardAuthPreprocessor.cs.
//
// Gates the DASHBOARD API on :8080 ONLY (/api/*). A request under /api/* must
// carry EITHER a valid dashboard session cookie (a2dash_session) OR the
// dashboard admin key header (x-dashboard-key / x-api-key). Everything else
// ("/", "/auth/*", the HTML) passes untouched. While Enforce=false it only
// logs the would-block decisions ([DASH-AUTH]).
package ares

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"time"

	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/stj"
)

// DashboardAuthPreprocessorSessionCookieName mirrors
// DashboardAuthPreprocessor.SessionCookieName (const).
const DashboardAuthPreprocessorSessionCookieName = "a2dash_session"

// The class's static fields. The original initialised them together in the
// type initializer (on first use of the class, not at process start), which
// is when a missing secret gets generated and printed; dashboardAuthOnce
// reproduces that lazily.
var (
	dashboardAuthOnce sync.Once

	// DashboardAuthPreprocessorEnforce mirrors the public mutable static
	// DashboardAuthPreprocessor.Enforce:
	// (DASHBOARD_AUTH_ENFORCE ?? "1") != "0". Read it through
	// DashboardAuthPreprocessorEnforced() unless the type was already used.
	DashboardAuthPreprocessorEnforce bool

	dashboardSessionSecret string
	dashboardAdminKey      string
)

func dashboardAuthTypeInit() {
	dashboardAuthOnce.Do(func() {
		v, ok := os.LookupEnv("DASHBOARD_AUTH_ENFORCE")
		if !ok {
			v = "1"
		}
		DashboardAuthPreprocessorEnforce = v != "0"
		dashboardSessionSecret = dashboardResolveSecret("DASHBOARD_SESSION_SECRET", "session secret")
		dashboardAdminKey = dashboardResolveSecret("DASHBOARD_ADMIN_KEY", "admin key")
	})
}

// DashboardAuthPreprocessorEnforced returns Enforce after type initialisation.
func DashboardAuthPreprocessorEnforced() bool {
	dashboardAuthTypeInit()
	return DashboardAuthPreprocessorEnforce
}

// dashboardResolveSecret mirrors ResolveSecret: secrets come from the
// ENVIRONMENT, never from source. If unset (or a CHANGE-ME placeholder) a
// strong random one is generated at startup and printed once.
func dashboardResolveSecret(envVar, label string) string {
	v := os.Getenv(envVar)
	if !dashIsNullOrWhiteSpace(v) && !dashHasPrefixFoldOrdinal(v, "CHANGE-ME") {
		return v
	}
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	gen := hex.EncodeToString(b)
	logging.Console("[DASH-AUTH] " + envVar + " not set -- GENERATED a random " + label + " for this run.")
	// SEC-DASH-3: the original printed the value to the console (and thus to
	// journald / log files); Solara writes it to a 0600 file instead.
	if p := dashboardPersistGenerated(envVar, gen); p != "" {
		logging.Console("[DASH-AUTH]     written to " + p + " (owner-only)")
	} else {
		logging.Console("[DASH-AUTH]     " + gen)
	}
	logging.Console("[DASH-AUTH] Set " + envVar + " in .env to keep it stable across restarts.")
	return gen
}

// DashboardAuthPreprocessorAdminKeyForDisplay mirrors AdminKeyForDisplay
// (exposed so the server can print it on boot; never returned over HTTP).
func DashboardAuthPreprocessorAdminKeyForDisplay() string {
	dashboardAuthTypeInit()
	return dashboardAdminKey
}

// DashboardAuthPreprocessor mirrors the IHttpRequestPreprocessor class.
type DashboardAuthPreprocessor struct{}

// TryPreprocessRequest mirrors DashboardAuthPreprocessor.TryPreprocessRequest.
func (p *DashboardAuthPreprocessor) TryPreprocessRequest(request *hosting.Request, response *hosting.Response) (hosting.Result, hosting.PreprocessorResult) {
	dashboardAuthTypeInit()
	dashboardSecurityHeaders(request, response) // SEC-DASH-2
	path := request.Uri()
	guarded := dashHasPrefixFoldOrdinal(path, "/api/")
	if !guarded || request.Method() == "OPTIONS" {
		return nil, hosting.PreOK // login page, SSO callback, HTML, CORS preflight -> always allow
	}

	// SEC-DASH-1: a locked-out client is refused before any key comparison.
	peer, now := dashPeer(request), time.Now()
	if dashboardBlocked(peer, now) {
		return hosting.Configurable(429, "application/json", `{"error":"too many failed attempts"}`),
			hosting.PreFail | hosting.PreStopAfter
	}
	if DashboardAuthPreprocessorHasValidAdminKey(request) || dashboardHasValidSession(request) {
		dashboardRecordSuccess(peer)
		return nil, hosting.PreOK
	}
	if dashboardPresentedCredential(request) {
		dashboardRecordFailure(peer, now)
	}

	if !DashboardAuthPreprocessorEnforce {
		logging.Console("[DASH-AUTH] would-401 (log-only) " + request.Method() + " " + path + " from " + request.Remote() + " — no session/key")
		return nil, hosting.PreOK
	}
	logging.Console("[DASH-AUTH] 401 " + request.Method() + " " + path + " from " + request.Remote() + " — no session/key")
	return hosting.Configurable(401, "application/json", `{"error":"unauthorized"}`),
		hosting.PreFail | hosting.PreStopAfter
}

// DashboardAuthPreprocessorHasValidAdminKey mirrors the public static
// HasValidAdminKey: x-dashboard-key ?? x-api-key ?? "" (a header that is
// present but empty is NOT replaced by the next one), compared in constant time.
func DashboardAuthPreprocessorHasValidAdminKey(req *hosting.Request) bool {
	dashboardAuthTypeInit()
	k, ok := req.Header("x-dashboard-key")
	if !ok {
		k, ok = req.Header("x-api-key")
		if !ok {
			k = ""
		}
	}
	return k != "" && dashboardCtEquals(k, dashboardAdminKey)
}

func dashboardHasValidSession(req *hosting.Request) bool {
	cookieHdr, ok := req.Header("Cookie")
	if !ok {
		cookieHdr = ""
	}
	for _, part := range strings.Split(cookieHdr, ";") {
		p := dashTrim(part)
		if strings.HasPrefix(p, DashboardAuthPreprocessorSessionCookieName+"=") {
			return dashboardValidateJwt(p[len(DashboardAuthPreprocessorSessionCookieName)+1:])
		}
	}
	return false
}

// dashboardValidateJwt mirrors ValidateJwt: session cookie =
// base64url(header).base64url(payload{sub,exp,...}).base64url(HMACSHA256(header.payload)).
// Any exception -> false.
func dashboardValidateJwt(token string) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	signing := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(dashboardSessionSecret))
	mac.Write([]byte(signing))
	sig := dashboardB64Url(mac.Sum(nil))
	if !dashboardCtEquals(sig, parts[2]) {
		return false // signature must verify
	}
	payload, err := dashboardB64UrlDecode(parts[1])
	if err != nil {
		return false
	}
	// Encoding.UTF8.GetString replaces invalid sequences with U+FFFD.
	doc, err := stj.Parse([]byte(strings.ToValidUTF8(string(payload), string(rune(0xFFFD)))))
	if err != nil {
		return false
	}
	if e, found := doc.TryGetProperty("exp"); found {
		if exp, isInt := e.TryGetInt64(); isInt {
			return exp > time.Now().Unix() // not expired
		}
	}
	return true
}

func dashboardB64Url(b []byte) string {
	s := base64.StdEncoding.EncodeToString(b)
	s = strings.ReplaceAll(s, "+", "-")
	s = strings.ReplaceAll(s, "/", "_")
	return strings.TrimRight(s, "=")
}

// dashboardB64UrlDecode mirrors B64UrlDecode + Convert.FromBase64String
// (which skips the whitespace characters space, tab, CR and LF).
func dashboardB64UrlDecode(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	s = strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\r', '\n':
			return -1
		}
		return r
	}, s)
	return base64.StdEncoding.DecodeString(s)
}

// dashboardCtEquals mirrors CryptographicOperations.FixedTimeEquals over the
// UTF-8 bytes (false on length mismatch).
func dashboardCtEquals(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
