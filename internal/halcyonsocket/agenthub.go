// Ports AUnrealFeatures.HalcyonSocket/AgentHub.cs (AgentConnection, AgentHub):
// the raw-TCP hub on :9100 that HalcyonAllocatorAgent / HalcyonAgent.ps1
// connects to. Newline-delimited JSON both directions. Solara adds optional
// agent authentication, a line cap, box-name hijack protection and launch
// ownership checks (agenthub_security.go; KNOWN_ISSUES F6).
package halcyonsocket

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// AgentConnection is one connected HalcyonAllocatorAgent (a game box).
// The C# properties are exposed as methods so the reader goroutine, the reaper
// and the watchdog can share them without data races.
type AgentConnection struct {
	Client net.Conn // TcpClient + NetworkStream

	mu       sync.Mutex
	box      string
	capacity int
	running  int
	lastSeen dotnet.DateTime

	// When this connection came up. The watchdog waits this long again before
	// judging a server on SILENCE (see ServerWatchdog).
	connectedAt dotnet.DateTime

	writeLock sync.Mutex

	// Solara (F6): authentication state, guarded by mu.
	ip          string // remote IP (fixed at accept)
	token       string // the agent token presented in hello (kept in memory only)
	authBox     string // box the agent token is bound to ("" = no token)
	allowlisted bool   // remote IP in SOLARA_AGENT_ALLOWED_IPS
	helloOK     bool   // a hello was admitted
	warned      bool   // compat-mode "unauthenticated agent" warning printed
}

func newAgentConnection(c net.Conn, now dotnet.DateTime) *AgentConnection {
	// Box = "?", LastSeen = DateTime.UtcNow, ConnectedAt = DateTime.UtcNow
	return &AgentConnection{Client: c, box: "?", lastSeen: now, connectedAt: now}
}

func (a *AgentConnection) Box() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.box
}

func (a *AgentConnection) Capacity() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.capacity
}

func (a *AgentConnection) Running() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

func (a *AgentConnection) LastSeen() dotnet.DateTime {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastSeen
}

func (a *AgentConnection) ConnectedAt() dotnet.DateTime { return a.connectedAt }

// Free mirrors Math.Max(0, Capacity - Running).
func (a *AgentConnection) Free() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return max(0, a.capacity-a.running)
}

func (a *AgentConnection) setBox(v string) {
	a.mu.Lock()
	a.box = v
	a.mu.Unlock()
}

func (a *AgentConnection) setCapacity(v int) {
	a.mu.Lock()
	a.capacity = v
	a.mu.Unlock()
}

func (a *AgentConnection) setRunning(v int) {
	a.mu.Lock()
	a.running = v
	a.mu.Unlock()
}

func (a *AgentConnection) setLastSeen(v dotnet.DateTime) {
	a.mu.Lock()
	a.lastSeen = v
	a.mu.Unlock()
}

// SendLine writes json + "\n" (UTF-8) under the connection's write lock.
// Any failure yields false.
func (a *AgentConnection) SendLine(json string) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	b := []byte(json + "\n")
	a.writeLock.Lock()
	defer a.writeLock.Unlock()
	if _, err := a.Client.Write(b); err != nil {
		return false
	}
	return true
}

// AgentHub is the raw-TCP hub for allocator agents (accept / heartbeat / reap).
type AgentHub struct {
	addr     string
	listener net.Listener

	mu     sync.Mutex
	agents map[string]*AgentConnection // Guid -> connection
	order  []string                    // enumeration order (see report: insertion order)

	// Set by HalcyonSocketServer once the watchdog exists, so the hub stays a
	// dumb transport and every decision lives in ServerWatchdog.
	onSpunUp      func(reqId, pid, status, box string)
	onExited      func(box, pid, code string)
	onProbeResult func(box, pid string, alive bool)

	now           func() dotnet.DateTime
	reapEvery     time.Duration
	staleAfterSec float64

	stop    chan struct{}
	stopped bool
	wg      sync.WaitGroup

	// Solara (F6): launches this hub sent, so spunup/exited reports can be
	// checked against the connection that launched them (guarded by mu).
	launches map[string]launchRec // request id -> launching connection
	pids     map[string]launchRec // "box/pid" -> launching connection
	// ipOf extracts a connection's remote IP (tests substitute it).
	ipOf func(net.Conn) string
}

