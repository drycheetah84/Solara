// Ports AUnrealFeatures.HalcyonSocket/ServerWatchdog.cs.
//
// The watchdog replaces a game server only when it is CERTAIN the old one is
// gone. It acts on exactly three verdicts:
//
//	process-exited  the agent that owns the process says it exited.
//	engine-frozen   heartbeats still arrive but report no UFunction dispatch
//	                for FreezeDispatchMs.
//	silent          the agent confirms the process alive, its box is up, and
//	                the server has sent nothing for SilentMs.
//
// Everything else (agent not connected, start-up grace, never heartbeated) is
// a logged hold. Restarts reuse the SAME deployment id. A circuit breaker
// retires a deployment after MaxRestarts inside RestartWindow.
//
// Concurrency: the original mutated Watched fields from several threads with
// no lock (only BoxClaims was locked). Solara guards all watchdog state with
// one mutex, released around every call that leaves the watchdog (agent
// sends, the TakeDeploymentOffline hook, the spin-up callback), so the
// interleavings the original allowed are preserved without data races.
package halcyonsocket

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/serverid"
)

// env mirrors ServerWatchdog.Env: a positive int from the environment, else the fallback.
func env(name string, fallback int) int {
	if v, ok := parseInt32(os.Getenv(name)); ok && v > 0 {
		return v
	}
	return fallback
}

// Knobs (env-overridable so a box can be tuned without a rebuild). Like the
// C# static readonly fields they are read once, at package initialisation.
// A heartbeat lands every 5s (HalcyonA2 kHeartbeatMs).
var (
	heartbeatGraceMs = env("HALCYON_WD_HB_GRACE_MS", 60_000)  // still "arriving" within this
	freezeDispatchMs = env("HALCYON_WD_FREEZE_MS", 90_000)    // no UFunction dispatch = wedged
	silentMs         = env("HALCYON_WD_SILENT_MS", 180_000)   // alive but saying nothing
	startupGraceMs   = env("HALCYON_WD_STARTUP_MS", 300_000)  // let a fresh server boot
	cooldownMs       = env("HALCYON_WD_COOLDOWN_MS", 180_000) // after we act, leave it alone
	maxRestarts      = env("HALCYON_WD_MAX_RESTARTS", 3)      //
	restartWindowMs  = env("HALCYON_WD_WINDOW_MS", 1_800_000) // 30 min
	evaluateEveryMs  = env("HALCYON_WD_TICK_MS", 10_000)      //
)

// TakeDeploymentOffline is wired by the Ares host at start-up: (deploymentId,
// reason). Dropping the EOS session and marking the DB row offline lives over
// there. Nil by default.
var TakeDeploymentOffline func(deploymentId string, reason string)

// Watched is the watchdog's state for one deployment.
type Watched struct {
	DeploymentId  string
	Box           string
	Name          string
	Map, Region   *string
	Args          string
	Pid           int
	CreatedAt     dotnet.DateTime
	LastHeartbeat *dotnet.DateTime
	LastSeq       uint64
	DispatchAgeMs uint64
	TickAgeMs     uint64
	UptimeMs      uint64
	Players       int

	LastRestartAt    *dotnet.DateTime
	WindowStart      dotnet.DateTime
	RestartsInWindow int
	Retired          bool // circuit breaker tripped, or stopped on purpose
	RetiredReason    string
	LastVerdict      string

	// probe handshake with the agent: we ask "is this pid alive?" and read the answer next pass.
	ProbeSentAt     *dotnet.DateTime
	ProbeAlive      *bool
	ProbeAnsweredAt *dotnet.DateTime

	// Adopting a server we did not launch: boxes that have claimed this pid as theirs.
	AdoptAskedAt *dotnet.DateTime
	BoxClaims    map[string]struct{}
}

func newWatched(deploymentId string, now dotnet.DateTime) *Watched {
	return &Watched{DeploymentId: deploymentId, CreatedAt: now, Players: -1, WindowStart: now,
		LastVerdict: "starting", BoxClaims: map[string]struct{}{}}
}

