package halcyonsocket

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"solara/internal/docdb"
	"solara/internal/serverid"
)

const testAdminKey = "test-dashboard-admin-key-0123456789"

func newTestServer(t *testing.T) (*HalcyonSocketServer, *fakeClock) {
	t.Helper()
	t.Setenv("DASHBOARD_ADMIN_KEY", testAdminKey)
	prevV := AdminKeyValidator
	AdminKeyValidator = nil
	t.Cleanup(func() { AdminKeyValidator = prevV })
	useTokenDB(t)
	clk := newFakeClock()
	k := currentKnobs()
	s, err := newHalcyonSocketServer(0, "127.0.0.1:0", false, clk.now, k, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	return s, clk
}

// useTokenDB gives the serverid default store a fresh database.
func useTokenDB(t *testing.T) *docdb.Database {
	t.Helper()
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 7, Name: "sid", Path: filepath.Join(t.TempDir(), "sid"+docdb.FileSuffix)})
	if err != nil {
		t.Fatal(err)
	}
	serverid.SetDatabase(db)
	t.Cleanup(func() { serverid.SetDatabase(nil); _ = db.Close() })
	return db
}

// call sends an authenticated (dashboard admin key) request to :9095.
func call(s *HalcyonSocketServer, method, url, body string) (int, string) {
	return callAs(s, method, url, body, map[string]string{"x-dashboard-key": testAdminKey})
}

