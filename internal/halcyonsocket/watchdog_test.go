package halcyonsocket

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"solara/internal/dotnet"
)

type wdHarness struct {
	t   *testing.T
	c   *capture
	clk *fakeClock
	hub *AgentHub
	wd  *ServerWatchdog
	ag  *fakeAgent
}

// newWdHarness mirrors the oracle's "wd"/"fast" set-up: a hub with one agent
// (boxA, capacity 4), a watchdog whose spin-up callback just logs, and the
// TakeDeploymentOffline hook logging too.
func newWdHarness(t *testing.T, k knobs) *wdHarness {
	h := &wdHarness{t: t, c: installCapture(t), clk: newFakeClock()}
	h.hub = newAgentHubAddr("127.0.0.1:0")
	h.hub.now = h.clk.now
	if err := h.hub.Start(); err != nil {
		t.Fatal(err)
	}
	h.ag = dialAgent(t, h.hub.Addr())
	h.ag.send(`{"type":"hello","box":"boxA","capacity":4,"running":0}` + "\n")
	h.clk.advance(200)
	prevHook := TakeDeploymentOffline
	TakeDeploymentOffline = func(d, r string) { consoleWriteLine(fmt.Sprintf("HOOK offline %s %s", d, r)) }
	t.Cleanup(func() { TakeDeploymentOffline = prevHook })
	or := func(s *string) string {
		if s == nil {
			return "<null>"
		}
		return *s
	}
	h.wd = newServerWatchdog(h.hub, func(b, m, r, a, n *string) *SpinUpResult {
		consoleWriteLine(fmt.Sprintf("SPINUP box=%s map=%s region=%s args=%s name=%s", or(b), or(m), or(r), or(a), or(n)))
		return &SpinUpResult{Success: true, Request_id: "REQ", Agent: deref(b)}
	}, h.clk.now, k, false)
	h.hub.SetOnSpunUp(h.wd.OnSpunUp)
	h.hub.SetOnExited(h.wd.OnExited)
	h.hub.SetOnProbeResult(h.wd.OnProbeResult)
	h.wd.Evaluate() // the constructor's first pass, on an empty set
	h.clk.advance(300)
	t.Cleanup(func() { h.hub.close(); h.wd.close() })
	return h
}

func (h *wdHarness) ev() { consoleWriteLine("-- evaluate"); h.wd.Evaluate() }

func (h *wdHarness) dump() {
	ws := h.wd.Servers()
	sort.SliceStable(ws, func(i, j int) bool { return ws[i].DeploymentId < ws[j].DeploymentId })
	for _, w := range ws {
		consoleWriteLine(fmt.Sprintf("STATE %s box=%s pid=%d verdict=%s restarts=%d retired=%s reason=%s hb=%s name=%s",
			w.DeploymentId, w.Box, w.Pid, w.LastVerdict, w.RestartsInWindow, dotnet.FormatBool(w.Retired), w.RetiredReason,
			dotnet.FormatBool(w.LastHeartbeat != nil), w.Name))
	}
}

func (h *wdHarness) wire() { consoleWriteLine("WIRE " + h.ag.readLine()) }

func (h *wdHarness) check(want string) {
	got := h.c.get()
	if !strings.HasPrefix(got[0], "[HalcyonSocket] agent hub listening on TCP ") {
		h.t.Fatalf("first line %q", got[0])
	}
	compareLines(h.t, got[1:], strings.Split(strings.TrimSpace(want), "\n"))
}

