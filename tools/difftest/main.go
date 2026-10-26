// Command difftest replays scenario files through Solara in-process (the
// counterpart of tools/difftest-oracle, which replays them through the
// original .NET servers) and compares the two result streams.
//
//	go run ./tools/difftest run <scenario.json> <workdir>         > solara.jsonl
//	go run ./tools/difftest compare <original.jsonl> <solara.jsonl>
//	go run ./tools/difftest all <scenario-dir> <oracle-dll>        (run both + compare)
//
// Normalisation (documented in VALIDATION_REPORT.md) replaces only values that
// are nondeterministic by construction: GUIDs, ObjectIds, JWTs, timestamps,
// epoch numbers, random hex/base64url tokens, EOS correlation ids.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"solara/internal/ares"
	"solara/internal/docdb"
	"solara/internal/eos"
	"solara/internal/halcyonsocket"
	"solara/internal/host"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/mothership"
	"solara/internal/secrets"
)

type step struct {
	Server  string            `json:"server"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    *string           `json:"body"`
	Capture map[string]string `json:"capture"`
}

type scenario struct {
	Name  string `json:"name"`
	Steps []step `json:"steps"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: difftest run|compare|all ...")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "run":
		os.Exit(run(os.Args[2], os.Args[3], os.Stdout))
	case "compare":
		os.Exit(compareFiles(os.Args[2], os.Args[3]))
	case "all":
		os.Exit(all(os.Args[2], os.Args[3]))
	}
	os.Exit(2)
}

func run(scenarioPath, workdir string, out *os.File) int {
	raw, err := os.ReadFile(scenarioPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var sc struct {
		Name  string            `json:"name"`
		Steps []json.RawMessage `json:"steps"`
	}
	if err := json.Unmarshal(raw, &sc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if secrets.FilePath == "" {
		secrets.FilePath, _ = filepath.Abs(filepath.Join("config", "legacy-secrets.env"))
	}
	abs, _ := filepath.Abs(workdir)
	_ = os.MkdirAll(abs, 0o755)
	_ = os.Chdir(abs)
	logging.LogDir = filepath.Join(abs, "logs")
	hosting.DisableListen = true

	path := filepath.Join(abs, "difftest"+docdb.FileSuffix)
	_ = os.Remove(path)
	_ = os.Remove(path + ".meta")
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 2, Name: "difftest", Path: path})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ares.Database, mothership.Database = db, db
	host.PreRegisterCollections(db)
	halcyonsocket.TakeDeploymentOffline = host.TakeDeploymentOffline

	servers := map[string]*hosting.Server{}
	get := func(name string) *hosting.Server {
		if s, ok := servers[name]; ok {
			return s
		}
		var s *hosting.Server
		var err error
		switch name {
		case "A2StationDbServer":
			var x *ares.A2StationDbServer
			if x, err = ares.NewA2StationDbServer(); err == nil {
				s = x.HTTP
			}
		case "AresDashboardServer":
			var x *ares.AresDashboardServer
			if x, err = ares.NewAresDashboardServer(); err == nil {
				s = x.HTTP
			}
		case "MothershipServer":
			var x *mothership.MothershipServer
			if x, err = mothership.NewMothershipServer(); err == nil {
				s = x.HTTP
			}
		case "EosGatewayServer":
			var x *eos.EosGatewayServer
			if x, err = eos.NewEosGatewayServer(); err == nil {
				s = x.HTTP
			}
		case "EosWsServer":
			var x *eos.EosWsServer
			if x, err = eos.NewEosWsServer(); err == nil {
				s = x.HTTP
			}
		case "HalcyonSocketServer":
			var x *halcyonsocket.HalcyonSocketServer
			if x, err = halcyonsocket.NewHalcyonSocketServer(); err == nil {
				s = x.HTTP
			}
		default:
			err = fmt.Errorf("unknown server %s", name)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "construct", name, err)
			os.Exit(1)
		}
		servers[name] = s
		return s
	}

	enc := json.NewEncoder(out)
	for i, rawStep := range sc.Steps {
		var st step
		if err := json.Unmarshal(applySubst(rawStep), &st); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		s := get(st.Server)
		body := ""
		if st.Body != nil {
			body = *st.Body
		}
		req := httptest.NewRequest(st.Method, "http://localhost"+st.Path, strings.NewReader(body))
		req.Header.Del("Host")
		for k, v := range st.Headers {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		hdrs := map[string]string{}
		for k, v := range rec.Header() {
			switch k {
			case "Content-Type", "Content-Length":
				continue
			}
			hdrs[k] = strings.Join(v, ",")
		}
		res := map[string]any{"step": i, "status": rec.Code, "contentType": rec.Header().Get("Content-Type"),
			"headers": hdrs, "body": rec.Body.String()}
		_ = enc.Encode(res)
		if st.Capture != nil {
			capture(st.Capture, rec.Body.String())
		}
	}
	return 0
}