// clone returns a detached snapshot (Servers hands out snapshots so readers
// never race the evaluate loop).
func (w *Watched) clone() *Watched {
	c := *w
	c.BoxClaims = make(map[string]struct{}, len(w.BoxClaims))
	for k := range w.BoxClaims {
		c.BoxClaims[k] = struct{}{}
	}
	return &c
}

// knobs is a per-instance copy of the package knobs (taken at construction).
type knobs struct {
	heartbeatGraceMs, freezeDispatchMs, silentMs, startupGraceMs, cooldownMs,
	maxRestarts, restartWindowMs, evaluateEveryMs int
}

func currentKnobs() knobs {
	return knobs{heartbeatGraceMs, freezeDispatchMs, silentMs, startupGraceMs, cooldownMs,
		maxRestarts, restartWindowMs, evaluateEveryMs}
}

// SpinUpFunc is the (box, map, region, args, name) => SpinUpResult callback.
type SpinUpFunc func(box, mp, region, args, name *string) *SpinUpResult

// ServerWatchdog watches every deployment.
type ServerWatchdog struct {
	mu              sync.Mutex
	servers         map[string]*Watched // deploymentId -> state
	order           []string            // enumeration order
	reqToDeployment map[string]string   // spin-up request id -> deployment
	pidToDeployment map[string]string   // "box/pid" -> deployment

	hub    *AgentHub
	spinUp SpinUpFunc
	k      knobs
	now    func() dotnet.DateTime

	stop chan struct{}
	done chan struct{}
}

// NewServerWatchdog mirrors the constructor: the evaluate loop starts at once.
func NewServerWatchdog(hub *AgentHub, spinUp SpinUpFunc) *ServerWatchdog {
	return newServerWatchdog(hub, spinUp, dotnet.UtcNow, currentKnobs(), true)
}

func newServerWatchdog(hub *AgentHub, spinUp SpinUpFunc, now func() dotnet.DateTime, k knobs, loop bool) *ServerWatchdog {
	wd := &ServerWatchdog{
		servers:         map[string]*Watched{},
		reqToDeployment: map[string]string{},
		pidToDeployment: map[string]string{},
		hub:             hub,
		spinUp:          spinUp,
		k:               k,
		now:             now,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
	}
	if loop {
		go wd.evaluateLoop()
	} else {
		close(wd.done)
	}
	return wd
}

func wdLog(m string) { consoleWriteLine("[Watchdog] " + m) }

// Short mirrors `id.Length > 8 ? id[..8] : id`.
func Short(id string) string {
	if utf16Len(id) > 8 {
		return utf16Prefix(id, 8)
	}
	return id
}

// Servers mirrors `_servers.Values.ToList()` (as detached snapshots).
func (wd *ServerWatchdog) Servers() []*Watched {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	out := make([]*Watched, 0, len(wd.order))
	for _, id := range wd.order {
		out = append(out, wd.servers[id].clone())
	}
	return out
}

// getOrAdd mirrors ConcurrentDictionary.GetOrAdd (caller holds mu).
func (wd *ServerWatchdog) getOrAdd(id string, factory func() *Watched) *Watched {
	if w, ok := wd.servers[id]; ok {
		return w
	}
	w := factory()
	wd.servers[id] = w
	wd.order = append(wd.order, id)
	return w
}

// ─── inputs ─────────────────────────────────────────────────────────────────

// OnSpinUpRequested is called by RequestSpinUp for every launch, so we know how
// to put this exact server back up: same box, map, args and deployment id.
func (wd *ServerWatchdog) OnSpinUpRequested(deploymentId, reqId, box string, mp, region *string, args string, name *string) {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	w := wd.getOrAdd(deploymentId, func() *Watched { return newWatched(deploymentId, wd.now()) })
	w.Box = box
	w.Map = mp
	w.Region = region
	w.Args = args
	if !isNullOrWhiteSpaceP(name) {
		w.Name = *name
	}
	// A fresh launch restarts the start-up grace, and clears a stale pid so a
	// late "exited" for the PREVIOUS process cannot be read as this one dying.
	w.CreatedAt = wd.now()
	w.Pid = 0
	w.ProbeAlive = nil
	w.ProbeSentAt = nil
	w.LastVerdict = "starting"
	// Someone put a retired deployment up again on purpose: watch it again.
	if w.Retired {
		w.Retired = false
		w.RetiredReason = ""
		w.RestartsInWindow = 0
		w.WindowStart = wd.now()
		wdLog(fmt.Sprintf("%s spun up again on purpose: back under watch", Short(deploymentId)))
	}
	wd.reqToDeployment[reqId] = deploymentId
}

