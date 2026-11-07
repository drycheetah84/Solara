// Solara addition: read-through cache for Axiom's sandbox content bucket.
//
// The game server fetches sandbox content (ScrapRun trap bundles, sandbox projects, configs) from
// https://a2-sandboxprojects.s3.us-east-2.amazonaws.com/<kind>/v1/<id>/<file>, e.g.
// configs/v1/065BB1094A429390370B6B9AB1B4CB4D/config.json ("Frenzy Fest", the Clubhouse ScrapRun trap bundle).
// Its built-in libcurl certificate store cannot verify Amazon's current certificate ("SSL certificate problem:
// unable to get local issuer certificate", libcurl error 60), so every such fetch fails: no trap bundle -> the
// ScrapRun trap manager spawns no traps, and traps vanish after a round.
//
// The game-server DLL rewrites those URLs to http://<station backend>/sandboxprojects/<same path>
// ([CURLREDIRECT]). This handler fetches the object from S3 (Go's TLS verifies it fine), keeps a copy under
// <working dir>/sandbox_cache, and serves the copy from then on -- so the content keeps working even if the bucket
// is ever taken down. Only successful (200) responses are cached; failures are passed through and retried.
package ares

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"solara/internal/hosting"
	"solara/internal/logging"
)

const sandboxUpstream = "https://a2-sandboxprojects.s3.us-east-2.amazonaws.com/"

var (
	sandboxSegRe  = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,128}$`)
	sandboxClient = &http.Client{Timeout: 20 * time.Second}
	sandboxMu     sync.Mutex // serializes cache writes per process (content is small and rarely fetched)
)

func sandboxCacheDir() string { return filepath.Join(a2CurrentDir(), "sandbox_cache") }

// sandboxPath validates the captured segments and joins them ("" = rejected).
func sandboxPath(c *hosting.Context, names ...string) string {
	segs := make([]string, 0, len(names))
	for _, n := range names {
		v := c.Param(n)
		if v == "" {
			break
		}
		if !sandboxSegRe.MatchString(v) || v == "." || v == ".." {
			return ""
		}
		segs = append(segs, v)
	}
	if len(segs) < 2 {
		return ""
	}
	return strings.Join(segs, "/")
}

func (s *A2StationDbServer) serveSandbox(rel string) hosting.Result {
	if rel == "" {
		return hosting.BadRequest("bad path")
	}
	local := filepath.Join(sandboxCacheDir(), filepath.FromSlash(rel))
	if b, err := os.ReadFile(local); err == nil {
		return hosting.Ok(b)
	}
	resp, err := sandboxClient.Get(sandboxUpstream + rel)
	if err != nil {
		logging.Console("[SANDBOX] fetch " + rel + " failed: " + err.Error())
		return hosting.NotFound("upstream fetch failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return hosting.NotFound("upstream read failed")
	}
	if resp.StatusCode != http.StatusOK {
		logging.Console("[SANDBOX] fetch " + rel + " -> HTTP " + resp.Status)
		return hosting.NotFound("not found upstream")
	}
	sandboxMu.Lock()
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err == nil {
		tmp := local + ".tmp"
		if os.WriteFile(tmp, body, 0o644) == nil {
			_ = os.Rename(tmp, local)
		}
	}
	sandboxMu.Unlock()
	logging.Console("[SANDBOX] cached " + rel)
	return hosting.Ok(body)
}

// GetSandboxObject3/4/5 handle GET /sandboxprojects/{a}/{b}/{c}[/{d}[/{e}]] (the router has no catch-all).
func (s *A2StationDbServer) GetSandboxObject3(c *hosting.Context) hosting.Result {
	return s.serveSandbox(sandboxPath(c, "a", "b", "c"))
}
func (s *A2StationDbServer) GetSandboxObject4(c *hosting.Context) hosting.Result {
	return s.serveSandbox(sandboxPath(c, "a", "b", "c", "d"))
}
func (s *A2StationDbServer) GetSandboxObject5(c *hosting.Context) hosting.Result {
	return s.serveSandbox(sandboxPath(c, "a", "b", "c", "d", "e"))
}
