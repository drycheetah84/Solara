package halcyonsocket

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"solara/internal/hosting"
	"solara/internal/secrets"
	"solara/internal/serverid"
)

// ─── F5: spin-up validation ──────────────────────────────────────────────────

func TestSpinUpValidation(t *testing.T) {
	sp := func(s string) *string { return &s }
	good := []struct{ box, mp, region, args, name *string }{
		{nil, nil, nil, nil, nil},
		{sp(""), sp(""), sp(""), sp(""), sp("")},
		{sp("win-gamebox-1"), sp("/Game/Maps/Arena"), sp("NAE"), sp("-foo -Port=7778 -GamemodeSlot=2 -log"), sp("My Station")},
		{nil, sp("Arena_01"), sp("eu-west"), sp("  -nosound   -Ratio=1.5,2:3+4  "), sp("Bob's place (EU) #2")},
		{nil, sp("A2/Modes/Spec.Spec_C"), nil, sp("-MaxPlayers=16"), nil},
	}
	for i, g := range good {
		if why := validateSpinUp(g.box, g.mp, g.region, g.args, g.name, false); why != "" {
			t.Errorf("good %d rejected: %s", i, why)
		}
	}
	bad := []struct {
		label                    string
		box, mp, region, args, n *string
	}{
		{"map injection", nil, sp(`/Game/Map -ExecCmds="quit"`), nil, nil, nil},
		{"map quote", nil, sp(`Arena"`), nil, nil, nil},
		{"map space", nil, sp("Arena -log"), nil, nil, nil},
		{"map dotdot", nil, sp("/Game/../../x"), nil, nil, nil},
		{"map too long", nil, sp(strings.Repeat("a", 129)), nil, nil, nil},
		{"args ExecCmds", nil, nil, nil, sp(`-ExecCmds="quit"`), nil},
		{"args ExecCmds plain", nil, nil, nil, sp(`-execcmds=quit`), nil},
		{"args quote", nil, nil, nil, sp(`-Name="x y"`), nil},
		{"args no dash", nil, nil, nil, sp(`foo`), nil},
		{"args DashboardApiKey", nil, nil, nil, sp(`-DashboardApiKey=x`), nil},
		{"args ServerApiKey", nil, nil, nil, sp(`-serverapikey=x`), nil},
		{"args DashboardDeploymentId", nil, nil, nil, sp(`-DashboardDeploymentId=abc`), nil},
		{"args abslog", nil, nil, nil, sp(`-abslog=C:\x`), nil},
		{"args abslog fwd", nil, nil, nil, sp(`-abslog=C:/x`), nil},
		{"args log path", nil, nil, nil, sp(`-log=../../x.log`), nil},
		{"args ini override", nil, nil, nil, sp(`-ENGINEINI=x`), nil},
		{"args ini colon form", nil, nil, nil, sp(`-ini:Engine:[Core.Log]:x=y`), nil},
		{"args BackendHost", nil, nil, nil, sp(`-BackendHost=1.2.3.4`), nil},
		{"args MothershipHost", nil, nil, nil, sp(`-MothershipHost=evil`), nil},
		{"args RegisterIp", nil, nil, nil, sp(`-RegisterIp=1.1.1.1`), nil},
		{"args DashboardApiUrl", nil, nil, nil, sp(`-DashboardApiUrl=http://evil:78`), nil},
		{"args run commandlet", nil, nil, nil, sp(`-run=cook`), nil},
		{"args too many", nil, nil, nil, sp(strings.Repeat("-a ", 33)), nil},
		{"args semicolon", nil, nil, nil, sp(`-a=1;calc`), nil},
		{"args backtick", nil, nil, nil, sp("-a=`x`"), nil},
		{"region space", nil, nil, sp("eu west"), nil, nil},
		{"box injection", sp(`gb1" -x`), nil, nil, nil, nil},
		{"name control", nil, nil, nil, nil, sp("a\nb")},
		{"name too long", nil, nil, nil, nil, sp(strings.Repeat("n", 65))},
	}
	for _, b := range bad {
		if why := validateSpinUp(b.box, b.mp, b.region, b.args, b.n, false); why == "" {
			t.Errorf("%s: accepted", b.label)
		}
	}
	// the watchdog's restart path may carry the deployment id it recorded, but no keys
	if why := validateSpinUp(nil, nil, nil, sp("-x -DashboardDeploymentId=abc"), nil, true); why != "" {
		t.Errorf("internal deployment id rejected: %s", why)
	}
	if why := validateSpinUp(nil, nil, nil, sp("-DashboardApiKey=abc"), nil, true); why == "" {
		t.Error("internal path accepted a key")
	}
}

