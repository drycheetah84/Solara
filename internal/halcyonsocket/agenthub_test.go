package halcyonsocket

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"solara/internal/dotnet"
)

// TestAgentHubProtocolOracle replays tools/oracle-halcyonsocket "hub" mode
// (the ORIGINAL AgentHub driven by a TCP fake agent) and compares every line.
func TestAgentHubProtocolOracle(t *testing.T) {
	c := installCapture(t)
	hub := newAgentHubAddr("127.0.0.1:0")
	hub.SetOnProbeResult(func(b, p string, al bool) {
		consoleWriteLine(fmt.Sprintf("CB probe_result box=%s pid=%s alive=%s", b, p, dotnet.FormatBool(al)))
	})
	hub.SetOnSpunUp(func(r, p, s, b string) {
		consoleWriteLine(fmt.Sprintf("CB spunup req=%s pid=%s status=%s box=%s", r, p, s, b))
	})
	hub.SetOnExited(func(b, p, code string) { consoleWriteLine(fmt.Sprintf("CB exited box=%s pid=%s code=%s", b, p, code)) })
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.close()
	log := consoleWriteLine
	agents := func() {
		for _, a := range hub.Agents() {
			log(fmt.Sprintf("AGENT box=%s cap=%d running=%d free=%d", a.Box(), a.Capacity(), a.Running(), a.Free()))
		}
	}
	pick := func(b *string) string {
		if a := hub.Pick(b); a != nil {
			return a.Box()
		}
		return "<none>"
	}
	ag := dialAgent(t, hub.Addr())
	waitFor(t, func() bool { return len(hub.Agents()) == 1 })
	agents()
	ag.send(`{"type":"hel`)
	ag.send(`lo","box":"boxA","capacity":2,"running":0}` + "\r\n")
	agents()
	log("PICK null => " + pick(nil))
	log("PICK empty => " + pick(strp("")))
	log("PICK boxA => " + pick(strp("boxA")))
	log("PICK boxZ => " + pick(strp("boxZ")))
	log("PROBE => " + dotnet.FormatBool(hub.Probe("boxA", "123")))
	log("WIRE " + ag.readLine())
	log("KILL => " + dotnet.FormatBool(hub.Kill("boxA", "123", "engine-frozen <&>")))
	log("WIRE " + ag.readLine())
	log(fmt.Sprintf("PROBEALL => %d", hub.ProbeAll("9")))
	log("WIRE " + ag.readLine())
	log("KILL none => " + dotnet.FormatBool(hub.Kill("nobox", "1", "x")))
	ag.send(`{"type":"heartbeat","running":"1"}` + "\n")
	agents()
	ag.send(`{"type":"heartbeat","running":1}` + "\n")
	agents()
	ag.send(`{"type":"hello","box":"boxB","capacity":"x","running":0}` + "\n")
	agents()
	ag.send(`{"type":"hello","box":"boxB","capacity":2.5,"running":2}` + "\n")
	agents()
	ag.send(`{"type":"hello","box":"boxB","capacity":3,"running":true}` + "\n")
	agents()
	ag.send("[1]\n garbage \n\n   \n" + `{"type":"probe_result","pid":77,"alive":true}` + "\n" + `{"type":"probe_result","pid":"78","alive":"true"}` + "\n")
	ag.send(`{"type":"spunup","id":"r1","pid":"4242","status":"ok","msg":"launched"}` + "\n" + `{"type":"spunup","id":true,"status":null}` + "\n")
	ag.send(`{"type":"exited","pid":"4242","code":"killed","running":1}` + "\n" + `{"type":"exited","pid":"5","code":"x","running":"1"}` + "\n")
	ag.send(`{"type":"heartbeat","running":0}{"type":"x"}` + "\n")
	agents()
	hb := []byte(`{"type":"hello","box":"caf` + string(rune(0xE9)) + `","capacity":1,"running":0}` + "\n")
	cut := strings.IndexByte(string(hb), 0xC3) + 1
	ag.send(string(hb[:cut]))
	ag.send(string(hb[cut:]))
	agents()
	ag.send(string([]byte{0x7b, 0x22, 0x74, 0x79, 0x70, 0x65, 0x22, 0x3a, 0x22, 0x68, 0x65, 0x6c, 0x6c, 0x6f, 0x22, 0x2c, 0x22, 0x62, 0x6f, 0x78, 0x22, 0x3a, 0x22, 0xE2, 0x82, 0x41, 0xF0, 0x9F, 0x80, 0xC0, 0x22, 0x7d, 0x0a}))
	agents()
	_ = ag.c.Close()
	waitLine(t, c, "disconnected")
	agents()
	log("END")

	got := c.get()
	if !strings.HasPrefix(got[0], "[HalcyonSocket] agent hub listening on TCP ") {
		t.Fatalf("first line %q", got[0])
	}
	want := strings.Split(strings.TrimSpace(`
AGENT box=? cap=0 running=0 free=0
[HalcyonSocket] agent 'boxA' online (cap=2, running=0)
AGENT box=boxA cap=2 running=0 free=2
PICK null => boxA
PICK empty => boxA
PICK boxA => boxA
PICK boxZ => <none>
PROBE => True
WIRE {"type":"probe","pid":"123"}
KILL => True
WIRE {"type":"kill","pid":"123","reason":"engine-frozen %u003C%u0026%u003E"}
PROBEALL => 1
WIRE {"type":"probe","pid":"9"}
KILL none => False
AGENT box=boxA cap=2 running=0 free=2
AGENT box=boxA cap=2 running=1 free=1
AGENT box=boxB cap=2 running=1 free=1
[HalcyonSocket] agent 'boxB' online (cap=0, running=2)
AGENT box=boxB cap=0 running=2 free=0
AGENT box=boxB cap=3 running=2 free=1
CB probe_result box=boxB pid=77 alive=True
CB probe_result box=boxB pid=78 alive=False
[HalcyonSocket] spunup id=r1 pid=4242 status=ok msg=launched
CB spunup req=r1 pid=4242 status=ok box=boxB
[HalcyonSocket] spunup id=True pid= status= msg=
CB spunup req=True pid= status= box=boxB
[HalcyonSocket] instance exited pid=4242 code=killed
CB exited box=boxB pid=4242 code=killed
AGENT box=boxB cap=3 running=1 free=2
[HalcyonSocket] agent 'caf<FFFD><FFFD>' online (cap=1, running=0)
AGENT box=caf<FFFD><FFFD> cap=1 running=0 free=1
[HalcyonSocket] agent '<FFFD>A<FFFD><FFFD>' online (cap=0, running=0)
AGENT box=<FFFD>A<FFFD><FFFD> cap=0 running=0 free=0
[HalcyonSocket] agent '<FFFD>A<FFFD><FFFD>' disconnected
END`), "\n")
	for i := range want {
		want[i] = u(want[i])
	}
	compareLines(t, got[1:], want)
}