// TestWatchdogOracle replays tools/oracle-halcyonsocket "wd" mode.
func TestWatchdogOracle(t *testing.T) {
	k := currentKnobs()
	k.evaluateEveryMs = 100000000
	h := newWdHarness(t, k)
	wd := h.wd
	wd.OnSpinUpRequested("dep-frozen-0001", "req1", "boxA", strp("mapA"), nil, "-x", strp("Alpha"))
	wd.OnSpunUp("req1", "4242", "ok", "boxA")
	wd.OnSpunUp("req1", "4242", "ok", "boxA")
	wd.OnSpinUpRequested("dep-healthy-0002", "req2", "boxA", nil, strp("eu"), "", strp("  "))
	wd.OnSpunUp("req2", " 77 ", "ok", "boxA")
	wd.OnSpinUpRequested("dep-start-0003", "req3", "boxA", strp("m"), strp("r"), "-DashboardDeploymentId=dep-start-0003", strp("Gamma"))
	wd.OnSpinUpRequested("dep-fail-0004", "req4", "boxA", strp("m"), strp("r"), "", strp("Delta"))
	wd.OnSpunUp("req4", "", "error", "boxA")
	wd.OnSpinUpRequested("dep-nobox-0005", "req5", "boxZ", strp("m"), strp("r"), "", strp("Eps"))
	wd.OnHeartbeat("dep-frozen-0001", 4242, 1, 100000, 95000, 7200000, 3, nil)
	wd.OnHeartbeat("dep-healthy-0002", 77, 1, 10, 10, 7260000, 5, nil)
	wd.OnHeartbeat("dep-adopt-0006", 999, 1, 10, 10, 60000, 0, nil)
	wd.OnHeartbeat("  ", 1, 1, 1, 1, 1, 1, nil)
	h.ev()
	h.dump()
	h.wire()
	h.wire()
	wd.OnProbeResult("boxA", "999", true)
	h.ev()
	h.dump()
	consoleWriteLine("STOP adopt => " + wd.Stop("dep-adopt-0006", "station deleted"))
	h.wire()
	consoleWriteLine("STOP none => " + wd.Stop("nope", "x"))
	consoleWriteLine("STOP start => " + wd.Stop("dep-start-0003", "x"))
	consoleWriteLine("STOP healthy => " + wd.Stop("dep-healthy-0002", "y"))
	h.wire()
	consoleWriteLine("RETIRE => " + dotnet.FormatBool(wd.Retire("dep-nobox-0005", "manual")) + " " + dotnet.FormatBool(wd.Retire("zzz", "manual")))
	consoleWriteLine("RESUME => " + dotnet.FormatBool(wd.Resume("dep-nobox-0005")) + " " + dotnet.FormatBool(wd.Resume("zzz")))
	wd.OnSpinUpRequested("dep-exit-0007", "req7", "boxA", strp("m7"), strp("r7"), "-p", strp("Seven"))
	wd.OnSpunUp("req7", "55", "ok", "boxA")
	wd.OnExited("boxA", "56", "1")
	wd.OnExited("boxA", "55", "1")
	h.wire()
	wd.OnExited("boxA", "55", "1")
	h.ev()
	h.dump()
	consoleWriteLine("FORGET => " + dotnet.FormatBool(wd.Forget("dep-exit-0007")) + " " + dotnet.FormatBool(wd.Forget("dep-exit-0007")))

	h.check(`
[HalcyonSocket] agent 'boxA' online (cap=4, running=0)
[Watchdog] dep-froz launched on 'boxA' pid=4242
[Watchdog] dep-heal launched on 'boxA' pid=77
[Watchdog] dep-fail spin-up failed on 'boxA' (error) -- not watching it
[Watchdog] dep-adop is heartbeating but we did not launch it -- watching it too
-- evaluate
[Watchdog] *** dep-froz 'Alpha' is DOWN [engine-frozen]: still heartbeating (so the process is alive) but no UFunction dispatch for 100s, and our own tick has not run for 95s
HOOK offline dep-frozen-0001 engine-frozen
[Watchdog]   killed pid 4242 on 'boxA'
SPINUP box=boxA map=mapA region=<null> args=-x -DashboardDeploymentId=dep-frozen-0001 name=Alpha
[Watchdog]   replacement requested on 'boxA' (request REQ), restart 1/3
STATE dep-adopt-0006 box= pid=999 verdict=asking 1 box(es) which one runs pid 999 restarts=0 retired=False reason= hb=True name=
STATE dep-fail-0004 box=boxA pid=0 verdict=spinup-failed restarts=0 retired=True reason=spin-up failed: error hb=False name=Delta
STATE dep-frozen-0001 box=boxA pid=0 verdict=restarting (engine-frozen) restarts=1 retired=False reason= hb=False name=Alpha
STATE dep-healthy-0002 box=boxA pid=77 verdict=healthy (5 player(s), up 121m) restarts=0 retired=False reason= hb=True name=
STATE dep-nobox-0005 box=boxZ pid=0 verdict=holding: agent for this box is not connected restarts=0 retired=False reason= hb=False name=Eps
STATE dep-start-0003 box=boxA pid=0 verdict=starting (no heartbeat yet) restarts=0 retired=False reason= hb=False name=Gamma
WIRE {"type":"kill","pid":"4242","reason":"engine-frozen"}
WIRE {"type":"probe","pid":"999"}
-- evaluate
[Watchdog] dep-adop adopted: pid 999 belongs to box 'boxA'
STATE dep-adopt-0006 box=boxA pid=999 verdict=healthy (0 player(s), up 1m) restarts=0 retired=False reason= hb=True name=
STATE dep-fail-0004 box=boxA pid=0 verdict=spinup-failed restarts=0 retired=True reason=spin-up failed: error hb=False name=Delta
STATE dep-frozen-0001 box=boxA pid=0 verdict=cooling down restarts=1 retired=False reason= hb=False name=Alpha
STATE dep-healthy-0002 box=boxA pid=77 verdict=healthy (5 player(s), up 121m) restarts=0 retired=False reason= hb=True name=
STATE dep-nobox-0005 box=boxZ pid=0 verdict=holding: agent for this box is not connected restarts=0 retired=False reason= hb=False name=Eps
STATE dep-start-0003 box=boxA pid=0 verdict=starting (no heartbeat yet) restarts=0 retired=False reason= hb=False name=Gamma
[Watchdog] dep-adop stopped (station deleted): kill pid 999 on 'boxA' sent
STOP adopt => server pid 999 on 'boxA' is being shut down
WIRE {"type":"kill","pid":"999","reason":"station deleted"}
STOP none => no running server known for it
STOP start => retired; its process id is not known yet
[Watchdog] dep-heal stopped (y): kill pid 77 on 'boxA' sent
STOP healthy => server pid 77 on 'boxA' is being shut down
WIRE {"type":"kill","pid":"77","reason":"y"}
[Watchdog] dep-nobo retired: manual
RETIRE => True False
[Watchdog] dep-nobo back under watch
RESUME => True False
[Watchdog] dep-exit launched on 'boxA' pid=55
[Watchdog] dep-exit pid 55 exited (code 1) -- the agent owns the process, so this is certain
[Watchdog] *** dep-exit 'Seven' is DOWN [process-exited]: the agent reported pid 55 exited with code 1
HOOK offline dep-exit-0007 process-exited
[Watchdog]   killed pid 55 on 'boxA'
SPINUP box=boxA map=m7 region=r7 args=-p -DashboardDeploymentId=dep-exit-0007 name=Seven
[Watchdog]   replacement requested on 'boxA' (request REQ), restart 1/3
WIRE {"type":"kill","pid":"55","reason":"process-exited"}
-- evaluate
STATE dep-adopt-0006 box=boxA pid=999 verdict=stopped restarts=0 retired=True reason=station deleted hb=True name=
STATE dep-exit-0007 box=boxA pid=0 verdict=cooling down restarts=1 retired=False reason= hb=False name=Seven
STATE dep-fail-0004 box=boxA pid=0 verdict=spinup-failed restarts=0 retired=True reason=spin-up failed: error hb=False name=Delta
STATE dep-frozen-0001 box=boxA pid=0 verdict=cooling down restarts=1 retired=False reason= hb=False name=Alpha
STATE dep-healthy-0002 box=boxA pid=77 verdict=stopped restarts=0 retired=True reason=y hb=True name=
STATE dep-nobox-0005 box=boxZ pid=0 verdict=holding: agent for this box is not connected restarts=0 retired=False reason= hb=False name=Eps
STATE dep-start-0003 box=boxA pid=0 verdict=stopped restarts=0 retired=True reason=x hb=False name=Gamma
FORGET => True False
`)
}

