// Security hardening of the HalcyonSocket control surface (Solara addition;
// KNOWN_ISSUES F5 / SEC-SID, docs/SERVER_IDENTITY.md):
//
//   - every :9095 HTTP route requires an admin credential;
//   - spin-up inputs are validated centrally in RequestSpinUp (so the
//     dashboard's in-process path is covered too): the original passed map and
//     args straight onto the game's command line, so a space or quote in
//     either injected extra Unreal flags (e.g. -ExecCmds="...").
package halcyonsocket

import (
	"crypto/subtle"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"solara/internal/hosting"
	"solara/internal/secrets"
	"solara/internal/serverid"
	"solara/internal/stj"
)

// ─── :9095 authentication ────────────────────────────────────────────────────

// AdminKeyValidator, when set (the host wires the dashboard's
// DashboardAuthPreprocessorHasValidAdminKey), decides whether a request
// carries the dashboard admin key. When nil, x-dashboard-key is compared with
// env DASHBOARD_ADMIN_KEY.
var AdminKeyValidator func(req *hosting.Request) bool

// controlAuthorized: the dashboard admin key (x-dashboard-key), or the static
// ServerMasterKey in x-api-key while the static-key policy allows it (compat
// mode). No local tooling of the original called :9095 (the dashboard reaches
// it in-process), so there is no unauthenticated escape hatch.
func controlAuthorized(req *hosting.Request) bool {
	if k, ok := req.Header("x-dashboard-key"); ok && k != "" {
		if AdminKeyValidator != nil {
			if AdminKeyValidator(req) {
				return true
			}
		} else if env := strings.TrimSpace(os.Getenv("DASHBOARD_ADMIN_KEY")); env != "" && ctEq(k, env) {
			return true
		}
	}
	if k, ok := req.Header("x-api-key"); ok && ctEq(k, secrets.Get("ServerMasterKey")) {
		return serverid.StaticKeyAllowed("ServerMasterKey", "HalcyonSocket "+req.Method()+" "+req.Uri())
	}
	return false
}

func ctEq(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// requireAdmin wraps a :9095 handler with controlAuthorized.
func requireAdmin(h hosting.Handler) hosting.Handler {
	return func(c *hosting.Context) hosting.Result {
		if !controlAuthorized(c.Request) {
			return hosting.Unauthorized(stj.O("error", "unauthorized"))
		}
		return h(c)
	}
}

// ─── spin-up input validation (F5) ───────────────────────────────────────────

var (
	// Unreal map / game-mode paths: /Game/Maps/Arena, Arena, A2/Mode.Thing
	spinMapRe    = regexp.MustCompile(`^[A-Za-z0-9_./-]{1,128}$`)
	spinRegionRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	spinBoxRe    = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	spinNameRe   = regexp.MustCompile(`^[\p{L}\p{N} _.,'!?&()#:+-]{1,64}$`)
	// one launch flag: -Flag or -Key=Value with a conservative value charset
	spinArgRe = regexp.MustCompile(`^-[A-Za-z][A-Za-z0-9_]*(=[A-Za-z0-9_./:,+-]*)?$`)
)

const (
	maxSpinArgs    = 32
	maxSpinArgsLen = 2048
)

// deniedSpinFlags are launch flags a spin-up caller may never pass
// (case-insensitive; the game's FParse matches flags case-insensitively):
//
//   - execcmds, exec, dpcvars, cvarsini: run console commands / set console
//     variables at startup (arbitrary server control).
//   - run: runs a commandlet instead of the game.
//   - abslog, log=<path>: write the log to an arbitrary path (plain -log is fine).
//   - userdir and any flag ending in "ini" (ENGINEINI, GAMEINI, DEFENGINEINI,
//     ...): load configuration from a caller-chosen place (= any setting).
//   - serverapikey, dashboardapikey, dashboarddeploymentid: the per-server
//     identity is minted by the backend; a caller must not choose or replace it.
//   - registerip, backendhost, backendport, mothershiphost, mothershipport,
//     dashboardapiurl: would point the server (and the token it presents) at
//     another host -- token theft / fake registrations.
var deniedSpinFlags = map[string]bool{
	"execcmds": true, "exec": true, "dpcvars": true, "cvarsini": true, "run": true,
	"abslog": true, "userdir": true,
	"serverapikey": true, "dashboardapikey": true, "dashboarddeploymentid": true,
	"registerip": true, "backendhost": true, "backendport": true,
	"mothershiphost": true, "mothershipport": true, "dashboardapiurl": true,
}

// validateSpinUp checks a spin-up request. internal=true is the watchdog's
// restart path, which re-sends the args it recorded and may therefore carry
// the -DashboardDeploymentId the backend minted earlier. Returns "" when OK,
// otherwise a human-readable reason.
func validateSpinUp(box, mp, region, args, name *string, internal bool) string {
	if v := deref(mp); v != "" && (!spinMapRe.MatchString(v) || strings.Contains(v, "..")) {
		return "map must match [A-Za-z0-9_./-]{1,128} and must not contain .."
	}
	if v := deref(region); v != "" && !spinRegionRe.MatchString(v) {
		return "region must match [A-Za-z0-9_-]{1,32}"
	}
	if v := deref(box); v != "" && !spinBoxRe.MatchString(v) {
		return "box must match [A-Za-z0-9_.-]{1,64}"
	}
	if v := deref(name); strings.TrimSpace(v) != "" && (utf8.RuneCountInString(v) > 64 || !spinNameRe.MatchString(v)) {
		return "name must be at most 64 letters, digits, spaces or _.,'!?&()#:+-"
	}
	a := deref(args)
	if len(a) > maxSpinArgsLen {
		return fmt.Sprintf("args longer than %d characters", maxSpinArgsLen)
	}
	toks := strings.Fields(a)
	if len(toks) > maxSpinArgs {
		return fmt.Sprintf("more than %d args", maxSpinArgs)
	}
	for _, t := range toks {
		if !spinArgRe.MatchString(t) {
			return fmt.Sprintf("arg %s is not a plain -Flag or -Key=Value token", truncate(t, 40))
		}
		flag, _, hasValue := strings.Cut(t[1:], "=")
		lf := strings.ToLower(flag)
		if internal && lf == "dashboarddeploymentid" {
			continue
		}
		if deniedSpinFlags[lf] || strings.HasSuffix(lf, "ini") || (lf == "log" && hasValue) {
			return fmt.Sprintf("arg -%s is not allowed in a spin-up", flag)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