// OnSpunUp handles the agent's "spunup" answer.
func (wd *ServerWatchdog) OnSpunUp(reqId, pid, status, box string) {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	dep, ok := wd.reqToDeployment[reqId]
	if !ok {
		return
	}
	delete(wd.reqToDeployment, reqId)
	w, ok := wd.servers[dep]
	if !ok {
		return
	}
	if p, ok := parseInt32(pid); status == "ok" && ok && p > 0 {
		w.Pid = p
		wd.pidToDeployment[fmt.Sprintf("%s/%d", box, p)] = dep
		wdLog(fmt.Sprintf("%s launched on '%s' pid=%d", Short(dep), box, p))
	} else {
		// The agent refused, or the process died during init. Leave it to a human.
		w.Retired = true
		w.RetiredReason = "spin-up failed: " + status
		w.LastVerdict = "spinup-failed"
		wdLog(fmt.Sprintf("%s spin-up failed on '%s' (%s) -- not watching it", Short(dep), box, status))
	}
}

// OnExited handles the agent's "exited" report.
func (wd *ServerWatchdog) OnExited(box, pid, code string) {
	g := &lockGuard{mu: &wd.mu}
	g.lock()
	defer g.release()
	key := box + "/" + pid
	dep, ok := wd.pidToDeployment[key]
	if !ok {
		return
	}
	delete(wd.pidToDeployment, key)
	w, ok := wd.servers[dep]
	if !ok {
		return
	}
	if strconv.Itoa(w.Pid) != pid { // a stale exit for a process we already replaced
		return
	}
	f := false
	w.ProbeAlive = &f
	t := wd.now()
	w.ProbeAnsweredAt = &t
	wdLog(fmt.Sprintf("%s pid %s exited (code %s) -- the agent owns the process, so this is certain", Short(dep), pid, code))
	// Solara (SEC-SID): the process is gone, so is its token (a restart mints a new one).
	revokeDeploymentTokens(dep, "server exited (code "+code+")")
	wd.tryRestart(g, w, "process-exited", fmt.Sprintf("the agent reported pid %s exited with code %s", pid, code))
}

// OnHeartbeat is the game server's own heartbeat (POST /server_heartbeat, every 5s).
// box is optional (C# default null).
func (wd *ServerWatchdog) OnHeartbeat(deploymentId string, pid int, seq, dispatchAgeMs, tickAgeMs, uptimeMs uint64, players int, box *string) {
	if isNullOrWhiteSpace(deploymentId) {
		return
	}
	wd.mu.Lock()
	defer wd.mu.Unlock()
	w := wd.getOrAdd(deploymentId, func() *Watched {
		wdLog(fmt.Sprintf("%s is heartbeating but we did not launch it -- watching it too", Short(deploymentId)))
		return newWatched(deploymentId, wd.now())
	})
	if pid > 0 && w.Pid != pid {
		w.Pid = pid
		if !isNullOrWhiteSpace(w.Box) {
			wd.pidToDeployment[fmt.Sprintf("%s/%d", w.Box, pid)] = deploymentId
		}
		// A different process now: start the ownership question over.
		w.BoxClaims = map[string]struct{}{}
		w.AdoptAskedAt = nil
	}
	if !isNullOrWhiteSpaceP(box) && isNullOrWhiteSpace(w.Box) {
		w.Box = *box
	}
	t := wd.now()
	w.LastHeartbeat = &t
	w.LastSeq = seq
	w.DispatchAgeMs = dispatchAgeMs
	w.TickAgeMs = tickAgeMs
	w.UptimeMs = uptimeMs
	w.Players = players
	w.ProbeAlive = nil // it is obviously alive; drop any stale probe answer
}

