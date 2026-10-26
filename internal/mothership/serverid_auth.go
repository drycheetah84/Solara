// Per-server identity tokens on the Mothership (:90) -- Solara addition
// (docs/SERVER_IDENTITY.md, KNOWN_ISSUES SEC-SID-*).
//
// Wherever the original accepted the static SERVER_API_KEY in
// x-server-api-key (IsTrustedServer, and the server half of IsAdmin), a valid
// per-server token (Kind "server") is accepted too. The static key keeps
// working in compat mode (with a rate-limited deprecation warning naming the
// route) and is rejected when SOLARA_REQUIRE_SERVER_TOKENS=1.
package mothership

import (
	"strings"

	"solara/internal/hosting"
	"solara/internal/serverid"
)

// serverTokenFromRequest validates a token presented in x-server-api-key.
func serverTokenFromRequest(req *hosting.Request) *serverid.ServerTokenDbObject {
	k, ok := req.Header("x-server-api-key")
	if !ok || !serverid.LooksLikeToken(k) {
		return nil
	}
	if r, ok := serverid.Validate(k); ok && r.Kind == serverid.KindServer {
		return r
	}
	return nil
}

// staticServerKeyOK applies the strict/compat policy after the static key matched.
func staticServerKeyOK(req *hosting.Request) bool {
	return serverid.StaticKeyAllowed("SERVER_API_KEY", routeLabel(req))
}

// routeLabel names a request for the deprecation warning without per-player
// path segments (any segment containing a digit becomes "*").
func routeLabel(req *hosting.Request) string {
	segs := strings.Split(req.Uri(), "/")
	for i, s := range segs {
		if strings.ContainsAny(s, "0123456789") || len(s) > 40 {
			segs[i] = "*"
		}
	}
	return "Mothership " + req.Method() + " " + strings.Join(segs, "/")
}