// TestAgentHubReap checks the 90s stale-agent reaper and the disconnect log.
func TestAgentHubReap(t *testing.T) {
	c := installCapture(t)
	clk := newFakeClock()
	hub := newAgentHubAddr("127.0.0.1:0")
	hub.now = clk.now
	hub.reapEvery = 20e6 // 20ms instead of 15s
	if err := hub.Start(); err != nil {
		t.Fatal(err)
	}
	defer hub.close()
	ag := dialAgent(t, hub.Addr())
	ag.send(`{"type":"hello","box":"old","capacity":1,"running":0}` + "\n")
	clk.advance(90_000) // exactly at the cutoff: LastSeen >= cutoff keeps it
	ag.send("")
	if len(hub.Agents()) != 1 {
		t.Fatal("reaped too early")
	}
	clk.advance(1)
	waitFor(t, func() bool { return len(hub.Agents()) == 0 })
	if ag.readLine() != "<none>" { // connection closed by the hub
		t.Log("closed")
	}
	waitLine(t, c, "disconnected")
	waitLine(t, c, "reaped stale agent 'old'")
	// The reaper logs after closing the socket and the reader logs "disconnected"
	// when it notices: the two lines race in the original too.
	got := c.get()[1:]
	sort.Strings(got)
	compareLines(t, got, []string{
		"[HalcyonSocket] agent 'old' disconnected",
		"[HalcyonSocket] agent 'old' online (cap=1, running=0)",
		"[HalcyonSocket] reaped stale agent 'old'",
	})
}
