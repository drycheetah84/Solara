// Celestial: Solara's Quest-client anticheat gate.
//
// Flow:
//  1. libmidnight.so calls POST /v1/celestial/token (header X-Celestial-IP = the IP the client
//     believes it has; optional JSON body {"obb":"<sha256 of the OBB>"}).
//  2. Mothership validates the OBB, then marks that caller's IP "trusted" for 2 hours and answers
//     {"celestialToken":"<jwt>","expiresIn":7200} (the JWT is a signed record of the grant; the lib
//     does not need to attach it to anything).
//  3. The Quest login (/v2/player/client/auth/complete/QUEST) is allowed when the login's IP was
//     issued a trusted token within the last 2 hours. Nothing is attached to the login itself — the
//     server knows the IP. So the lib only has to call /v1/celestial/token on startup; there is no
//     race between the token fetch and any single login request.
//
// Policy (env, read once):
//
//	SOLARA_CELESTIAL_MODE         off | log | enforce (default enforce)
//	SOLARA_CELESTIAL_OBB_SHA256   comma list of accepted OBB hashes; when set, /token refuses others
//
// CELESTIAL_KEY comes from env SOLARA_CELESTIAL_KEY or config/legacy-secrets.env (hex, >= 32 bytes).
// Without it a random per-process key is used (tokens then die on restart).
package mothership

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"solara/internal/hosting"
	"solara/internal/secrets"
	"solara/internal/stj"
)

const (
	celestialTTL   = 2 * time.Hour
	celestialClaim = "trusted"
)

var (
	celOnce     sync.Once
	celMode     string
	celKey      []byte
	celObbAllow map[string]bool

	// celTrust maps an IP to the unix time its Celestial grant expires. A login from an IP present
	// (and unexpired) here is trusted. This is why the login needs nothing attached: the grant is
	// remembered per IP for the token's lifetime.
	celTrustMu sync.Mutex
	celTrust   = map[string]int64{}
)

// celMarkTrusted records that ip holds a valid Celestial grant for the token's lifetime, and prunes
// expired entries so the map stays bounded by the number of recent, distinct client IPs.
func celMarkTrusted(ip string, now time.Time) {
	exp := now.Add(celestialTTL).Unix()
	celTrustMu.Lock()
	defer celTrustMu.Unlock()
	nowU := now.Unix()
	for k, e := range celTrust {
		if e <= nowU {
			delete(celTrust, k)
		}
	}
	celTrust[ip] = exp
}

// celIsTrusted reports whether ip has an unexpired Celestial grant.
func celIsTrusted(ip string, now time.Time) bool {
	celTrustMu.Lock()
	defer celTrustMu.Unlock()
	exp, ok := celTrust[ip]
	if !ok {
		return false
	}
	if now.Unix() >= exp {
		delete(celTrust, ip)
		return false
	}
	return true
}

func celestialInit() {
	celOnce.Do(func() {
		celMode = strings.ToLower(strings.TrimSpace(os.Getenv("SOLARA_CELESTIAL_MODE")))
		if celMode == "" {
			celMode = "enforce"
		}
		if k, err := hex.DecodeString(strings.TrimSpace(secrets.Get("CELESTIAL_KEY"))); err == nil && len(k) >= 32 {
			celKey = k
		} else {
			celKey = make([]byte, 32)
			_, _ = rand.Read(celKey)
			authLog("[CELESTIAL] CELESTIAL_KEY not set: using a random key, tokens will not survive a restart")
		}
		celObbAllow = map[string]bool{}
		for _, s := range strings.Split(os.Getenv("SOLARA_CELESTIAL_OBB_SHA256"), ",") {
			if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
				celObbAllow[s] = true
			}
		}
	})
}

// celestialClaims is the JWT payload. Field names are the wire contract with libmidnight.so.
type celestialClaims struct {
	Claim string `json:"Claim"`
	IP    string `json:"ip"`
	Obb   string `json:"obb,omitempty"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
	Jti   string `json:"jti"`
}

func celSign(signing string) string {
	m := hmac.New(sha256.New, celKey)
	m.Write([]byte(signing))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func issueCelestialToken(c celestialClaims) string {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, _ := json.Marshal(c)
	signing := hdr + "." + base64.RawURLEncoding.EncodeToString(body)
	return signing + "." + celSign(signing)
}

// CelestialToken: POST /v1/celestial/token. Validates the OBB, marks the caller's IP trusted for
// 2 hours, and returns a signed record of the grant (the lib does not have to use it).
func (s *MothershipServer) CelestialToken(c *hosting.Context) hosting.Result {
	celestialInit()
	req := c.Request
	ip := req.Remote()
	claimed, _ := req.Header("X-Celestial-IP")
	obb := strings.ToLower(strings.TrimSpace(str(parseBody(req), "obb")))
	if len(celObbAllow) > 0 && !celObbAllow[obb] {
		authLog(fmt.Sprintf("[CELESTIAL] token DENY ip=%s claimedIp=%s obb=%s reason='OBB not recognised'", ip, claimed, obb))
		return errorResult("Celestial: game files not recognised. Install the current Halcyon build.", 403)
	}
	now := time.Now()
	celMarkTrusted(ip, now)
	jti := make([]byte, 16)
	_, _ = rand.Read(jti)
	tok := issueCelestialToken(celestialClaims{
		Claim: celestialClaim, IP: ip, Obb: obb,
		Iat: now.Unix(), Exp: now.Add(celestialTTL).Unix(), Jti: hex.EncodeToString(jti),
	})
	authLog(fmt.Sprintf("[CELESTIAL] token ISSUED ip=%s claimedIp=%s obb=%s", ip, claimed, obb))
	return jsonAnon(stj.O("celestialToken", tok, "expiresIn", int(celestialTTL/time.Second)), 200)
}

// celestialGate runs on the Quest login routes; (result, false) means refuse the login.
// The login is trusted when its IP was issued a Celestial token within the last 2 hours.
// Our own game servers (trusted server key) are exempt. "log" mode only logs.
func celestialGate(req *hosting.Request, stage, userId string) (hosting.Result, bool) {
	celestialInit()
	if celMode == "off" || isTrustedServer(req) {
		return nil, true
	}
	ip := req.Remote()
	trusted := celIsTrusted(ip, time.Now())
	verdict, reason := "OK", ""
	if !trusted {
		verdict, reason = "FAIL", "no trusted Celestial token for this IP (client without Celestial)"
	}
	authLog(fmt.Sprintf("[CELESTIAL] %s %s userId=%s mode=%s reason='%s' ip=%s", stage, verdict, userId, celMode, reason, ip))
	if trusted || celMode != "enforce" {
		return nil, true
	}
	return errorResult("Celestial: "+reason+". Install the current Halcyon build.", 401), false
}