// --- comparison --------------------------------------------------------------

var norms = []struct {
	re  *regexp.Regexp
	rep string
}{
	{regexp.MustCompile(`"n":"[A-Za-z0-9_-]{100,}"`), `"n":"<rsa-modulus>"`},
	// HOST-005: .NET appends the stack trace to exception text in 500 pages.
	{regexp.MustCompile(`(?s)\r?\n   at .*?(</p>)`), "$1"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`), "<jwt>"},
	{regexp.MustCompile(`EOS-[A-Za-z0-9_-]{8,}`), "<eos-correlation>"},
	{regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), "<guid>"},
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})?`), "<timestamp>"},
	{regexp.MustCompile(`\b[0-9a-f]{24}\b`), "<objectid>"},
	{regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`), "<hex>"},
	{regexp.MustCompile(`\b[0-9a-f]{16}\b`), "<id16>"},
	{regexp.MustCompile(`"(Attestation)?Nonce":"[A-Za-z0-9_-]{16,}"`), `"Nonce":"<nonce>"`},
	{regexp.MustCompile(`Server_[0-9a-f]{6}\b`), "Server_<rand6>"},
	// TURN credentials: password = HMAC over a username that embeds the current time
	// base64 of a 20-byte HMAC; STJ escapes "+" as a 6-char unicode escape
	{regexp.MustCompile(`"password":"[^"]{27,45}="`), `"password":"<turn-hmac>"`},
	{regexp.MustCompile(`\b1[6-9]\d{8}(\d{3})?\b`), "<epoch>"},
}

func normalize(s string) string {
	for _, n := range norms {
		s = n.re.ReplaceAllString(s, n.rep)
	}
	return s
}