// NewAgentHub mirrors new AgentHub(port): a TcpListener on IPAddress.Any.
func NewAgentHub(port int) *AgentHub { return newAgentHubAddr(fmt.Sprintf(":%d", port)) }

func newAgentHubAddr(addr string) *AgentHub {
	return &AgentHub{
		addr:          addr,
		agents:        map[string]*AgentConnection{},
		now:           dotnet.UtcNow,
		reapEvery:     15000 * time.Millisecond,
		staleAfterSec: 90,
		stop:          make(chan struct{}),
		launches:      map[string]launchRec{},
		pids:          map[string]launchRec{},
		ipOf:          remoteIP,
	}
}

// SetOnSpunUp / SetOnExited / SetOnProbeResult mirror assigning the public
// Action fields of the original.
func (h *AgentHub) SetOnSpunUp(f func(reqId, pid, status, box string)) {
	h.mu.Lock()
	h.onSpunUp = f
	h.mu.Unlock()
}

func (h *AgentHub) SetOnExited(f func(box, pid, code string)) {
	h.mu.Lock()
	h.onExited = f
	h.mu.Unlock()
}

func (h *AgentHub) SetOnProbeResult(f func(box, pid string, alive bool)) {
	h.mu.Lock()
	h.onProbeResult = f
	h.mu.Unlock()
}

// Start binds the listener (a failure is returned, as TcpListener.Start threw)
// and starts the accept and reap loops.
func (h *AgentHub) Start() error {
	ln, err := net.Listen("tcp", h.addr)
	if err != nil {
		return err
	}
	h.listener = ln
	h.wg.Add(2)
	go h.acceptLoop()
	go h.reapLoop()
	consoleWriteLine(fmt.Sprintf("[HalcyonSocket] agent hub listening on TCP %d", ln.Addr().(*net.TCPAddr).Port))
	return nil
}

// Addr returns the bound address (tests).
func (h *AgentHub) Addr() string {
	if h.listener == nil {
		return ""
	}
	return h.listener.Addr().String()
}

func (h *AgentHub) acceptLoop() {
	defer h.wg.Done()
	for {
		c, err := h.listener.Accept()
		if err != nil {
			break
		}
		h.addConn(c)
	}
}

// addConn registers an accepted connection and starts its reader.
func (h *AgentHub) addConn(c net.Conn) *AgentConnection {
	conn := newAgentConnection(c, h.now())
	conn.ip = h.ipOf(c)
	id := dotnet.NewGuid()
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		_ = c.Close()
		return conn
	}
	h.agents[id] = conn
	h.order = append(h.order, id)
	h.wg.Add(1)
	h.mu.Unlock()
	go h.clientLoop(id, conn)
	return conn
}

func (h *AgentHub) remove(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.agents[id]; !ok {
		return false
	}
	delete(h.agents, id)
	for i, x := range h.order {
		if x == id {
			h.order = append(h.order[:i:i], h.order[i+1:]...)
			break
		}
	}
	return true
}

func (h *AgentHub) clientLoop(id string, conn *AgentConnection) {
	defer h.wg.Done()
	buf := make([]byte, 4096)
	acc := ""
	func() {
		defer func() { _ = recover() }() // catch { /* fallthrough to disconnect */ }
		for {
			// .NET ReadAsync returns 0 at EOF and throws on error; either way the
			// loop ends (bytes delivered together with an error are processed first).
			n, err := conn.Client.Read(buf)
			if n <= 0 {
				return
			}
			// Each read is decoded on its own (Encoding.UTF8.GetString(buf, 0, n)).
			s := acc + decodeUTF8(buf[:n])
			for {
				nl := strings.IndexByte(s, '\n')
				if nl < 0 {
					break
				}
				if nl > maxAgentLine { // Solara (F6): bounded lines
					securityLog(fmt.Sprintf("[HalcyonSocket] agent '%s' (%s) sent a line over %d bytes -- disconnecting", conn.Box(), conn.ip, maxAgentLine))
					return
				}
				line := trim(s[:nl])
				s = s[nl+1:]
				if len(line) > 0 {
					if drop := h.handleAgentMessage(conn, line); drop {
						return
					}
				}
			}
			// Solara (F6): the original buffered an unterminated line without
			// limit (memory exhaustion); disconnect instead.
			if len(s) > maxAgentLine {
				securityLog(fmt.Sprintf("[HalcyonSocket] agent '%s' (%s) sent over %d bytes without a newline -- disconnecting", conn.Box(), conn.ip, maxAgentLine))
				return
			}
			acc = s
			if err != nil {
				return
			}
		}
	}()

	h.remove(id)
	_ = conn.Client.Close()
	consoleWriteLine(fmt.Sprintf("[HalcyonSocket] agent '%s' disconnected", conn.Box()))
}