func TestSpinUpRejectionShapeAndWatchdogRestartMintsNewToken(t *testing.T) {
	_ = installCapture(t)
	s, _ := newTestServer(t)
	ag := dialAgent(t, s.Hub().Addr())
	ag.send(`{"type":"hello","box":"gb1","capacity":4,"running":0}` + "\n")

	_, body := call(s, "POST", "/spinup", `{"map":"/Game/Map -ExecCmds=\"quit\""}`)
	if body != `{"success":false,"request_id":"","agent":"","error":"invalid spin-up request: map must match [A-Za-z0-9_./-]{1,128} and must not contain .."}` {
		t.Fatal(body)
	}
	if l := ag.readLine(); l != "<none>" {
		t.Fatalf("rejected spin-up reached the agent: %s", l)
	}

	// a watchdog restart (internal path) re-uses the deployment id and mints a NEW token
	r := s.requestSpinUp("", strp("gb1"), strp("Arena"), strp(""), strp("-x -DashboardDeploymentId=dep1"), nil, true)
	if !r.Success {
		t.Fatal(r.Error)
	}
	l1 := ag.readLine()
	r = s.requestSpinUp("", strp("gb1"), strp("Arena"), strp(""), strp("-x -DashboardDeploymentId=dep1"), nil, true)
	l2 := ag.readLine()
	if !r.Success || !strings.Contains(l1, `"args":"-x -DashboardDeploymentId=dep1 -ServerApiKey=sst1_`) || l1 == l2 {
		t.Fatalf("restart wire:\n%s\n%s", l1, l2)
	}
	if strings.Contains(s.Watchdog.Servers()[0].Args, "sst1_") {
		t.Fatal("watchdog recorded the token")
	}
}

// ─── F5: :9095 authentication ────────────────────────────────────────────────

func TestControlApiAuth(t *testing.T) {
	_ = installCapture(t)
	s, _ := newTestServer(t)
	prev := secrets.Get("ServerMasterKey")
	secrets.Set("ServerMasterKey", "test-master-key")
	t.Cleanup(func() { secrets.Set("ServerMasterKey", prev) })
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "")

	routes := [][2]string{{"GET", "/agents"}, {"POST", "/spinup"}, {"GET", "/watchdog"}, {"POST", "/watchdog/retire"}, {"POST", "/watchdog/resume"}}
	for _, r := range routes {
		for _, hdr := range []map[string]string{
			nil,
			{"x-dashboard-key": "wrong"},
			{"x-api-key": "wrong"},
			{"x-api-key": testAdminKey}, // the admin key belongs in x-dashboard-key
			{"x-dashboard-key": ""},
		} {
			if code, body := callAs(s, r[0], r[1], "", hdr); code != 401 || body != `{"error":"unauthorized"}` {
				t.Errorf("%s %s %v: %d %s", r[0], r[1], hdr, code, body)
			}
		}
		if code, _ := callAs(s, r[0], r[1], "", map[string]string{"x-dashboard-key": testAdminKey}); code != 200 {
			t.Errorf("%s %s admin key: %d", r[0], r[1], code)
		}
		if code, _ := callAs(s, r[0], r[1], "", map[string]string{"x-api-key": "test-master-key"}); code != 200 {
			t.Errorf("%s %s master key (compat): %d", r[0], r[1], code)
		}
	}
	// strict: the static master key no longer opens :9095
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "1")
	if code, _ := callAs(s, "GET", "/agents", "", map[string]string{"x-api-key": "test-master-key"}); code != 401 {
		t.Errorf("strict master key: %d", code)
	}
	// the host-wired validator (dashboard admin key logic) is honoured
	AdminKeyValidator = func(*hosting.Request) bool { return true }
	if code, _ := callAs(s, "GET", "/agents", "", map[string]string{"x-dashboard-key": "anything"}); code != 200 {
		t.Errorf("validator: %d", code)
	}
}

// ─── F6: agent hub ───────────────────────────────────────────────────────────

// ipHub starts a hub whose connections get fake remote IPs, assigned in dial
// order by dialFrom.
type ipHub struct {
	*AgentHub
	mu  sync.Mutex
	ips map[string]string // client local addr -> fake IP
}

func newIPHub(t *testing.T) *ipHub {
	t.Helper()
	h := &ipHub{AgentHub: newAgentHubAddr("127.0.0.1:0"), ips: map[string]string{}}
	h.ipOf = func(c net.Conn) string {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			h.mu.Lock()
			ip, ok := h.ips[c.RemoteAddr().String()]
			h.mu.Unlock()
			if ok {
				return ip
			}
			time.Sleep(5 * time.Millisecond)
		}
		return "?"
	}
	if err := h.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.close)
	return h
}