// TestWatchdogSilentAndGiveUpOracle replays the oracle's "fast" mode
// (HB_GRACE 40ms, SILENT 80ms, COOLDOWN 1ms) on a fake clock.
func TestWatchdogSilentAndGiveUpOracle(t *testing.T) {
	k := currentKnobs()
	k.evaluateEveryMs = 100000000
	k.heartbeatGraceMs, k.silentMs, k.cooldownMs = 40, 80, 1
	h := newWdHarness(t, k)
	wd := h.wd
	wd.OnSpinUpRequested("dep-silent-0001", "r1", "boxA", strp("m"), strp("r"), "-a", strp("S1"))
	wd.OnSpunUp("r1", "100", "ok", "boxA")
	wd.OnHeartbeat("dep-silent-0001", 100, 1, 10, 10, 60000, 1, nil)
	h.ev()
	h.dump()
	h.clk.advance(50)
	h.ev()
	h.dump()
	h.clk.advance(100)
	h.ev()
	h.dump()
	h.wire()
	h.ev()
	h.dump()
	wd.OnProbeResult("boxA", "100", true)
	h.ev()
	h.dump()
	h.wire()
	for i := 0; i < 4; i++ {
		wd.OnSpinUpRequested("dep-loop-0002", fmt.Sprintf("L%d", i), "boxA", strp("m"), strp("r"), "-b", strp("Loop"))
		wd.OnSpunUp(fmt.Sprintf("L%d", i), fmt.Sprint(200+i), "ok", "boxA")
		h.clk.advance(5)
		wd.OnExited("boxA", fmt.Sprint(200+i), "-1")
		h.wire()
	}
	h.dump()

	var loop strings.Builder
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&loop, `[Watchdog] dep-loop launched on 'boxA' pid=%[1]d
[Watchdog] dep-loop pid %[1]d exited (code -1) -- the agent owns the process, so this is certain
[Watchdog] *** dep-loop 'Loop' is DOWN [process-exited]: the agent reported pid %[1]d exited with code -1
HOOK offline dep-loop-0002 process-exited
[Watchdog]   killed pid %[1]d on 'boxA'
SPINUP box=boxA map=m region=r args=-b -DashboardDeploymentId=dep-loop-0002 name=Loop
[Watchdog]   replacement requested on 'boxA' (request REQ), restart %[2]d/3
WIRE {"type":"kill","pid":"%[1]d","reason":"process-exited"}
`, 200+i, i+1)
	}
	h.check(`
[HalcyonSocket] agent 'boxA' online (cap=4, running=0)
[Watchdog] dep-sile launched on 'boxA' pid=100
-- evaluate
STATE dep-silent-0001 box=boxA pid=100 verdict=healthy (1 player(s), up 1m) restarts=0 retired=False reason= hb=True name=S1
-- evaluate
STATE dep-silent-0001 box=boxA pid=100 verdict=quiet for 0s -- waiting restarts=0 retired=False reason= hb=True name=S1
-- evaluate
STATE dep-silent-0001 box=boxA pid=100 verdict=silent for 0s -- asking the agent whether pid 100 is alive restarts=0 retired=False reason= hb=True name=S1
WIRE {"type":"probe","pid":"100"}
-- evaluate
STATE dep-silent-0001 box=boxA pid=100 verdict=silent for 0s -- asking the agent whether pid 100 is alive restarts=0 retired=False reason= hb=True name=S1
-- evaluate
[Watchdog] *** dep-sile 'S1' is DOWN [silent]: pid 100 is alive and its box is online, but it has sent nothing for 0s after heartbeating every 5s
HOOK offline dep-silent-0001 silent
[Watchdog]   killed pid 100 on 'boxA'
SPINUP box=boxA map=m region=r args=-a -DashboardDeploymentId=dep-silent-0001 name=S1
[Watchdog]   replacement requested on 'boxA' (request REQ), restart 1/3
STATE dep-silent-0001 box=boxA pid=0 verdict=restarting (silent) restarts=1 retired=False reason= hb=False name=S1
WIRE {"type":"kill","pid":"100","reason":"silent"}
` + loop.String() + `[Watchdog] dep-loop launched on 'boxA' pid=203
[Watchdog] dep-loop pid 203 exited (code -1) -- the agent owns the process, so this is certain
[Watchdog] *** dep-loop 'Loop' GIVING UP: 3 restarts inside 30m -- it is not coming back on its own. Last verdict was process-exited: the agent reported pid 203 exited with code -1
WIRE <none>
STATE dep-loop-0002 box=boxA pid=203 verdict=GIVING UP restarts=3 retired=True reason=3 restarts inside 30m -- it is not coming back on its own hb=False name=Loop
STATE dep-silent-0001 box=boxA pid=0 verdict=restarting (silent) restarts=1 retired=False reason= hb=False name=S1
`)
}