// OnProbeResult handles the agent's "probe_result" answer.
func (wd *ServerWatchdog) OnProbeResult(box, pid string, alive bool) {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	if dep, ok := wd.pidToDeployment[box+"/"+pid]; ok {
		if known, ok := wd.servers[dep]; ok {
			a := alive
			known.ProbeAlive = &a
			t := wd.now()
			known.ProbeAnsweredAt = &t
			return
		}
	}
	// An answer we did not have a mapping for: adoption of a server we never launched.
	p, ok := parseInt32(pid)
	if !alive || !ok {
		return
	}
	for _, id := range wd.order {
		w := wd.servers[id]
		if w.Pid == p && isNullOrWhiteSpace(w.Box) {
			w.BoxClaims[box] = struct{}{}
		}
	}
}

// Retire is a deliberate stop: stop watching, so a planned shutdown is never "restored".
func (wd *ServerWatchdog) Retire(deploymentId, reason string) bool {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	w, ok := wd.servers[deploymentId]
	if !ok {
		return false
	}
	w.Retired = true
	w.RetiredReason = reason
	w.LastVerdict = "retired"
	wdLog(fmt.Sprintf("%s retired: %s", Short(deploymentId), reason))
	return true
}

// Resume puts a deployment back under watch (also clears the give-up state).
func (wd *ServerWatchdog) Resume(deploymentId string) bool {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	w, ok := wd.servers[deploymentId]
	if !ok {
		return false
	}
	w.Retired = false
	w.RetiredReason = ""
	w.RestartsInWindow = 0
	w.WindowStart = wd.now()
	w.LastVerdict = "watching"
	wdLog(fmt.Sprintf("%s back under watch", Short(deploymentId)))
	return true
}

// Forget mirrors `_servers.TryRemove(deploymentId, out _)`.
func (wd *ServerWatchdog) Forget(deploymentId string) bool {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	if _, ok := wd.servers[deploymentId]; !ok {
		return false
	}
	delete(wd.servers, deploymentId)
	for i, x := range wd.order {
		if x == deploymentId {
			wd.order = append(wd.order[:i:i], wd.order[i+1:]...)
			break
		}
	}
	return true
}

// Stop shuts a deleted deployment's server down for good: retire first (so the
// kill reads as planned), then kill on the owning box -- or the single claimed
// box, or the single connected box; never guessed across several.
func (wd *ServerWatchdog) Stop(deploymentId, reason string) string {
	g := &lockGuard{mu: &wd.mu}
	g.lock()
	defer g.release()
	w, ok := wd.servers[deploymentId]
	if !ok {
		return "no running server known for it"
	}
	w.Retired = true
	w.RetiredReason = reason
	w.LastVerdict = "stopped"
	// Solara (SEC-SID): a deliberately stopped server loses its identity token.
	revokeDeploymentTokens(deploymentId, "stopped: "+reason)
	if w.Pid <= 0 {
		return "retired; its process id is not known yet"
	}
	pid := w.Pid
	var box *string
	if !isNullOrWhiteSpace(w.Box) {
		box = strp(w.Box)
	}
	if box == nil && len(w.BoxClaims) == 1 {
		for k := range w.BoxClaims {
			box = strp(k)
		}
	}
	g.unlock()
	if box == nil {
		agents := wd.hub.Agents()
		if len(agents) == 1 {
			box = strp(agents[0].Box())
		}
	}
	if box == nil {
		wdLog(fmt.Sprintf("%s stopped (%s) but pid %d has no known box -- NOT killed", Short(deploymentId), reason, pid))
		return fmt.Sprintf("retired; pid %d has no known box, so it was not killed", pid)
	}
	sent := wd.hub.Kill(*box, strconv.Itoa(pid), reason)
	how := "FAILED to send"
	if sent {
		how = "sent"
	}
	wdLog(fmt.Sprintf("%s stopped (%s): kill pid %d on '%s' %s", Short(deploymentId), reason, pid, *box, how))
	if sent {
		return fmt.Sprintf("server pid %d on '%s' is being shut down", pid, *box)
	}
	return fmt.Sprintf("could not reach '%s' to stop pid %d", *box, pid)
}

