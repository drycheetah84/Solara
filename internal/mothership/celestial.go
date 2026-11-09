// Celestial: Solara's Quest-client anticheat gate.
//
// Flow:
//  1. libmidnight.so calls POST /v1/celestial/token (header X-Celestial-IP = the IP the client
//     believes it has; optional JSON body {"obb":"<sha256 of the OBB>"}).
//  2. Mothership answers {"celestialToken":"<jwt>","expiresIn":7200}. The JWT is HS256-signed with
//     the server-only secret CELESTIAL_KEY and carries
//     {"Claim":"trusted","ip":<IP the server saw>,"obb":...,"iat":...,"exp":iat+2h,"jti":...}.
//  3. The Quest login routes (/v2/player/client/auth/begin|complete/QUEST) require that token, either
//     as header x-celestial-token or query ?celestialToken=. It must verify, be unexpired, carry
//     Claim=trusted, and its ip must match the IP of the login request.
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
)

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

// parseCelestialToken verifies signature, Claim, expiry and IP binding. Returns "" when valid.
func parseCelestialToken(tok, ip string, now time.Time) (celestialClaims, string) {
	var c celestialClaims
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		return c, "malformed celestialToken"
	}
	if !hmac.Equal([]byte(celSign(p[0]+"."+p[1])), []byte(p[2])) {
		return c, "celestialToken signature invalid"
	}
	raw, err := base64.RawURLEncoding.DecodeString(p[1])
	if err != nil || json.Unmarshal(raw, &c) != nil {
		return c, "malformed celestialToken"
	}
	if c.Claim != celestialClaim {
		return c, "celestialToken is not trusted"
	}
	if now.Unix() >= c.Exp {
		return c, "celestialToken expired"
	}
	if c.IP != ip {
		return c, fmt.Sprintf("celestialToken issued to %s, used from %s", c.IP, ip)
	}
	return c, ""
}

// CelestialToken: POST /v1/celestial/token. Issues the 2-hour trusted JWT to the calling client.
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
// Our own game servers (trusted server key) are exempt. "log" mode only logs.
func celestialGate(req *hosting.Request, stage, userId string) (hosting.Result, bool) {
	celestialInit()
	if celMode == "off" || isTrustedServer(req) {
		return nil, true
	}
	tok, _ := req.Header("x-celestial-token")
	if tok == "" {
		tok, _ = req.Query("celestialToken")
	}
	ip := req.Remote()
	reason := "missing celestialToken (client without Celestial)"
	var cl celestialClaims
	if tok != "" {
		cl, reason = parseCelestialToken(tok, ip, time.Now())
	}
	verdict := "OK"
	if reason != "" {
		verdict = "FAIL"
	}
	authLog(fmt.Sprintf("[CELESTIAL] %s %s userId=%s mode=%s reason='%s' ip=%s obb=%s jti=%s",
		stage, verdict, userId, celMode, reason, ip, cl.Obb, cl.Jti))
	if reason == "" || celMode != "enforce" {
		return nil, true
	}
	return errorResult("Celestial: "+reason+". Install the current Halcyon build.", 401), false
}
