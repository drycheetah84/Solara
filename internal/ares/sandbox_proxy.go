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
	"sort"
	"strings"
	"sync"
	"time"

	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/stj"
)

const sandboxUpstream = "https://a2-sandboxprojects.s3.us-east-2.amazonaws.com/"

var (
	sandboxSegRe  = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,128}$`)
	sandboxClient = &http.Client{Timeout: 20 * time.Second}
	sandboxMu     sync.Mutex // serializes cache writes per process (content is small and rarely fetched)

	// Per-object request stats since this process started (the dashboard's Sandbox tab). The cache itself is the
	// durable record of what has ever been fetched.
	sandboxStatsMu sync.Mutex
	sandboxStats   = map[string]*sandboxStat{}
)

type sandboxStat struct {
	Hits       int
	Failures   int
	LastAt     time.Time
	LastClient string
	LastError  string
}

func sandboxRecord(rel, client, failure string) {
	sandboxStatsMu.Lock()
	defer sandboxStatsMu.Unlock()
	st := sandboxStats[rel]
	if st == nil {
		if len(sandboxStats) > 4096 {
			return
		}
		st = &sandboxStat{}
		sandboxStats[rel] = st
	}
	st.Hits++
	st.LastAt = time.Now()
	st.LastClient = client
	if failure != "" {
		st.Failures++
		st.LastError = failure
	}
}

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

func (s *A2StationDbServer) serveSandbox(c *hosting.Context, rel string) hosting.Result {
	if rel == "" {
		return hosting.BadRequest("bad path")
	}
	client := sandboxClientIP(c)
	local := filepath.Join(sandboxCacheDir(), filepath.FromSlash(rel))
	if b, err := os.ReadFile(local); err == nil {
		sandboxRecord(rel, client, "")
		return hosting.Ok(b)
	}
	resp, err := sandboxClient.Get(sandboxUpstream + rel)
	if err != nil {
		logging.Console("[SANDBOX] fetch " + rel + " failed: " + err.Error())
		sandboxRecord(rel, client, "upstream fetch failed: "+err.Error())
		return hosting.NotFound("upstream fetch failed")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		sandboxRecord(rel, client, "upstream read failed")
		return hosting.NotFound("upstream read failed")
	}
	if resp.StatusCode != http.StatusOK {
		logging.Console("[SANDBOX] fetch " + rel + " -> HTTP " + resp.Status)
		sandboxRecord(rel, client, "S3 answered "+resp.Status)
		return hosting.NotFound("not found upstream")
	}
	sandboxRecord(rel, client, "")
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
	return s.serveSandbox(c, sandboxPath(c, "a", "b", "c"))
}
func (s *A2StationDbServer) GetSandboxObject4(c *hosting.Context) hosting.Result {
	return s.serveSandbox(c, sandboxPath(c, "a", "b", "c", "d"))
}
func (s *A2StationDbServer) GetSandboxObject5(c *hosting.Context) hosting.Result {
	return s.serveSandbox(c, sandboxPath(c, "a", "b", "c", "d", "e"))
}

// sandboxClientIP is the requesting game server's address ("" if unknown; Remote panics on a malformed header).
func sandboxClientIP(c *hosting.Context) (ip string) {
	defer func() {
		if recover() != nil {
			ip = ""
		}
	}()
	if c == nil || c.Request == nil {
		return ""
	}
	return c.Request.Remote()
}

// SandboxObject is one row of the dashboard's Sandbox tab.
type SandboxObject struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Id          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Version     string `json:"version"`
	Bytes       int64  `json:"bytes"`
	CachedAt    string `json:"cached_at"`
	Hits        int    `json:"hits"`
	Failures    int    `json:"failures"`
	LastAt      string `json:"last_at"`
	LastClient  string `json:"last_client"`
	LastError   string `json:"last_error"`
	FailedFetch bool   `json:"failed_fetch"` // requested but never cached (every fetch failed)
}

// sandboxNames reads display names / types from the cached project infos and trap bundles.
func sandboxNames(root string) map[string][2]string {
	out := map[string][2]string{}
	read := func(path string) {
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		doc, err := stj.Parse(trimBOM(b))
		if err != nil || doc.ValueKind() != stj.Object {
			return
		}
		name, typ := "", ""
		if v, ok := doc.TryGetProperty("displayName"); ok && v.ValueKind() == stj.String {
			name = *v.GetString()
		}
		if v, ok := doc.TryGetProperty("type"); ok && v.ValueKind() == stj.String {
			typ = *v.GetString()
		}
		id := strings.ToUpper(filepath.Base(filepath.Dir(path)))
		if name != "" {
			out[id] = [2]string{name, typ}
		}
	}
	for _, sub := range []string{"projects/v1", "configs/v1"} {
		dirs, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(sub)))
		for _, d := range dirs {
			f := "info.json"
			if sub == "configs/v1" {
				f = "config.json"
			}
			read(filepath.Join(root, filepath.FromSlash(sub), d.Name(), f))
		}
	}
	return out
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// SandboxObjects lists every cached sandbox object plus anything requested that never cached.
func SandboxObjects() []SandboxObject {
	root := sandboxCacheDir()
	names := sandboxNames(root)
	sandboxStatsMu.Lock()
	stats := make(map[string]sandboxStat, len(sandboxStats))
	for k, v := range sandboxStats {
		stats[k] = *v
	}
	sandboxStatsMu.Unlock()
	fmtT := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	}
	row := func(rel string) SandboxObject {
		parts := strings.Split(rel, "/")
		o := SandboxObject{Path: rel}
		if len(parts) >= 2 {
			o.Kind = parts[0] + "/" + parts[1]
		}
		if len(parts) >= 3 {
			o.Id = strings.ToUpper(parts[2])
		}
		if len(parts) >= 4 {
			o.Version = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(parts[len(parts)-1], ".aarel"), ".aaprj"), ".aaqst"), ".json")
			if o.Version == "info" || o.Version == "config" {
				o.Version = ""
			}
		}
		if n, ok := names[o.Id]; ok {
			o.Name, o.Type = n[0], n[1]
		}
		if o.Kind == "configs/v1" && o.Type == "" {
			o.Type = "Trap bundle"
		}
		if st, ok := stats[rel]; ok {
			o.Hits, o.Failures, o.LastAt, o.LastClient, o.LastError = st.Hits, st.Failures, fmtT(st.LastAt), st.LastClient, st.LastError
		}
		return o
	}
	out := []SandboxObject{}
	seen := map[string]bool{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".tmp") {
			return nil
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		o := row(rel)
		if fi, e := d.Info(); e == nil {
			o.Bytes = fi.Size()
			o.CachedAt = fmtT(fi.ModTime())
		}
		seen[rel] = true
		out = append(out, o)
		return nil
	})
	for rel := range stats {
		if !seen[rel] {
			o := row(rel)
			o.FailedFetch = true
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FailedFetch != out[j].FailedFetch {
			return out[i].FailedFetch
		}
		if !strings.EqualFold(out[i].Name, out[j].Name) {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// GetSandboxObjects is GET /api/sandbox/objects (master key only: not in the scoped-token route table).
func (s *AresDashboardServer) GetSandboxObjects(c *hosting.Context) hosting.Result {
	objs := SandboxObjects()
	var total int64
	for _, o := range objs {
		total += o.Bytes
	}
	return hosting.Ok(stj.O("upstream", sandboxUpstream, "cache_dir", sandboxCacheDir(), "total_bytes", total, "objects", objs))
}
