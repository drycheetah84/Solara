package main

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"solara/internal/pak"
)

// fakeOodle stands in for the Oodle DLL (flate under the "Oodle" name).
type fakeOodle struct{}

func (fakeOodle) CanCompress() bool { return true }

func (fakeOodle) Compress(method string, raw []byte) ([]byte, error) {
	var b bytes.Buffer
	w, _ := flate.NewWriter(&b, flate.BestCompression)
	w.Write(raw)
	w.Close()
	return b.Bytes(), nil
}

func (fakeOodle) Decompress(method string, comp []byte, rawLen int) ([]byte, error) {
	out, err := io.ReadAll(flate.NewReader(bytes.NewReader(comp)))
	if err == nil && len(out) != rawLen {
		err = fmt.Errorf("got %d bytes, want %d", len(out), rawLen)
	}
	return out, err
}

var bigEngineIni = strings.Repeat("[/Script/Engine.Engine]\r\nbSmoothFrameRate=True\r\n", 400)

func questTree(t *testing.T) (root, pakPath string) {
	t.Helper()
	root = t.TempDir()
	paks := filepath.Join(root, "A2", "Content", "Paks")
	os.MkdirAll(paks, 0o755)
	pakPath = filepath.Join(paks, "A2-Android_ASTC.pak")
	files := []pak.NewFile{
		{Name: "A2/A2.uproject", Data: []byte("{}")},
		{Name: "A2/Config/DefaultEngine.ini", Data: []byte(bigEngineIni), CompressionMethod: 1, CompressionBlockSize: 1024},
		{Name: "A2/Config/DefaultGame.ini", Data: []byte(gameIni), CompressionMethod: 1},
		{Name: "A2/Config/DefaultInput.ini", Data: []byte("[X]\r\nA=1\r\n")},
		{Name: "A2/Content/Map.umap", Data: bytes.Repeat([]byte{9}, 3000), CompressionMethod: 1},
	}
	err := pak.WriteNew(pakPath, "../../../", files, &pak.WriteOptions{
		Version: 11, Compressor: fakeOodle{}, CompressionNames: []string{"Oodle"}, PhysicalOrder: []int{4, 0, 2, 1, 3}, PathHashSeed: 0x1234,
	})
	if err != nil {
		t.Fatal(err)
	}
	return root, pakPath
}

func oodleTestServer(t *testing.T, settingsPath string) *server {
	s := newTestServer()
	s.settingsPath = settingsPath
	s.loadOodle = func(p string) (oodleCodec, error) {
		if _, err := os.Stat(p); err != nil || filepath.Base(p) != "oodle.dll" {
			return nil, errors.New("does not export OodleLZ_Decompress (not an Oodle DLL)")
		}
		return fakeOodle{}, nil
	}
	s.reloadOodle()
	return s
}

