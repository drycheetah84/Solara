package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"solara/internal/pak"
)

const (
	testToken = "test-token"
	testPort  = 54321
	engineIni = "[/Script/Engine.Engine]\r\nbSmoothFrameRate=True\r\n"
	gameIni   = "[/Script/EngineSettings.GeneralProjectSettings]\r\nProjectID=1\r\n"
)

func newTestServer() *server {
	ui := fstest.MapFS{
		"index.html":    {Data: []byte("<html>ui</html>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	return newServer(testToken, testPort, ui)
}

// gameTree builds <root>/A2/Content/Paks/A2-Windows.pak plus decoys.
func gameTree(t *testing.T, withOriginalDecoy bool) (root, pakPath string) {
	t.Helper()
	root = t.TempDir()
	paks := filepath.Join(root, "A2", "Content", "Paks")
	if err := os.MkdirAll(paks, 0o755); err != nil {
		t.Fatal(err)
	}
	pakPath = filepath.Join(paks, "A2-Windows.pak")
	files := []pak.NewFile{
		{Name: "A2/A2.uproject", Data: []byte("{}")},
		{Name: "A2/Config/DefaultInput.ini", Data: []byte("[X]\r\nA=1\r\n")},
		{Name: "A2/Config/DefaultGame.ini", Data: []byte(gameIni)},
		{Name: "A2/Config/DefaultEngine.ini", Data: []byte(engineIni)},
		{Name: "Engine/Plugins/P/Config/BaseP.ini", Data: []byte("[P]\r\n")},
		{Name: "A2/Content/Map.umap", Data: bytes.Repeat([]byte{9}, 3000)},
	}
	if err := pak.WriteNew(pakPath, "../../../", files, &pak.WriteOptions{PhysicalOrder: []int{5, 0, 3, 1, 2, 4}}); err != nil {
		t.Fatal(err)
	}
	decoys := []string{"A2-Windows.pak.bak-20260101-000000", "A2-Windows.pak.solara-tmp", "global.utoc"}
	if withOriginalDecoy {
		decoys = append(decoys, "A2-Windows.pak.original")
	}
	for _, decoy := range decoys {
		os.WriteFile(filepath.Join(paks, decoy), []byte("x"), 0o644)
	}
	return root, pakPath
}

type resp struct {
	code int
	body map[string]any
	raw  string
}

func do(t *testing.T, s *server, method, target string, body any, mutate ...func(*http.Request)) resp {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, rd)
	req.Host = "127.0.0.1:54321"
	req.Header.Set("X-Session-Token", testToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, m := range mutate {
		m(req)
	}
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, req)
	r := resp{code: w.Code, raw: w.Body.String()}
	json.Unmarshal(w.Body.Bytes(), &r.body)
	return r
}

func TestSecurity(t *testing.T) {
	s := newTestServer()
	if r := do(t, s, "POST", "/api/validate", map[string]string{"text": ""}); r.code != 200 {
		t.Fatalf("valid request: %d %s", r.code, r.raw)
	}
	if r := do(t, s, "POST", "/api/validate", map[string]string{"text": ""}, func(q *http.Request) { q.Header.Del("X-Session-Token") }); r.code != 401 {
		t.Fatalf("missing token: %d", r.code)
	}
	if r := do(t, s, "POST", "/api/validate", map[string]string{"text": ""}, func(q *http.Request) { q.Header.Set("X-Session-Token", "nope") }); r.code != 401 {
		t.Fatalf("bad token: %d", r.code)
	}
	for _, host := range []string{"evil.example:54321", "127.0.0.1:1", "localhost", "127.0.0.1"} {
		if r := do(t, s, "POST", "/api/validate", map[string]string{"text": ""}, func(q *http.Request) { q.Host = host }); r.code != 403 {
			t.Fatalf("host %s: %d", host, r.code)
		}
	}
	if r := do(t, s, "GET", "/", nil, func(q *http.Request) { q.Host = "evil.example:54321" }); r.code != 403 {
		t.Fatalf("UI with bad host: %d", r.code)
	}
	if r := do(t, s, "POST", "/api/validate", map[string]string{"text": ""}, func(q *http.Request) { q.Host = "localhost:54321" }); r.code != 200 {
		t.Fatalf("localhost host: %d", r.code)
	}
	if r := do(t, s, "POST", "/api/validate", map[string]string{"text": ""}, func(q *http.Request) { q.Header.Set("Content-Type", "text/plain") }); r.code != 415 {
		t.Fatalf("non-JSON content type: %d", r.code)
	}
	big := map[string]string{"text": strings.Repeat("a", maxBody+10)}
	if r := do(t, s, "POST", "/api/validate", big); r.code != 413 {
		t.Fatalf("oversized body: %d", r.code)
	}
	r := do(t, s, "GET", "/api/nothing", nil)
	if r.code != 404 || r.body["error"] == nil {
		t.Fatalf("unknown api: %d %s", r.code, r.raw)
	}
	// no CORS headers
	req := httptest.NewRequest("OPTIONS", "/api/scan", nil)
	req.Host = "127.0.0.1:54321"
	req.Header.Set("Origin", "http://evil.example")
	w := httptest.NewRecorder()
	s.handler().ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS header present")
	}
}

func TestUI(t *testing.T) {
	s := newTestServer()
	for path, want := range map[string]string{"/": "<html>ui</html>", "/some/route": "<html>ui</html>", "/assets/app.js": "console.log(1)"} {
		r := do(t, s, "GET", path, nil, func(q *http.Request) { q.Header.Del("X-Session-Token") })
		if r.code != 200 || r.raw != want {
			t.Fatalf("%s: %d %q", path, r.code, r.raw)
		}
	}
	// embedded placeholder exists
	if _, err := uiFS().Open("index.html"); err != nil {
		t.Fatal(err)
	}
}

func TestScanVariants(t *testing.T) {
	s := newTestServer()
	root, pakPath := gameTree(t, true)
	paksDir := filepath.Dir(pakPath)
	inputs := []string{root, filepath.Join(root, "A2"), filepath.Join(root, "A2", "Content"), paksDir, pakPath, "  \"" + root + "\"  "}
	for _, in := range inputs {
		r := do(t, s, "POST", "/api/scan", map[string]string{"path": in})
		if r.code != 200 {
			t.Fatalf("%s: %d %s", in, r.code, r.raw)
		}
		var sr scanResponse
		json.Unmarshal([]byte(r.raw), &sr)
		if sr.Root != paksDir || len(sr.Paks) != 1 {
			t.Fatalf("%s: root %q paks %d", in, sr.Root, len(sr.Paks))
		}
		p := sr.Paks[0]
		if !p.Supported || p.Version != 8 || p.Entries != 6 || p.Name != "A2-Windows.pak" {
			t.Fatalf("%s: %+v", in, p)
		}
		want := []string{"A2/Config/DefaultEngine.ini", "A2/Config/DefaultGame.ini", "A2/Config/DefaultInput.ini", "Engine/Plugins/P/Config/BaseP.ini"}
		if strings.Join(p.ConfigFiles, ",") != strings.Join(want, ",") {
			t.Fatalf("config files %v", p.ConfigFiles)
		}
	}
	if r := do(t, s, "POST", "/api/scan", map[string]string{"path": filepath.Join(root, "missing")}); r.code != 400 {
		t.Fatalf("missing path: %d", r.code)
	}
	empty := t.TempDir()
	if r := do(t, s, "POST", "/api/scan", map[string]string{"path": empty}); r.code != 400 {
		t.Fatalf("empty dir: %d", r.code)
	}
	// unsupported pak is reported, not fatal
	b, _ := os.ReadFile(pakPath)
	b[len(b)-221+21] = 11
	os.WriteFile(filepath.Join(paksDir, "Other.pak"), b, 0o644)
	r := do(t, s, "POST", "/api/scan", map[string]string{"path": root})
	var sr scanResponse
	json.Unmarshal([]byte(r.raw), &sr)
	if len(sr.Paks) != 2 || sr.Paks[1].Supported || sr.Paks[1].Version != 11 || sr.Paks[1].Error == "" {
		t.Fatalf("unsupported pak: %+v", sr.Paks)
	}
}

func sha1hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func waitJob(t *testing.T, s *server, id string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		r := do(t, s, "GET", "/api/jobs/"+id, nil)
		if r.code != 200 {
			t.Fatalf("job status: %d %s", r.code, r.raw)
		}
		if r.body["state"] != "running" {
			return r.body
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return nil
}

func TestFileSaveRestore(t *testing.T) {
	s := newTestServer()
	_, pakPath := gameTree(t, false)
	orig, _ := os.ReadFile(pakPath)
	const name = "A2/Config/DefaultGame.ini"
	q := "/api/file?pak=" + pakPath + "&name=" + name
	r := do(t, s, "GET", q, nil)
	if r.code != 200 {
		t.Fatalf("file: %d %s", r.code, r.raw)
	}
	text := r.body["text"].(string)
	if strings.Contains(text, "\r") || r.body["lineEnding"] != "CRLF" || r.body["bom"] != false || r.body["sha1"] != sha1hex(gameIni) {
		t.Fatalf("file response: %s", r.raw)
	}
	if secs := r.body["sections"].([]any); len(secs) != 1 {
		t.Fatalf("sections: %v", secs)
	}
	if r := do(t, s, "GET", "/api/file?pak="+pakPath+"&name=A2/A2.uproject", nil); r.code != 400 {
		t.Fatalf("non-ini: %d", r.code)
	}
	if r := do(t, s, "GET", "/api/file?pak="+pakPath+"&name=A2/Config/Nope.ini", nil); r.code != 404 {
		t.Fatalf("missing entry: %d", r.code)
	}

	save := func(files map[string]string, expect map[string]string) resp {
		return do(t, s, "POST", "/api/save", map[string]any{"pak": pakPath, "files": files, "expectSha1": expect})
	}
	sha := map[string]string{name: r.body["sha1"].(string)}
	// unchanged text
	if r := save(map[string]string{name: text}, sha); r.code != 400 || r.body["error"] != "no changes" {
		t.Fatalf("no changes: %d %s", r.code, r.raw)
	}
	// stale sha
	if r := save(map[string]string{name: text + "; x\n"}, map[string]string{name: sha1hex("other")}); r.code != 409 {
		t.Fatalf("stale sha: %d %s", r.code, r.raw)
	}
	// validation error
	r = save(map[string]string{name: text + "Bad=\x01\n"}, sha)
	if r.code != 422 || len(r.body["issues"].([]any)) == 0 {
		t.Fatalf("validation: %d %s", r.code, r.raw)
	}
	// real save
	edited := text + "; edited\n"
	r = save(map[string]string{name: edited}, sha)
	if r.code != 202 {
		t.Fatalf("save: %d %s", r.code, r.raw)
	}
	st := waitJob(t, s, r.body["jobId"].(string))
	if st["state"] != "done" || st["progress"].(float64) != 1 {
		t.Fatalf("job: %v", st)
	}
	res := st["result"].(map[string]any)
	if ch := res["changed"].([]any); len(ch) != 1 || ch[0] != name {
		t.Fatalf("changed: %v", ch)
	}
	r = do(t, s, "GET", q, nil)
	if r.body["text"] != edited || r.body["lineEnding"] != "CRLF" {
		t.Fatalf("after save: %s", r.raw)
	}
	p, _ := pak.Open(pakPath)
	if raw, _ := p.ReadEntry(name); string(raw) != gameIni+"; edited\r\n" {
		t.Fatalf("stored bytes %q", raw)
	}
	// old sha is now stale
	if r := save(map[string]string{name: edited + "; more\n"}, sha); r.code != 409 {
		t.Fatalf("stale after save: %d", r.code)
	}
	// backups
	r = do(t, s, "GET", "/api/backups?pak="+pakPath, nil)
	bs := r.body["backups"].([]any)
	if len(bs) != 3 { // .original, the save's backup, the decoy-named bak file
		t.Fatalf("backups: %s", r.raw)
	}
	first := bs[0].(map[string]any)
	if first["kind"] != "original" {
		t.Fatalf("first backup: %v", first)
	}
	// restore rejects unknown files
	if r := do(t, s, "POST", "/api/restore", map[string]string{"pak": pakPath, "backup": pakPath}); r.code != 400 {
		t.Fatalf("restore arbitrary: %d", r.code)
	}
	r = do(t, s, "POST", "/api/restore", map[string]string{"pak": pakPath, "backup": first["path"].(string)})
	if r.code != 202 {
		t.Fatalf("restore: %d %s", r.code, r.raw)
	}
	if st := waitJob(t, s, r.body["jobId"].(string)); st["state"] != "done" {
		t.Fatalf("restore job: %v", st)
	}
	now, _ := os.ReadFile(pakPath)
	if !bytes.Equal(now, orig) {
		t.Fatal("restore did not bring back the original pak")
	}
	// restoring the 1-byte decoy fails verification and leaves the pak intact
	var decoy string
	for _, b := range bs {
		if strings.HasSuffix(b.(map[string]any)["name"].(string), "20260101-000000") {
			decoy = b.(map[string]any)["path"].(string)
		}
	}
	r = do(t, s, "POST", "/api/restore", map[string]string{"pak": pakPath, "backup": decoy})
	if st := waitJob(t, s, r.body["jobId"].(string)); st["state"] != "error" || st["error"] == "" {
		t.Fatalf("corrupt backup restore: %v", st)
	}
	if now, _ := os.ReadFile(pakPath); !bytes.Equal(now, orig) {
		t.Fatal("pak changed by failed restore")
	}
}