func (h *ipHub) dialFrom(t *testing.T, ip string) *fakeAgent {
	t.Helper()
	a := dialAgent(t, h.Addr())
	h.mu.Lock()
	h.ips[a.c.LocalAddr().String()] = ip
	h.mu.Unlock()
	return a
}

// closed reports whether the hub closed the agent's connection.
func (a *fakeAgent) closed() bool {
	_ = a.c.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	_, err := a.c.Read(buf)
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return false
	}
	return err != nil
}

func secCapture(t *testing.T) *capture {
	c := &capture{}
	prev := securityLog
	securityLog = c.add
	t.Cleanup(func() { securityLog = prev })
	return c
}

func TestAgentLineCap(t *testing.T) {
	_ = installCapture(t)
	sc := secCapture(t)
	h := newIPHub(t)
	a := h.dialFrom(t, "10.0.0.1")
	a.send(`{"type":"hello","box":"gb1","capacity":1,"running":0}` + "\n")
	big := strings.Repeat("x", 70*1024) // no newline: the original buffered this forever
	_, _ = a.c.Write([]byte(big))
	if !a.closed() {
		t.Fatal("not disconnected on an unterminated 70 KiB line")
	}
	waitFor(t, func() bool { return len(h.Agents()) == 0 })
	waitLine(t, sc, "without a newline -- disconnecting")

	b := h.dialFrom(t, "10.0.0.2")
	_, _ = b.c.Write([]byte(`{"type":"heartbeat","pad":"` + strings.Repeat("y", 66*1024) + `"}` + "\n"))
	if !b.closed() {
		t.Fatal("not disconnected on a 66 KiB line")
	}
	// a line just under the cap is fine
	c := h.dialFrom(t, "10.0.0.3")
	c.send(`{"type":"hello","box":"gb3","capacity":1,"running":0,"pad":"` + strings.Repeat("z", 60*1024) + `"}` + "\n")
	if c.closed() || h.ByBox("gb3") == nil {
		t.Fatal("60 KiB line refused")
	}
}

func TestAgentAuthentication(t *testing.T) {
	_ = installCapture(t)
	sc := secCapture(t)
	useTokenDB(t)
	h := newIPHub(t)
	tok, rec, err := serverid.Issue(serverid.KindAgent, "", "gb1", serverid.TTLs{})
	if err != nil {
		t.Fatal(err)
	}
	srvTok, _, _ := serverid.Issue(serverid.KindServer, "dep", "gb1", serverid.TTLs{})

	// compat mode: an unauthenticated agent is accepted with a warning
	t.Setenv("SOLARA_REQUIRE_AGENT_AUTH", "")
	a := h.dialFrom(t, "10.0.0.1")
	a.send(`{"type":"hello","box":"plain","capacity":1,"running":0}` + "\n")
	if h.ByBox("plain") == nil || h.ByBox("plain").Authenticated() {
		t.Fatal("compat agent")
	}
	waitLine(t, sc, "accepted (compat mode; issue an agent token or set SOLARA_AGENT_ALLOWED_IPS)")

	// strict mode
	t.Setenv("SOLARA_REQUIRE_AGENT_AUTH", "1")
	t.Setenv("SOLARA_AGENT_ALLOWED_IPS", "192.168.10.0/24, 10.9.9.9")
	for _, c := range []struct{ label, ip, hello string }{
		{"no token", "10.0.0.2", `{"type":"hello","box":"gbX","capacity":9,"running":0}`},
		{"token for another box", "10.0.0.3", `{"type":"hello","box":"gb2","capacity":9,"running":0,"token":"` + tok + `"}`},
		{"server token", "10.0.0.4", `{"type":"hello","box":"gb1","capacity":9,"running":0,"token":"` + srvTok + `"}`},
		{"garbage token", "10.0.0.5", `{"type":"hello","box":"gb1","capacity":9,"running":0,"token":"sst1_nope"}`},
	} {
		x := h.dialFrom(t, c.ip)
		x.send(c.hello + "\n")
		if !x.closed() {
			t.Errorf("%s: not disconnected", c.label)
		}
	}
	// strict: messages before an admitted hello are ignored
	pre := h.dialFrom(t, "10.0.0.6")
	pre.send(`{"type":"heartbeat","running":5}` + "\n")
	for _, ag := range h.Agents() {
		if ag.Running() == 5 {
			t.Error("pre-hello heartbeat processed in strict mode")
		}
	}

	good := h.dialFrom(t, "10.0.0.7")
	good.send(`{"type":"hello","box":"gb1","capacity":2,"running":0,"token":"` + tok + `"}` + "\n")
	if g := h.ByBox("gb1"); g == nil || !g.Authenticated() {
		t.Fatal("agent token hello not admitted")
	}
	for _, ip := range []string{"192.168.10.44", "10.9.9.9"} {
		x := h.dialFrom(t, ip)
		x.send(`{"type":"hello","box":"allow-` + ip + `","capacity":1,"running":0}` + "\n")
		if g := h.ByBox("allow-" + ip); g == nil || !g.Authenticated() {
			t.Errorf("allowlisted %s not admitted", ip)
		}
	}

	// revoking the agent token disconnects the agent at its next heartbeat
	if _, err := serverid.Revoke(rec.Prefix, "test"); err != nil {
		t.Fatal(err)
	}
	good.send(`{"type":"heartbeat","running":0}` + "\n")
	if !good.closed() {
		t.Fatal("revoked agent still connected")
	}
}