type result struct {
	Step        int               `json:"step"`
	Status      int               `json:"status"`
	ContentType string            `json:"contentType"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
}

func readResults(path string) ([]result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []result
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var r result
		if err := json.Unmarshal(line, &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// bodiesEqual compares bodies textually after normalisation; JSON bodies are
// additionally compared structurally so that object key order (not part of
// the contract for Dictionary<,> members) does not count as a difference.
func bodiesEqual(a, b string) (bool, string) {
	na, nb := normalize(a), normalize(b)
	if na == nb {
		return true, ""
	}
	var ja, jb any
	if json.Unmarshal([]byte(na), &ja) == nil && json.Unmarshal([]byte(nb), &jb) == nil {
		ca, _ := json.Marshal(ja)
		cb, _ := json.Marshal(jb)
		if bytes.Equal(ca, cb) {
			return true, "key-order"
		}
	}
	return false, ""
}

func compareFiles(a, b string) int {
	ra, err := readResults(a)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	rb, err := readResults(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	diffs, keyOrder := 0, 0
	n := len(ra)
	if len(rb) != n {
		fmt.Printf("  step count differs: original %d, solara %d\n", len(ra), len(rb))
		diffs++
		if len(rb) < n {
			n = len(rb)
		}
	}
	for i := 0; i < n; i++ {
		x, y := ra[i], rb[i]
		var why []string
		if x.Status != y.Status {
			why = append(why, fmt.Sprintf("status %d != %d", x.Status, y.Status))
		}
		if x.ContentType != y.ContentType {
			why = append(why, fmt.Sprintf("content-type %q != %q", x.ContentType, y.ContentType))
		}
		if ok, how := bodiesEqual(x.Body, y.Body); !ok {
			na, nb := normalize(x.Body), normalize(y.Body)
			k := 0
			for k < len(na) && k < len(nb) && na[k] == nb[k] {
				k++
			}
			from := max(0, k-120)
			why = append(why, fmt.Sprintf("body (first difference at byte %d)\n      original: ...%.360s\n      solara:   ...%.360s", k, na[from:], nb[from:]))
		} else if how == "key-order" {
			keyOrder++
		}
		var hk []string
		for k, v := range x.Headers {
			if normalize(y.Headers[k]) != normalize(v) {
				hk = append(hk, k)
			}
		}
		for k := range y.Headers {
			if _, ok := x.Headers[k]; !ok {
				hk = append(hk, k)
			}
		}
		sort.Strings(hk)
		if len(hk) > 0 {
			why = append(why, "headers differ: "+strings.Join(hk, ","))
		}
		if len(why) > 0 {
			diffs++
			fmt.Printf("  step %d: %s\n", i, strings.Join(why, "; "))
		}
	}
	fmt.Printf("  %d steps compared, %d differing, %d equal modulo JSON key order\n", n, diffs, keyOrder)
	if diffs > 0 {
		return 1
	}
	return 0
}

func all(dir, oracleDLL string) int {
	secFile, _ := filepath.Abs(filepath.Join("config", "legacy-secrets.env"))
	os.Setenv("SOLARA_SECRETS_FILE", secFile)
	os.Setenv("DASHBOARD_ADMIN_KEY", "difftest-admin-key")
	os.Setenv("DASHBOARD_SESSION_SECRET", "difftest-session-secret")
	os.Unsetenv("HALCYON_SERVER_API_KEY")
	os.Unsetenv("MOTHERSHIP_INSECURE")
	os.Unsetenv("MOTHERSHIP_ALLOW_NONSTORE")
	os.Unsetenv("DASHBOARD_AUTH_ENFORCE")
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	sort.Strings(files)
	tmp, _ := os.MkdirTemp("", "solara-difftest-")
	fail := 0
	self, _ := os.Executable()
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		fmt.Printf("scenario %s\n", name)
		// both stacks persist EOS sessions/lobbies next to their binaries
		cleanState(filepath.Dir(oracleDLL))
		cleanState(filepath.Dir(self))
		absF, _ := filepath.Abs(f)
		orig := filepath.Join(tmp, name+".original.jsonl")
		sol := filepath.Join(tmp, name+".solara.jsonl")
		if err := runTo(orig, "dotnet", oracleDLL, absF, filepath.Join(tmp, name+"-orig")); err != nil {
			fmt.Println("  oracle failed:", err)
			fail++
			continue
		}
		if err := runTo(sol, self, "run", absF, filepath.Join(tmp, name+"-solara")); err != nil {
			fmt.Println("  solara run failed:", err)
			fail++
			continue
		}
		if compareFiles(orig, sol) != 0 {
			fail++
		}
	}
	fmt.Printf("results in %s; %d scenario(s) with differences\n", tmp, fail)
	if fail > 0 {
		return 1
	}
	return 0
}

func runTo(outPath string, name string, args ...string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	cmd := exec.Command(name, args...)
	cmd.Stdout = f
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %.300s", err, stderr.String())
	}
	return nil
}

// cleanState removes state the servers persist next to their binaries
// (EOS sessions/lobbies) so every scenario starts from the same empty state.
func cleanState(dir string) {
	for _, pat := range []string{"sessions*.json", "lobbies*.json", "playerdata"} {
		ms, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, m := range ms {
			_ = os.RemoveAll(m)
		}
	}
}