// ─── the decision ───────────────────────────────────────────────────────────

func (wd *ServerWatchdog) evaluateLoop() {
	defer close(wd.done)
	for {
		func() {
			defer func() {
				if p := recover(); p != nil {
					wdLog("evaluate failed: " + hosting.ExceptionMessage(p))
				}
			}()
			wd.Evaluate()
		}()
		select {
		case <-wd.stop:
			return
		case <-time.After(time.Duration(wd.k.evaluateEveryMs) * time.Millisecond):
		}
	}
}

// close stops the evaluate loop (Solara addition; the original loop ran forever).
func (wd *ServerWatchdog) close() {
	select {
	case <-wd.stop:
	default:
		close(wd.stop)
	}
	<-wd.done
}

// lockGuard tracks whether the watchdog mutex is held so a panic unwinding
// through a temporarily-unlocked section never double-unlocks.
type lockGuard struct {
	mu   *sync.Mutex
	held bool
}

func (g *lockGuard) lock()   { g.mu.Lock(); g.held = true }
func (g *lockGuard) unlock() { g.held = false; g.mu.Unlock() }
func (g *lockGuard) release() {
	if g.held {
		g.unlock()
	}
}

// mul32 mirrors unchecked C# int multiplication.
func mul32(a, b int) int { return int(int32(a) * int32(b)) }

// Evaluate runs one decision pass over every watched deployment. A panic
// aborts the pass (the loop logs "evaluate failed: ...").
func (wd *ServerWatchdog) Evaluate() {
	now := wd.now()
	wd.mu.Lock()
	list := make([]*Watched, 0, len(wd.order))
	for _, id := range wd.order {
		list = append(list, wd.servers[id])
	}
	wd.mu.Unlock()
	for _, w := range list {
		func() {
			g := &lockGuard{mu: &wd.mu}
			g.lock()
			defer g.release()
			wd.evaluateOne(g, w, now)
		}()
	}
}