// TestWatchdogDecisionTimings covers the remaining verdict branches with the
// default knobs on a fake clock.
func TestWatchdogDecisionTimings(t *testing.T) {
	h := newWdHarness(t, currentKnobs())
	wd := h.wd
	verdict := func(id string) string {
		for _, w := range wd.Servers() {
			if w.DeploymentId == id {
				return w.LastVerdict
			}
		}
		return "<missing>"
	}
	// never heartbeated: starting until StartupGraceMs, then holding
	wd.OnSpinUpRequested("boot", "rb", "boxA", nil, nil, "", nil)
	wd.Evaluate()
	if v := verdict("boot"); v != "starting (no heartbeat yet)" {
		t.Fatal(v)
	}
	h.clk.advance(300_000)
	wd.Evaluate()
	if v := verdict("boot"); v != "holding: never heartbeated, so we never knew it was up" {
		t.Fatal(v)
	}
	wd.Forget("boot")

	// silent, but the box only just came back: agent ConnectedAt is recent
	wd.OnSpinUpRequested("sil", "rs", "boxB", nil, nil, "", nil)
	wd.OnSpunUp("rs", "7", "ok", "boxB")
	wd.OnHeartbeat("sil", 7, 1, 0, 0, 0, 0, nil)
	h.clk.advance(100_000)
	wd.Evaluate()
	if v := verdict("sil"); v != "holding: agent for this box is not connected" {
		t.Fatal(v)
	}
	h2 := dialAgent(t, h.hub.Addr()) // the box (re)connects 100s after the last heartbeat
	h2.send(`{"type":"hello","box":"boxB","capacity":1,"running":0}` + "\n")
	h.clk.advance(79_999)
	wd.Evaluate()
	if v := verdict("sil"); v != "quiet for 180s -- waiting" {
		t.Fatal(v)
	}
	h.clk.advance(1)
	wd.Evaluate()
	if v := verdict("sil"); v != "silent, but its box only came back 80s ago -- waiting for a fresh heartbeat" {
		t.Fatal(v)
	}
	// box up long enough: probe, then a "not alive" answer -> process-exited
	h.clk.advance(100_000)
	wd.Evaluate()
	if v := verdict("sil"); v != "silent for 280s -- asking the agent whether pid 7 is alive" {
		t.Fatal(v)
	}
	if l := h2.readLine(); l != `{"type":"probe","pid":"7"}` {
		t.Fatal(l)
	}
	// no resend within 2 ticks
	h.clk.advance(20_000)
	wd.Evaluate()
	h.clk.advance(1)
	wd.Evaluate() // > 20s since the probe: resend
	if l := h2.readLine(); l != `{"type":"probe","pid":"7"}` {
		t.Fatal(l)
	}
	// the agent answers through the wire: not alive
	h2.send(`{"type":"probe_result","pid":"7","alive":false}` + "\n")
	wd.Evaluate()
	if v := verdict("sil"); v != "restarting (process-exited)" {
		t.Fatal(v)
	}
	if l := h2.readLine(); l != `{"type":"kill","pid":"7","reason":"process-exited"}` {
		t.Fatal(l)
	}

	// two boxes claiming one pid: never guess
	wd.OnHeartbeat("twin", 4242, 1, 0, 0, 0, 0, nil)
	wd.OnProbeResult("boxA", "4242", true)
	wd.OnProbeResult("boxB", "4242", true)
	wd.Evaluate()
	if v := verdict("twin"); v != "holding: pid 4242 was claimed by 2 boxes -- will not guess" {
		t.Fatal(v)
	}
	// Stop with several claims and several agents: not killed
	if r := wd.Stop("twin", "bye"); r != "retired; pid 4242 has no known box, so it was not killed" {
		t.Fatal(r)
	}
	// a hook that panics is logged, the restart continues
	TakeDeploymentOffline = func(string, string) { panic(fmt.Errorf("db down")) }
	wd.OnSpinUpRequested("hk", "rh", "boxA", nil, nil, "", strp("H"))
	wd.OnSpunUp("rh", "9", "ok", "boxA")
	wd.OnExited("boxA", "9", "0")
	found := false
	for _, l := range h.c.get() {
		if l == "[Watchdog]   could not take hk offline: db down" {
			found = true
		}
	}
	if !found {
		t.Fatal("hook failure not logged")
	}
}

