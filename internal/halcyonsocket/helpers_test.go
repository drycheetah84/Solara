package halcyonsocket

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"

	"solara/internal/dotnet"
	"solara/internal/logging"
)

func init() { logging.DisableFiles = true }

// u expands the "%u" placeholder to a JSON unicode escape introducer (keeps
// literal escape sequences out of the source text; see PORTING_GUIDE §0).
func u(s string) string { return strings.ReplaceAll(s, "%u", "\\"+"u") }

// esc mirrors the oracle's Esc(): UTF-16 code units outside printable ASCII
// are rendered as <XXXX>.
func esc(s string) string {
	var b strings.Builder
	for _, c := range utf16.Encode([]rune(s)) {
		if c < 0x20 || c > 0x7e {
			fmt.Fprintf(&b, "<%04X>", c)
		} else {
			b.WriteByte(byte(c))
		}
	}
	return b.String()
}

// capture collects Console.WriteLine output. Install it before starting any
// goroutine of the package and restore it only after they have stopped.
type capture struct {
	mu    sync.Mutex
	lines []string
}

func (c *capture) add(s string) {
	c.mu.Lock()
	c.lines = append(c.lines, esc(s))
	c.mu.Unlock()
}

func (c *capture) get() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

func installCapture(t *testing.T) *capture {
	c := &capture{}
	prev := consoleWriteLine
	consoleWriteLine = c.add
	t.Cleanup(func() { consoleWriteLine = prev })
	return c
}

func compareLines(t *testing.T, got, want []string) {
	t.Helper()
	n := max(len(got), len(want))
	bad := false
	for i := 0; i < n; i++ {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			t.Errorf("line %d:\n got: %q\nwant: %q", i, g, w)
			bad = true
		}
	}
	if bad {
		t.Logf("full output:\n%s", strings.Join(got, "\n"))
	}
}

// fakeClock is an injectable DateTime.UtcNow.
type fakeClock struct {
	mu sync.Mutex
	t  dotnet.DateTime
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: dotnet.New(2026, 10, 25, 12, 0, 0, dotnet.Utc)}
}

func (c *fakeClock) now() dotnet.DateTime {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(ms float64) {
	c.mu.Lock()
	c.t = c.t.AddMilliseconds(ms)
	c.mu.Unlock()
}

// fakeAgent is a TCP client speaking the HalcyonAgent.ps1 side of the protocol.
type fakeAgent struct {
	c  net.Conn
	rd *bufio.Reader
}

func dialAgent(t *testing.T, addr string) *fakeAgent {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeAgent{c: c, rd: bufio.NewReader(c)}
}

// send writes raw bytes then pauses, so each send is one read on the hub side
// (as in the oracle run).
func (a *fakeAgent) send(s string) {
	_, _ = a.c.Write([]byte(s))
	time.Sleep(150 * time.Millisecond)
}

// readLine mirrors StreamReader.ReadLine with a 500ms timeout ("<none>").
func (a *fakeAgent) readLine() string {
	_ = a.c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	line, err := a.rd.ReadString('\n')
	if err != nil {
		return "<none>"
	}
	return strings.TrimRight(strings.TrimSuffix(line, "\n"), "\r")
}

// waitFor polls cond for up to 3s.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

// waitLine waits until a captured line has the given suffix.
func waitLine(t *testing.T, c *capture, suffix string) {
	t.Helper()
	waitFor(t, func() bool {
		for _, l := range c.get() {
			if strings.HasSuffix(l, suffix) {
				return true
			}
		}
		return false
	})
}