// getStr mirrors GetStr: TryGetProperty(k) ? v.ToString() : "".
func getStr(e stj.Element, k string) string {
	if v, ok := e.TryGetProperty(k); ok {
		return v.ToString()
	}
	return ""
}

// getInt mirrors GetInt: TryGetProperty(k) && v.TryGetInt32(out i) ? i : 0.
// TryGetInt32 on a non-number throws InvalidOperationException, which aborts
// the whole message (caught by handleAgentMessage).
func getInt(e stj.Element, k string) int {
	if v, ok := e.TryGetProperty(k); ok {
		if i, ok := v.TryGetInt32(); ok {
			return int(i)
		}
	}
	return 0
}

// handleAgentMessage processes one line. drop=true asks the reader to
// disconnect the agent (Solara: refused authentication / hijack / revoked token).
func (h *AgentHub) handleAgentMessage(conn *AgentConnection, line string) (drop bool) {
	defer func() {
		if recover() != nil { // catch { /* ignore malformed */ }
			drop = false
		}
	}()
	root := stj.MustParse([]byte(line))
	typ := getStr(root, "type")
	// Solara (F6): in strict mode nothing but hello is processed before an
	// authenticated hello was admitted.
	if typ != "hello" && !conn.HelloOK() && agentAuthRequired() {
		return false
	}
	switch typ {
	case "hello":
		if !h.admitHello(conn, getStr(root, "box"), getStr(root, "token")) {
			return true
		}
		conn.setBox(getStr(root, "box"))
		conn.setCapacity(getInt(root, "capacity"))
		conn.setRunning(getInt(root, "running"))
		conn.setLastSeen(h.now())
		consoleWriteLine(fmt.Sprintf("[HalcyonSocket] agent '%s' online (cap=%d, running=%d)", conn.Box(), conn.Capacity(), conn.Running()))
	case "heartbeat":
		if !h.recheckAgentToken(conn) {
			return true
		}
		conn.setRunning(getInt(root, "running"))
		conn.setLastSeen(h.now())
	case "spunup":
		consoleWriteLine(fmt.Sprintf("[HalcyonSocket] spunup id=%s pid=%s status=%s msg=%s",
			getStr(root, "id"), getStr(root, "pid"), getStr(root, "status"), getStr(root, "msg")))
		// Solara (F6): a launch this hub sent is only answered by its launcher.
		if !h.acceptSpunUp(conn, getStr(root, "id"), getStr(root, "pid"), getStr(root, "status")) {
			return false
		}
		h.mu.Lock()
		f := h.onSpunUp
		h.mu.Unlock()
		if f != nil {
			f(getStr(root, "id"), getStr(root, "pid"), getStr(root, "status"), conn.Box())
		}
	case "exited":
		conn.setRunning(getInt(root, "running"))
		consoleWriteLine(fmt.Sprintf("[HalcyonSocket] instance exited pid=%s code=%s", getStr(root, "pid"), getStr(root, "code")))
		// Solara (F6): a pid this hub saw launched is only reported by its launcher.
		if !h.acceptExited(conn, getStr(root, "pid")) {
			return false
		}
		// The agent holds the Process object, so this is not a guess about the
		// process -- it is the process telling us. The watchdog treats it as certain.
		h.mu.Lock()
		f := h.onExited
		h.mu.Unlock()
		if f != nil {
			f(conn.Box(), getStr(root, "pid"), getStr(root, "code"))
		}
	case "probe_result":
		h.mu.Lock()
		f := h.onProbeResult
		h.mu.Unlock()
		if f != nil {
			box, pid := conn.Box(), getStr(root, "pid")
			al, ok := root.TryGetProperty("alive")
			f(box, pid, ok && al.ValueKind() == stj.True)
		}
	}
	return false
}

