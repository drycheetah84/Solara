package mothership

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"solara/internal/hosting"
)

func celReqIP(ip string) *hosting.Request {
	r := httptest.NewRequest("POST", "/v2/player/client/auth/complete/QUEST", nil)
	if ip != "" {
		r.Header.Set("X-Real-Ip", ip)
	}
	return hosting.NewRequest(r)
}

func TestCelestialTrustMap(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	celMarkTrusted("9.9.9.9", now)
	if !celIsTrusted("9.9.9.9", now) {
		t.Fatal("marked IP should be trusted")
	}
	if celIsTrusted("8.8.8.8", now) {
		t.Fatal("an unmarked IP must not be trusted")
	}
	if celIsTrusted("9.9.9.9", now.Add(celestialTTL+time.Second)) {
		t.Fatal("trust should lapse after the TTL")
	}
}

func TestCelestialGateIP(t *testing.T) {
	celestialInit()
	old := celMode
	defer func() { celMode = old }()

	celMode = "enforce"
	if _, ok := celestialGate(celReqIP("5.5.5.5"), "complete", "u"); ok {
		t.Fatal("an untrusted IP should be refused in enforce")
	}
	celMarkTrusted("5.5.5.5", time.Now())
	if _, ok := celestialGate(celReqIP("5.5.5.5"), "complete", "u"); !ok {
		t.Fatal("a trusted IP should pass")
	}
	celMode = "log"
	if _, ok := celestialGate(celReqIP("6.6.6.6"), "complete", "u"); !ok {
		t.Fatal("log mode should always pass")
	}
}

// End to end: the lib's /v1/celestial/token call makes its IP trusted; a later login from that IP
// passes with nothing attached. A different IP is refused. Game servers are exempt.
func TestCelestialLoginGateE2E(t *testing.T) {
	s := newTestServer(t)
	celestialInit()
	old := celMode
	celMode = "enforce"
	defer func() { celMode = old }()
	celTrustMu.Lock()
	celTrust = map[string]int64{}
	celTrustMu.Unlock()

	// login from an IP that never fetched a token -> refused
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"c1"}`, "X-Real-Ip", "7.7.7.7")
	rec := do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"c1","AttestationToken":"x","MetaNonce":"n"}`, "X-Real-Ip", "7.7.7.7")
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "no trusted Celestial token") {
		t.Fatal(rec.Code, rec.Body.String())
	}

	// the lib fetches a token from that IP
	m := decode(t, do(t, s, "POST", "/v1/celestial/token", `{}`, "X-Real-Ip", "7.7.7.7"))
	if m["celestialToken"] == nil || m["expiresIn"] != float64(7200) {
		t.Fatal(m)
	}

	// now any login from that IP is allowed, nothing attached
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"c1"}`, "X-Real-Ip", "7.7.7.7")
	rec = do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"c1","AttestationToken":"x","MetaNonce":"n"}`, "X-Real-Ip", "7.7.7.7")
	if strings.Contains(rec.Body.String(), "Celestial") {
		t.Fatal("trusted-IP login refused:", rec.Body.String())
	}

	// a different IP is still untrusted
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"c2"}`, "X-Real-Ip", "8.8.8.8")
	rec = do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"c2","AttestationToken":"x","MetaNonce":"n"}`, "X-Real-Ip", "8.8.8.8")
	if rec.Code != 401 {
		t.Fatal("untrusted IP should be refused:", rec.Code, rec.Body.String())
	}

	// trusted game servers are exempt regardless of IP
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"q1234567890"}`, "X-Real-Ip", "3.3.3.3")
	rec = do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"q1234567890","AttestationToken":"x","MetaNonce":"n"}`, "x-server-api-key", testServerKey, "X-Real-Ip", "3.3.3.3")
	if rec.Code != 201 {
		t.Fatal("trusted server refused:", rec.Code, rec.Body.String())
	}
}