func TestWatchdogEnvKnobs(t *testing.T) {
	t.Setenv("HALCYON_WD_X", " 42 ")
	if env("HALCYON_WD_X", 7) != 42 {
		t.Fatal("env parse")
	}
	t.Setenv("HALCYON_WD_X", "0")
	if env("HALCYON_WD_X", 7) != 7 {
		t.Fatal("non-positive must fall back")
	}
	t.Setenv("HALCYON_WD_X", "abc")
	if env("HALCYON_WD_X", 7) != 7 {
		t.Fatal("invalid must fall back")
	}
	if heartbeatGraceMs != 60_000 || freezeDispatchMs != 90_000 || silentMs != 180_000 || startupGraceMs != 300_000 ||
		cooldownMs != 180_000 || maxRestarts != 3 || restartWindowMs != 1_800_000 || evaluateEveryMs != 10_000 {
		t.Skip("knob env overrides set in this environment")
	}
}

// TestWatchdogLoopStops checks the evaluate goroutine runs at once and stops on close.
func TestWatchdogLoopStops(t *testing.T) {
	c := installCapture(t)
	hub := newAgentHubAddr("127.0.0.1:0")
	k := currentKnobs()
	k.evaluateEveryMs = 5
	wd := newServerWatchdog(hub, func(b, m, r, a, n *string) *SpinUpResult { return NewSpinUpResult() }, dotnet.UtcNow, k, true)
	wd.OnHeartbeat("x", 1, 1, 0, 0, 0, 0, nil)
	waitFor(t, func() bool { return wd.Servers()[0].LastVerdict == "asking 0 box(es) which one runs pid 1" })
	wd.close()
	wd.close() // idempotent
	_ = c
}
