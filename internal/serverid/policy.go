package serverid

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"solara/internal/logging"
)

// RequireServerTokens reports strict mode (env SOLARA_REQUIRE_SERVER_TOKENS=1
// / true / yes): the static shared game keys (SERVER_API_KEY /
// HALCYON_SERVER_API_KEY and ServerMasterKey) are then rejected wherever they
// authenticate a game server, and the game-server ingest routes on :78 need a
// valid server token. Read on every call so it can be flipped by a restart
// (and by tests).
func RequireServerTokens() bool { return envTrue("SOLARA_REQUIRE_SERVER_TOKENS") }

func envTrue(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// EnvTrue is exported for sibling packages that read similar switches.
func EnvTrue(name string) bool { return envTrue(name) }

// staticWarn rate-limits the deprecation warning to once per hour per route.
var (
	staticWarnMu   sync.Mutex
	staticWarnLast = map[string]time.Time{}
	// WarnLog is where deprecation warnings go (swappable by tests).
	WarnLog = func(s string) { logging.Console(s) }
	warnNow = time.Now
)

// StaticKeyAllowed is called wherever a static shared game key just matched.
// In strict mode it returns false (the caller must treat the key as invalid).
// In compat mode it returns true and logs a deprecation warning naming the
// route, at most once per hour per route. The key itself is never logged.
func StaticKeyAllowed(keyName, route string) bool {
	if RequireServerTokens() {
		warnOnce("strict:"+keyName+":"+route, fmt.Sprintf("[ServerId] REJECTED static %s on %s (SOLARA_REQUIRE_SERVER_TOKENS=1): use a per-server token", keyName, route))
		return false
	}
	warnOnce(keyName+":"+route, fmt.Sprintf("[ServerId] DEPRECATED: static %s accepted on %s -- issue per-server tokens and remove the static key from agent configs (docs/SERVER_IDENTITY.md)", keyName, route))
	return true
}

func warnOnce(key, msg string) {
	now := warnNow()
	staticWarnMu.Lock()
	last, ok := staticWarnLast[key]
	if ok && now.Sub(last) < time.Hour {
		staticWarnMu.Unlock()
		return
	}
	if len(staticWarnLast) > 4096 { // bounded: routes are a fixed set, keys too
		staticWarnLast = map[string]time.Time{}
	}
	staticWarnLast[key] = now
	staticWarnMu.Unlock()
	WarnLog(msg)
}

// TokenFromHeaders returns the first header value that looks like a token
// (x-server-api-key first, then x-api-key), or "".
func TokenFromHeaders(get func(name string) (string, bool)) string {
	for _, h := range []string{"x-server-api-key", "x-api-key"} {
		if v, ok := get(h); ok && LooksLikeToken(v) {
			return v
		}
	}
	return ""
}
