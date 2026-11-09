package mothership

import (
	"strings"
	"testing"
	"time"
)

func TestCelestialToken(t *testing.T) {
	celestialInit()
	now := time.Unix(1_800_000_000, 0)
	good := celestialClaims{Claim: "trusted", IP: "1.2.3.4", Obb: "ab", Iat: now.Unix(), Exp: now.Add(2 * time.Hour).Unix(), Jti: "x"}
	tok := issueCelestialToken(good)

	if c, r := parseCelestialToken(tok, "1.2.3.4", now); r != "" || c.Obb != "ab" {
		t.Fatalf("good token: %q", r)
	}
	if _, r := parseCelestialToken(tok, "1.2.3.4", now.Add(2*time.Hour+time.Second)); !strings.Contains(r, "expired") {
		t.Fatalf("after 2h: %q", r)
	}
	if _, r := parseCelestialToken(tok, "5.6.7.8", now); !strings.Contains(r, "issued to") {
		t.Fatalf("other ip: %q", r)
	}
	if _, r := parseCelestialToken(tok[:len(tok)-2]+"AA", "1.2.3.4", now); !strings.Contains(r, "signature") {
		t.Fatalf("tampered sig: %q", r)
	}
	bad := good
	bad.Claim = "untrusted"
	if _, r := parseCelestialToken(issueCelestialToken(bad), "1.2.3.4", now); !strings.Contains(r, "not trusted") {
		t.Fatalf("claim: %q", r)
	}
	if _, r := parseCelestialToken("a.b", "1.2.3.4", now); !strings.Contains(r, "malformed") {
		t.Fatalf("malformed: %q", r)
	}
}

func TestCelestialLoginGate(t *testing.T) {
	s := newTestServer(t)
	celestialInit()
	old := celMode
	celMode = "enforce"
	defer func() { celMode = old }()

	// no token: the client login is refused before attestation
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"c1"}`)
	rec := do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"c1","AttestationToken":"x","MetaNonce":"n"}`)
	if rec.Code != 401 || !strings.Contains(rec.Body.String(), "missing celestialToken") {
		t.Fatal(rec.Code, rec.Body.String())
	}

	// the lib's request: token comes back, and it opens the gate from the same IP
	m := decode(t, do(t, s, "POST", "/v1/celestial/token", `{}`, "X-Celestial-IP", "192.0.2.1"))
	tok, _ := m["celestialToken"].(string)
	if tok == "" || m["expiresIn"] != float64(7200) {
		t.Fatal(m)
	}
	c, reason := parseCelestialToken(tok, "192.0.2.1", time.Now())
	if reason != "" || c.Claim != "trusted" || c.Exp-c.Iat != 7200 {
		t.Fatal(reason, c)
	}
	rec = do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"c1","AttestationToken":"x","MetaNonce":"n"}`, "x-celestial-token", tok)
	if strings.Contains(rec.Body.String(), "Celestial") {
		t.Fatal("valid token refused:", rec.Body.String())
	}

	// trusted game servers are exempt
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"q1234567890"}`)
	rec = do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"q1234567890","AttestationToken":"x","MetaNonce":"n"}`, "x-server-api-key", testServerKey)
	if rec.Code != 201 {
		t.Fatal("trusted server refused:", rec.Code, rec.Body.String())
	}
}