func (wd *ServerWatchdog) evaluateOne(g *lockGuard, w *Watched, now dotnet.DateTime) {
	k := wd.k
	if w.Retired {
		return
	}

	// Just acted: a replacement needs time to launch, inject and register.
	if w.LastRestartAt != nil && msBetween(now.Ticks(), w.LastRestartAt.Ticks()) < float64(k.cooldownMs) {
		w.LastVerdict = "cooling down"
		return
	}

	// A server that heartbeats but has no box is one we did not launch. Find its
	// owner: ask every connected box about its pid. Adopt only on a SINGLE claim.
	if isNullOrWhiteSpace(w.Box) && w.Pid > 0 {
		claims := make([]string, 0, len(w.BoxClaims))
		for b := range w.BoxClaims {
			claims = append(claims, b)
		}
		if len(claims) == 1 {
			w.Box = claims[0]
			wd.pidToDeployment[fmt.Sprintf("%s/%d", w.Box, w.Pid)] = w.DeploymentId
			wdLog(fmt.Sprintf("%s adopted: pid %d belongs to box '%s'", Short(w.DeploymentId), w.Pid, w.Box))
		} else {
			if len(claims) > 1 {
				w.LastVerdict = fmt.Sprintf("holding: pid %d was claimed by %d boxes -- will not guess", w.Pid, len(claims))
				return
			}
			if w.AdoptAskedAt == nil || msBetween(now.Ticks(), w.AdoptAskedAt.Ticks()) > float64(mul32(k.evaluateEveryMs, 3)) {
				t := now
				w.AdoptAskedAt = &t
				pid := w.Pid
				g.unlock()
				asked := wd.hub.ProbeAll(strconv.Itoa(pid))
				g.lock()
				w.LastVerdict = fmt.Sprintf("asking %d box(es) which one runs pid %d", asked, w.Pid)
			}
			return
		}
	}

	// We can only act on a box we are still talking to.
	var agent *AgentConnection
	if !isNullOrWhiteSpace(w.Box) {
		agent = wd.hub.ByBox(w.Box)
	}
	if agent == nil {
		w.LastVerdict = "holding: agent for this box is not connected"
		return
	}

	if w.LastHeartbeat == nil {
		// Never heard from it: still booting, or it came up without the DLL.
		if msBetween(now.Ticks(), w.CreatedAt.Ticks()) < float64(k.startupGraceMs) {
			w.LastVerdict = "starting (no heartbeat yet)"
		} else {
			w.LastVerdict = "holding: never heartbeated, so we never knew it was up"
		}
		return
	}

	sinceHb := msBetween(now.Ticks(), w.LastHeartbeat.Ticks())

	// ── verdict: engine-frozen ──
	if sinceHb < float64(k.heartbeatGraceMs) {
		if w.DispatchAgeMs >= uint64(k.freezeDispatchMs) {
			wd.tryRestart(g, w, "engine-frozen",
				"still heartbeating (so the process is alive) but no UFunction dispatch for "+
					fmt.Sprintf("%ds, and our own tick has not run for %ds", w.DispatchAgeMs/1000, w.TickAgeMs/1000))
		} else {
			w.LastVerdict = fmt.Sprintf("healthy (%d player(s), up %dm)", w.Players, w.UptimeMs/60000)
		}
		return
	}

	// ── verdict: silent ──
	if sinceHb < float64(k.silentMs) {
		w.LastVerdict = fmt.Sprintf("quiet for %ss -- waiting", formatF0(sinceHb/1000))
		return
	}

	// The agent has only just (re)connected: give it the same window again.
	agentUp := msBetween(now.Ticks(), agent.ConnectedAt().Ticks())
	if agentUp < float64(k.silentMs) {
		w.LastVerdict = fmt.Sprintf("silent, but its box only came back %ss ago -- waiting for a fresh heartbeat", formatF0(agentUp/1000))
		return
	}

	if w.Pid <= 0 {
		w.LastVerdict = "holding: silent, but we do not know its pid to check"
		return
	}

	if w.ProbeAlive == nil || w.ProbeAnsweredAt == nil || w.ProbeAnsweredAt.Before(*w.LastHeartbeat) {
		if w.ProbeSentAt == nil || msBetween(now.Ticks(), w.ProbeSentAt.Ticks()) > float64(mul32(k.evaluateEveryMs, 2)) {
			t := now
			w.ProbeSentAt = &t
			box, pid := w.Box, w.Pid
			g.unlock()
			wd.hub.Probe(box, strconv.Itoa(pid))
			g.lock()
			w.LastVerdict = fmt.Sprintf("silent for %ss -- asking the agent whether pid %d is alive", formatF0(sinceHb/1000), w.Pid)
		}
		return // decide next pass, on the answer
	}

	if !*w.ProbeAlive {
		wd.tryRestart(g, w, "process-exited",
			fmt.Sprintf("silent for %ss and the agent confirms pid %d is not running", formatF0(sinceHb/1000), w.Pid))
	} else {
		wd.tryRestart(g, w, "silent",
			fmt.Sprintf("pid %d is alive and its box is online, but it has sent nothing for ", w.Pid)+
				fmt.Sprintf("%ss after heartbeating every 5s", formatF0(sinceHb/1000)))
	}
}

// ─── the action ─────────────────────────────────────────────────────────────