// reapLoop reaps agents that haven't heartbeated in 90s (the agent beats every 30s).
func (h *AgentHub) reapLoop() {
	defer h.wg.Done()
	for {
		select {
		case <-h.stop:
			return
		case <-time.After(h.reapEvery):
		}
		cutoff := h.now().AddSeconds(-h.staleAfterSec)
		h.mu.Lock()
		type kv struct {
			id string
			c  *AgentConnection
		}
		snap := make([]kv, 0, len(h.order))
		for _, id := range h.order {
			snap = append(snap, kv{id, h.agents[id]})
		}
		h.mu.Unlock()
		for _, e := range snap {
			if !e.c.LastSeen().Before(cutoff) {
				continue
			}
			h.remove(e.id)
			_ = e.c.Client.Close()
			consoleWriteLine(fmt.Sprintf("[HalcyonSocket] reaped stale agent '%s'", e.c.Box()))
		}
	}
}

// Agents mirrors `_agents.Values.ToList()` (a snapshot).
func (h *AgentHub) Agents() []*AgentConnection {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*AgentConnection, 0, len(h.order))
	for _, id := range h.order {
		out = append(out, h.agents[id])
	}
	return out
}

// ByBox returns the agent for a named box, or nil when that box is not
// connected (the watchdog treats nil as "cannot verify").
func (h *AgentHub) ByBox(box string) *AgentConnection {
	if isNullOrWhiteSpace(box) {
		return nil
	}
	for _, a := range h.Agents() {
		if a.Box() == box {
			return a
		}
	}
	return nil
}

// Probe asks a box whether a pid is still running ("probe_result" answers).
func (h *AgentHub) Probe(box, pid string) bool {
	a := h.ByBox(box)
	return a != nil && a.SendLine(stj.MarshalString(stj.O("type", "probe", "pid", pid)))
}

// ProbeAll asks EVERY connected box about a pid; returns how many were asked.
func (h *AgentHub) ProbeAll(pid string) int {
	line := stj.MarshalString(stj.O("type", "probe", "pid", pid))
	n := 0
	for _, a := range h.Agents() {
		if a.SendLine(line) {
			n++
		}
	}
	return n
}

// Kill force-stops a pid on a box.
func (h *AgentHub) Kill(box, pid, reason string) bool {
	a := h.ByBox(box)
	return a != nil && a.SendLine(stj.MarshalString(stj.O("type", "kill", "pid", pid, "reason", reason)))
}

// Pick picks a target agent: exact box name if given (and it has free
// capacity), else the box with the most free capacity (stable on ties).
func (h *AgentHub) Pick(box *string) *AgentConnection {
	list := h.Agents()
	if !isNullOrEmptyP(box) {
		for _, a := range list {
			if a.Box() == *box && a.Free() > 0 {
				return a
			}
		}
		return nil
	}
	var best *AgentConnection
	bestFree := 0
	for _, a := range list {
		if f := a.Free(); f > 0 && (best == nil || f > bestFree) {
			best, bestFree = a, f
		}
	}
	return best
}

// close stops the hub: the listener, every connection and the reaper
// (Solara addition; the original never stopped the hub).
func (h *AgentHub) close() {
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		return
	}
	h.stopped = true
	conns := make([]*AgentConnection, 0, len(h.agents))
	for _, id := range h.order {
		conns = append(conns, h.agents[id])
	}
	h.mu.Unlock()
	close(h.stop)
	if h.listener != nil {
		_ = h.listener.Close()
	}
	for _, c := range conns {
		_ = c.Client.Close()
	}
	h.wg.Wait()
}