func TestAgentBoxHijackAndForeignReports(t *testing.T) {
	_ = installCapture(t)
	_ = secCapture(t)
	useTokenDB(t)
	t.Setenv("SOLARA_REQUIRE_AGENT_AUTH", "")
	t.Setenv("SOLARA_AGENT_ALLOWED_IPS", "")
	h := newIPHub(t)
	var mu sync.Mutex
	var exits, spunups []string
	h.SetOnExited(func(box, pid, code string) { mu.Lock(); exits = append(exits, box+"/"+pid); mu.Unlock() })
	h.SetOnSpunUp(func(req, pid, status, box string) { mu.Lock(); spunups = append(spunups, req); mu.Unlock() })
	got := func(l *[]string) []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), *l...) }

	real := h.dialFrom(t, "10.0.0.1")
	real.send(`{"type":"hello","box":"gb1","capacity":4,"running":0}` + "\n")
	conn := h.ByBox("gb1")

	// another IP claiming the connected box name is refused
	fake := h.dialFrom(t, "10.6.6.6")
	fake.send(`{"type":"hello","box":"gb1","capacity":99,"running":0}` + "\n")
	if !fake.closed() || h.Pick(nil) != conn {
		t.Fatal("box-name hijack not refused")
	}
	// the same machine reconnecting is fine (original behaviour)
	again := h.dialFrom(t, "10.0.0.1")
	again.send(`{"type":"hello","box":"gb1","capacity":4,"running":0}` + "\n")
	if again.closed() {
		t.Fatal("same-IP reconnect refused")
	}

	// launches tracked against the real connection
	h.trackLaunch("req1", conn)
	h.trackLaunch("req2", conn)
	real.send(`{"type":"spunup","id":"req1","pid":"100","status":"ok","msg":"x"}` + "\n")
	if s := got(&spunups); len(s) != 1 || s[0] != "req1" {
		t.Fatalf("own spunup: %v", s)
	}

	// the real agent drops; an impostor from another IP takes the free name
	_ = real.c.Close()
	_ = again.c.Close()
	waitFor(t, func() bool { return h.ByBox("gb1") == nil })
	imp := h.dialFrom(t, "10.6.6.6")
	imp.send(`{"type":"hello","box":"gb1","capacity":99,"running":0}` + "\n")
	imp.send(`{"type":"exited","pid":"100","code":"0","running":0}` + "\n")
	imp.send(`{"type":"spunup","id":"req2","pid":"200","status":"error","msg":"x"}` + "\n")
	if e := got(&exits); len(e) != 0 {
		t.Fatalf("foreign exited accepted: %v", e)
	}
	if s := got(&spunups); len(s) != 1 {
		t.Fatalf("foreign spunup accepted: %v", s)
	}
	// unknown pids keep the original behaviour (the watchdog decides)
	imp.send(`{"type":"exited","pid":"555","code":"0","running":0}` + "\n")
	if e := got(&exits); len(e) != 1 || e[0] != "gb1/555" {
		t.Fatalf("unknown pid: %v", e)
	}

	// a token holder for the box evicts the impostor and may report
	tok, _, _ := serverid.Issue(serverid.KindAgent, "", "gb1", serverid.TTLs{})
	owner := h.dialFrom(t, "10.0.0.99")
	owner.send(`{"type":"hello","box":"gb1","capacity":4,"running":0,"token":"` + tok + `"}` + "\n")
	if !imp.closed() {
		t.Fatal("impostor not evicted by the token holder")
	}
	owner.send(`{"type":"exited","pid":"100","code":"0","running":0}` + "\n")
	if e := got(&exits); len(e) != 2 || e[1] != "gb1/100" {
		t.Fatalf("token holder's exited: %v", e)
	}
}