func TestQuestPakOodleFlow(t *testing.T) {
	t.Setenv(pak.OodleEnvVar, "")
	cfg := t.TempDir()
	settingsPath := filepath.Join(cfg, "pak-editor", "settings.json")
	s := oodleTestServer(t, settingsPath)
	root, pakPath := questTree(t)

	if r := do(t, s, "GET", "/api/settings", nil); r.code != 200 || r.body["oodleAvailable"] != false {
		t.Fatalf("settings: %d %s", r.code, r.raw)
	}
	r := do(t, s, "POST", "/api/scan", map[string]string{"path": root})
	if r.code != 200 {
		t.Fatalf("scan: %s", r.raw)
	}
	p0 := r.body["paks"].([]any)[0].(map[string]any)
	if p0["version"] != float64(11) || p0["supported"] != true || p0["oodleRequired"] != true || p0["oodleAvailable"] != false ||
		fmt.Sprint(p0["compressionMethods"]) != "[Oodle]" || len(p0["configFiles"].([]any)) != 3 {
		t.Fatalf("scan pak: %v", p0)
	}
	const game = "A2/Config/DefaultGame.ini"
	q := "/api/file?pak=" + pakPath + "&name=" + game
	if r := do(t, s, "GET", q, nil); r.code != 422 || r.body["code"] != "oodle_required" {
		t.Fatalf("file without Oodle: %d %s", r.code, r.raw)
	}
	// uncompressed entries of a v11 pak need no Oodle
	if r := do(t, s, "GET", "/api/file?pak="+pakPath+"&name=A2/Config/DefaultInput.ini", nil); r.code != 200 || r.body["compression"] != "" {
		t.Fatalf("uncompressed v11 entry: %d %s", r.code, r.raw)
	}

	bad := filepath.Join(cfg, "bad.dll")
	os.WriteFile(bad, []byte("x"), 0o644)
	if r := do(t, s, "POST", "/api/settings", map[string]string{"oodleDll": bad}); r.code != 400 || r.body["code"] != "oodle_invalid" {
		t.Fatalf("invalid dll: %d %s", r.code, r.raw)
	}
	if _, err := os.Stat(settingsPath); err == nil {
		t.Fatal("invalid DLL was persisted")
	}
	good := filepath.Join(cfg, "oodle.dll")
	os.WriteFile(good, []byte("x"), 0o644)
	r = do(t, s, "POST", "/api/settings", map[string]string{"oodleDll": "\"" + good + "\""})
	if r.code != 200 || r.body["oodleAvailable"] != true || r.body["oodleDll"] != good || r.body["oodleSource"] != "settings" {
		t.Fatalf("valid dll: %d %s", r.code, r.raw)
	}
	if b, _ := os.ReadFile(settingsPath); !strings.Contains(string(b), "oodle.dll") {
		t.Fatalf("settings not persisted: %q", b)
	}
	// a new server picks the persisted setting up
	if s2 := oodleTestServer(t, settingsPath); s2.settingsSnapshot().OodlePath != good {
		t.Fatal("persisted setting not loaded")
	}
	r = do(t, s, "POST", "/api/scan", map[string]string{"path": pakPath})
	if r.body["paks"].([]any)[0].(map[string]any)["oodleAvailable"] != true {
		t.Fatalf("scan after settings: %s", r.raw)
	}
	r = do(t, s, "GET", q, nil)
	if r.code != 200 || r.body["compression"] != "Oodle" || r.body["lineEnding"] != "CRLF" {
		t.Fatalf("file with Oodle: %d %s", r.code, r.raw)
	}
	text := r.body["text"].(string)

	save := func(name, text, sha string, recompress bool) map[string]any {
		r := do(t, s, "POST", "/api/save", map[string]any{"pak": pakPath, "files": map[string]string{name: text}, "expectSha1": map[string]string{name: sha}, "recompress": recompress})
		if r.code != 202 {
			t.Fatalf("save %s: %d %s", name, r.code, r.raw)
		}
		st := waitJob(t, s, r.body["jobId"].(string))
		if st["state"] != "done" {
			t.Fatalf("save job: %v", st)
		}
		return st["result"].(map[string]any)
	}
	res := save(game, text+"; recompressed\n", r.body["sha1"].(string), true)
	if fmt.Sprint(res["recompressed"]) != "["+game+"]" {
		t.Fatalf("recompressed: %v", res)
	}
	r = do(t, s, "GET", q, nil)
	if r.body["text"] != text+"; recompressed\n" || r.body["compression"] != "Oodle" {
		t.Fatalf("after recompressed save: %s", r.raw)
	}
	const engine = "A2/Config/DefaultEngine.ini"
	re := do(t, s, "GET", "/api/file?pak="+pakPath+"&name="+engine, nil)
	res = save(engine, re.body["text"].(string)+"; plain\n", re.body["sha1"].(string), false)
	if len(res["recompressed"].([]any)) != 0 {
		t.Fatalf("uncompressed save: %v", res)
	}
	re = do(t, s, "GET", "/api/file?pak="+pakPath+"&name="+engine, nil)
	if re.body["compression"] != "" || !strings.HasSuffix(re.body["text"].(string), "; plain\n") {
		t.Fatalf("after plain save: %s", re.raw)
	}
	p, _ := pak.Open(pakPath)
	if _, err := pak.Verify(pakPath, &pak.VerifyOptions{Decompressor: fakeOodle{}}); err != nil || p.Footer.Version != 11 {
		t.Fatal(err)
	}

	// clearing the setting: Oodle unavailable again, reads report oodle_required
	if r := do(t, s, "POST", "/api/settings", map[string]string{"oodleDll": ""}); r.code != 200 || r.body["oodleAvailable"] != false {
		t.Fatalf("clear: %d %s", r.code, r.raw)
	}
	if r := do(t, s, "GET", q, nil); r.body["code"] != "oodle_required" {
		t.Fatalf("after clear: %s", r.raw)
	}
	rr := do(t, s, "POST", "/api/save", map[string]any{"pak": pakPath, "files": map[string]string{game: "x"}, "expectSha1": map[string]string{game: "0"}, "recompress": true})
	if rr.code != 422 || rr.body["code"] != "oodle_required" {
		t.Fatalf("recompress without Oodle: %d %s", rr.code, rr.raw)
	}
	// recompress is refused for v8 paks
	_, v8 := gameTree(t, false)
	rr = do(t, s, "POST", "/api/save", map[string]any{"pak": v8, "files": map[string]string{game: "x"}, "expectSha1": map[string]string{game: "0"}, "recompress": true})
	if rr.code != 400 {
		t.Fatalf("recompress on v8: %d %s", rr.code, rr.raw)
	}
}