func callAs(s *HalcyonSocketServer, method, url, body string, hdr map[string]string) (int, string) {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.HTTP.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestRouteTableMatchesOriginal(t *testing.T) {
	_ = installCapture(t)
	s, _ := newTestServer(t)
	data, err := os.ReadFile("../../docs/original_routes.tsv")
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, l := range strings.Split(strings.ReplaceAll(string(data), "\r", ""), "\n") {
		f := strings.Split(l, "\t")
		if len(f) == 4 && f[0] == "HalcyonSocketServer" {
			want = append(want, f[1]+" "+f[2]+" "+f[3])
		}
	}
	var got []string
	for _, e := range s.HTTP.Endpoints() {
		got = append(got, e.Method+" "+e.RouteUri+" "+e.Name)
	}
	if len(want) != 5 {
		t.Fatalf("tsv rows: %v", want)
	}
	compareLines(t, got, want)
}

func TestHTTPNoAgents(t *testing.T) {
	_ = installCapture(t)
	s, _ := newTestServer(t)
	if Instance != s {
		t.Fatal("Instance not set")
	}
	cases := []struct{ method, url, body, want string }{
		{"GET", "/agents", "", `{"agents":[]}`},
		{"POST", "/spinup", "", `{"success":false,"request_id":"","agent":"","error":"no agent with free capacity"}`},
		{"POST", "/spinup", `{"box":""}`, `{"success":false,"request_id":"","agent":"","error":"no agent with free capacity"}`},
		{"POST", "/spinup", `{"box":"gb1"}`, u(`{"success":false,"request_id":"","agent":"","error":"agent %u0027gb1%u0027 unavailable or full"}`)},
		{"POST", "/spinup", `{"box":5}`, `{"success":false,"request_id":"","agent":"","error":"no agent with free capacity"}`},
		{"GET", "/watchdog", "", `{"servers":[]}`},
		{"POST", "/watchdog/retire", "", `{"success":false}`},
		{"POST", "/watchdog/retire", "null", `{"success":false}`},
		{"POST", "/watchdog/retire", `{"deployment_id":"nope"}`, `{"success":false}`},
		{"POST", "/watchdog/resume", `{}`, `{"success":false}`},
	}
	for _, c := range cases {
		code, body := call(s, c.method, c.url, c.body)
		if code != 200 || body != c.want {
			t.Errorf("%s %s %q: %d %s", c.method, c.url, c.body, code, body)
		}
	}
	if code, _ := call(s, "GET", "/spinup", ""); code != 404 {
		t.Errorf("GET /spinup: %d", code)
	}
}

// TestSpinUpRoundTrip: an agent registers, /spinup relays a launch command,
// the agent answers "spunup", the watchdog records it, retire/resume work and
// the disconnect removes the agent.
func TestSpinUpRoundTrip(t *testing.T) {
	c := installCapture(t)
	s, _ := newTestServer(t)
	ag := dialAgent(t, s.Hub().Addr())
	ag.send(`{"type":"hello","box":"gb1","capacity":2,"running":0}` + "\n")

	code, body := call(s, "GET", "/agents", "")
	if code != 200 || !regexp.MustCompile(`^\{"agents":\[\{"box":"gb1","capacity":2,"running":0,"free":2,"last_seen":"2026-10-25T12:00:00Z"\}\]\}$`).MatchString(body) {
		t.Fatalf("agents: %s", body)
	}

	code, body = call(s, "POST", "/spinup", `{"map":"Arena","region":"eu","args":"-foo","name":"  My Station "}`)
	m := regexp.MustCompile(`^\{"success":true,"request_id":"([0-9a-f]{32})","agent":"gb1","error":""\}$`).FindStringSubmatch(body)
	if code != 200 || m == nil {
		t.Fatalf("spinup: %d %s", code, body)
	}
	reqId := m[1]
	line := ag.readLine()
	// Solara (SEC-SID): the wire args carry a fresh per-server token as
	// -ServerApiKey and -DashboardApiKey, repeated in "server_token".
	wm := regexp.MustCompile(`^\{"type":"spinup","id":"` + reqId + `","map":"Arena","region":"eu","args":"-foo -DashboardDeploymentId=([0-9a-f]{32}) -ServerApiKey=(sst1_[A-Za-z0-9_-]{43}) -DashboardApiKey=(sst1_[A-Za-z0-9_-]{43})","server_token":"(sst1_[A-Za-z0-9_-]{43})"\}$`).FindStringSubmatch(line)
	if wm == nil || wm[2] != wm[3] || wm[3] != wm[4] {
		t.Fatalf("wire: %s", line)
	}
	dep := wm[1]
	if rec, ok := serverid.Validate(wm[4]); !ok || rec.Deployment() != dep || rec.BoxName() != "gb1" || rec.Kind != serverid.KindServer {
		t.Fatalf("token record: %v %+v", ok, rec)
	}
	if n, ok := PendingNames.TryRemove(dep); !ok || n != "My Station" {
		t.Fatalf("PendingNames: %q %v", n, ok)
	}

	// Solara (F5): a caller can no longer supply its own deployment id
	_, body = call(s, "POST", "/spinup", `{"box":"gb1","args":"-dashboarddeploymentid=fixed1 -x"}`)
	if body != `{"success":false,"request_id":"","agent":"","error":"invalid spin-up request: arg -dashboarddeploymentid is not allowed in a spin-up"}` {
		t.Fatal(body)
	}
	if l := ag.readLine(); l != "<none>" {
		t.Fatalf("rejected spin-up reached the agent: %s", l)
	}

	ag.send(`{"type":"spunup","id":"` + reqId + `","pid":"4321","status":"ok","msg":"launched and injected; it self-registers once up"}` + "\n")
	_, body = call(s, "GET", "/watchdog", "")
	want := `{"servers":[` +
		`{"deployment_id":"` + dep + `","name":"My Station ","box":"gb1","pid":4321,"players":-1,"uptime_minutes":0,"last_heartbeat":null,"dispatch_age_ms":0,"tick_age_ms":0,"verdict":"starting","restarts":0,"last_restart":null,"retired":false,"retired_reason":""}]}`
	// note: the watchdog keeps the UNTRIMMED name (only PendingNames is trimmed) -- original behaviour
	want = strings.Replace(want, `"name":"My Station "`, `"name":"  My Station "`, 1)
	if body != want {
		t.Fatalf("watchdog:\n got %s\nwant %s", body, want)
	}

	// heartbeat feed (A2StationDbServer /server_heartbeat calls this)
	Instance.Watchdog.OnHeartbeat(dep, 4321, 9, 1, 2, 3_600_000, 4, nil)
	_, body = call(s, "POST", "/watchdog/retire", `{"deployment_id":"`+dep+`"}`)
	if body != `{"success":true}` {
		t.Fatal(body)
	}
	rows := s.WatchdogRows()
	if rows[0].Retired_reason != "stopped on purpose" || rows[0].Verdict != "retired" || rows[0].Uptime_minutes != 60 || rows[0].Players != 4 || rows[0].Last_heartbeat == nil {
		t.Fatalf("row: %+v", rows[0])
	}
	_, body = call(s, "POST", "/watchdog/resume", `{"deployment_id":"`+dep+`"}`)
	if body != `{"success":true}` || s.WatchdogRows()[0].Verdict != "watching" {
		t.Fatal(body)
	}

	// failed send: the agent's socket is gone but the hub has not noticed yet
	_ = ag.c.Close()
	waitLine(t, c, "disconnected")
	if len(s.GetAgents()) != 0 {
		t.Fatal("agent not removed")
	}
	got := c.get()
	has := func(p string) bool {
		for _, l := range got {
			if strings.HasPrefix(l, p) {
				return true
			}
		}
		return false
	}
	for _, p := range []string{
		"[HalcyonSocket] agent hub listening on TCP ",
		"[HalcyonSocket] control HTTP on 9095, agent TCP on 9100, watchdog armed",
		"[HalcyonSocket] agent 'gb1' online (cap=2, running=0)",
		"[HalcyonSocket] spinup " + reqId + " -> agent 'gb1' (map='Arena', region='eu')",
		"[HalcyonSocket] spunup id=" + reqId + " pid=4321 status=ok msg=launched and injected; it self-registers once up",
		"[Watchdog] " + dep[:8] + " launched on 'gb1' pid=4321",
		"[Watchdog] " + dep[:8] + " retired: stopped on purpose",
		"[Watchdog] " + dep[:8] + " back under watch",
		"[HalcyonSocket] agent 'gb1' disconnected",
	} {
		if !has(p) {
			t.Errorf("missing log line %q", p)
		}
	}
}

func TestWatchdogRowsOrderedByNameOrdinal(t *testing.T) {
	_ = installCapture(t)
	s, _ := newTestServer(t)
	for _, n := range []string{"b", "B", "a", "Z"} {
		s.Watchdog.OnSpinUpRequested("id-"+n, "r"+n, "x", nil, nil, "", strp(n))
	}
	s.Watchdog.OnHeartbeat("id-none", 1, 1, 1, 1, 1, 1, nil)
	var names []string
	for _, r := range s.WatchdogRows() {
		names = append(names, r.Name)
	}
	if got := strings.Join(names, ","); got != ",B,Z,a,b" {
		t.Fatal(got)
	}
}