// tryRestart is entered and left with g held; it releases the lock around the
// TakeDeploymentOffline hook, the kill and the spin-up callback.
func (wd *ServerWatchdog) tryRestart(g *lockGuard, w *Watched, verdict, why string) {
	k := wd.k
	now := wd.now()
	if w.Retired {
		return
	}
	if w.LastRestartAt != nil && msBetween(now.Ticks(), w.LastRestartAt.Ticks()) < float64(k.cooldownMs) {
		return
	}

	if msBetween(now.Ticks(), w.WindowStart.Ticks()) > float64(k.restartWindowMs) {
		w.WindowStart = now
		w.RestartsInWindow = 0
	}
	if w.RestartsInWindow >= k.maxRestarts {
		w.Retired = true
		w.RetiredReason = fmt.Sprintf("%d restarts inside %dm -- it is not coming back on its own", k.maxRestarts, k.restartWindowMs/60000)
		w.LastVerdict = "GIVING UP"
		wdLog(fmt.Sprintf("*** %s '%s' GIVING UP: %s. Last verdict was %s: %s", Short(w.DeploymentId), w.Name, w.RetiredReason, verdict, why))
		// Solara (SEC-SID): the deployment is abandoned -- its server token goes with it.
		revokeDeploymentTokens(w.DeploymentId, "watchdog gave up")
		return
	}

	t := now
	w.LastRestartAt = &t
	w.RestartsInWindow++
	w.LastVerdict = "restarting (" + verdict + ")"
	wdLog(fmt.Sprintf("*** %s '%s' is DOWN [%s]: %s", Short(w.DeploymentId), w.Name, verdict, why))

	// Stop players being sent to a corpse.
	dep := w.DeploymentId
	g.unlock()
	func() {
		defer func() {
			if p := recover(); p != nil {
				wdLog(fmt.Sprintf("  could not take %s offline: %s", Short(dep), hosting.ExceptionMessage(p)))
			}
		}()
		if f := TakeDeploymentOffline; f != nil {
			f(dep, verdict)
		}
	}()
	g.lock()

	// Kill first, ALWAYS: a frozen server still owns the game port.
	if w.Pid > 0 && !isNullOrWhiteSpace(w.Box) {
		box, pid := w.Box, w.Pid
		g.unlock()
		ok := wd.hub.Kill(box, strconv.Itoa(pid), verdict)
		if ok {
			wdLog(fmt.Sprintf("  killed pid %d on '%s'", pid, box))
		} else {
			wdLog(fmt.Sprintf("  could not reach '%s' to kill pid %d", box, pid))
		}
		g.lock()
		delete(wd.pidToDeployment, fmt.Sprintf("%s/%d", box, pid))
	}

	// Put it back up AS THE SAME DEPLOYMENT (register_server upserts on the id).
	args := EnsureDeploymentArg(w.Args, w.DeploymentId)
	box, mp, region := strp(w.Box), w.Map, w.Region
	var name *string
	if !isNullOrWhiteSpace(w.Name) {
		name = strp(w.Name)
	}
	g.unlock()
	result := wd.spinUp(box, mp, region, strp(args), name)
	g.lock()
	if result.Success {
		w.CreatedAt = now // restart the start-up grace
		w.Pid = 0
		w.LastHeartbeat = nil
		w.ProbeAlive = nil
		w.ProbeSentAt = nil
		wdLog(fmt.Sprintf("  replacement requested on '%s' (request %s), restart %d/%d", result.Agent, result.Request_id, w.RestartsInWindow, k.maxRestarts))
	} else {
		wdLog("  replacement REFUSED: " + result.Error)
	}
}

// EnsureDeploymentArg appends -DashboardDeploymentId=<id> unless the args
// already carry one (ordinal, ASCII case-insensitive).
func EnsureDeploymentArg(args string, deploymentId string) string {
	if containsASCIIFold(args, "-DashboardDeploymentId=") {
		return args
	}
	return trim(args + " -DashboardDeploymentId=" + deploymentId)
}

// revokeDeploymentTokens revokes a deployment's per-server tokens (Solara,
// SEC-SID). A store without a database (tests) is a no-op; failures are logged
// and never block the watchdog.
func revokeDeploymentTokens(deploymentId, reason string) {
	if _, err := serverid.RevokeDeployment(deploymentId, reason); err != nil {
		securityLog(fmt.Sprintf("[Watchdog] could not revoke the server tokens of %s: %v", Short(deploymentId), err))
	}
}
