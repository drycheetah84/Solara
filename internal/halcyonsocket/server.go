// Ports AUnrealFeatures.HalcyonSocket/HalcyonSocketServer.cs (the server
// class; its DTOs are in models.go).
//
// The allocator socket: an HTTP control surface on 9095 (for testing / direct
// use) that OWNS the raw agent TCP hub on 9100. The dashboard's "Spin Up"
// reaches this in-process via Instance.RequestSpinUp(...). The launched game
// server then self-registers via /register_server.
package halcyonsocket

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/serverid"
	"solara/internal/stj"
)

const (
	HOSTNAME   = "localhost"
	PORT       = 9095 // HTTP control
	AGENT_PORT = 9100 // raw TCP for HalcyonAllocatorAgent connections
)

// Instance mirrors HalcyonSocketServer.Instance (set by the constructor).
var Instance *HalcyonSocketServer

// PendingNames holds dashboard-supplied deployment names, keyed by the
// deployment id minted at spin-up. register_server (A2StationDbServer) reads
// (TryRemove) it so a name typed in the "Spin Up" modal becomes the
// deployment's DeploymentName. Survives until the server registers.
var PendingNames = NewConcurrentDictionary[string, string]()

// HalcyonSocketServer mirrors the C# class (an AstraHttpServer).
type HalcyonSocketServer struct {
	HTTP *hosting.Server

	hub *AgentHub

	// Watchdog watches every deployment and replaces one when it is CERTAIN it is down.
	Watchdog *ServerWatchdog

	httpStarted bool
}

// NewHalcyonSocketServer mirrors the constructor: the HTTP control server is
// bound (base constructor), then the agent hub is started, the watchdog
// created and wired, Instance set. Bind failures are returned.
func NewHalcyonSocketServer() (*HalcyonSocketServer, error) {
	return newHalcyonSocketServer(PORT, fmt.Sprintf(":%d", AGENT_PORT), true, dotnet.UtcNow, currentKnobs(), true)
}

// newHalcyonSocketServer is the injectable variant (tests: no fixed ports,
// optional HTTP bind, injectable clock, knobs and watchdog loop).
func newHalcyonSocketServer(httpPort uint16, agentAddr string, bindHTTP bool, now func() dotnet.DateTime, k knobs, loop bool) (*HalcyonSocketServer, error) {
	s := &HalcyonSocketServer{hub: newAgentHubAddr(agentAddr)}
	s.hub.now = now
	h := hosting.NewServer("HalcyonSocketServer", HOSTNAME, httpPort)
	s.HTTP = h
	h.Handle("GET", "/agents", "ListAgents", requireAdmin(s.ListAgents))
	h.Handle("POST", "/spinup", "SpinUp", requireAdmin(s.SpinUp))
	h.Handle("GET", "/watchdog", "WatchdogStatus", requireAdmin(s.WatchdogStatus))
	h.Handle("POST", "/watchdog/retire", "WatchdogRetire", requireAdmin(s.WatchdogRetire))
	h.Handle("POST", "/watchdog/resume", "WatchdogResume", requireAdmin(s.WatchdogResume))
	if bindHTTP {
		if err := h.Start(); err != nil {
			return nil, err
		}
		s.httpStarted = true
	}

	if err := s.hub.Start(); err != nil {
		// The original constructor threw here, aborting host start-up.
		if s.httpStarted {
			h.Stop()
			s.httpStarted = false
		}
		return nil, err
	}
	// The watchdog re-launches through the internal path, which may re-use the
	// deployment id it recorded (callers may not supply one).
	s.Watchdog = newServerWatchdog(s.hub, func(b, m, r, a, n *string) *SpinUpResult { return s.requestSpinUp("", b, m, r, a, n, true) }, now, k, loop)
	s.hub.SetOnSpunUp(s.Watchdog.OnSpunUp)
	s.hub.SetOnExited(s.Watchdog.OnExited)
	s.hub.SetOnProbeResult(s.Watchdog.OnProbeResult)
	Instance = s
	consoleWriteLine(fmt.Sprintf("[HalcyonSocket] control HTTP on %d, agent TCP on %d, watchdog armed", PORT, AGENT_PORT))
	return s, nil
}

// Stop shuts down the HTTP server, the agent hub (listener, connections,
// reaper) and the watchdog loop. Solara addition for clean shutdown: the
// original only ever stopped the HTTP listener.
func (s *HalcyonSocketServer) Stop() {
	if s.httpStarted {
		s.HTTP.Stop()
		s.httpStarted = false
	}
	s.hub.close()
	s.Watchdog.close()
}

// Hub exposes the agent hub (the original field was private; used by tests
// and diagnostics).
func (s *HalcyonSocketServer) Hub() *AgentHub { return s.hub }

// deploymentIdRe mirrors Regex(@"-DashboardDeploymentId=(\S+)", IgnoreCase)
// under invariant globalization: ASCII-only case folding, and .NET's \s
// (Unicode white space [\f\n\r\t\v\x85\p{Z}]).
var deploymentIdRe = regexp.MustCompile(`-[dD][aA][sS][hH][bB][oO][aA][rR][dD][dD][eE][pP][lL][oO][yY][mM][eE][nN][tT][iI][dD]=([^\f\n\r\t\v\x{85}\p{Z}]+)`)

// RequestSpinUp relays a spin-up request to a connected agent. box=nil ->
// least-loaded box. Fire-and-forget: the agent launches + injects, then the
// game server self-registers. name is optional (C# default null).
//
// Solara (F5 / SEC-SID): the inputs are validated first (validateSpinUp), a
// caller may no longer supply -DashboardDeploymentId / -ServerApiKey /
// -DashboardApiKey, and every launch gets a fresh per-server token bound to
// the deployment id minted here and the chosen box.
func (s *HalcyonSocketServer) RequestSpinUp(box, mp, region, args, name *string) *SpinUpResult {
	return s.requestSpinUp("", box, mp, region, args, name, false)
}

// RequestSpinUpForStation is RequestSpinUp for a known station (the
// dashboard's station-scoped Spin Up): the server token is also bound to it.
func (s *HalcyonSocketServer) RequestSpinUpForStation(stationId string, box, mp, region, args, name *string) *SpinUpResult {
	return s.requestSpinUp(stationId, box, mp, region, args, name, false)
}

func spinUpError(msg string) *SpinUpResult {
	r := NewSpinUpResult()
	r.Success = false
	r.Error = msg
	return r
}

// requestSpinUp is the shared implementation; internal=true is the
// watchdog's restart path (it may re-use its recorded deployment id, and its
// box name comes from an agent's hello rather than from a caller).
func (s *HalcyonSocketServer) requestSpinUp(stationId string, box, mp, region, args, name *string, internal bool) *SpinUpResult {
	vbox := box
	if internal {
		vbox = nil
	}
	if why := validateSpinUp(vbox, mp, region, args, name, internal); why != "" {
		securityLog(fmt.Sprintf("[HalcyonSocket] spin-up REJECTED: %s", why))
		return spinUpError("invalid spin-up request: " + why)
	}

	agent := s.hub.Pick(box)
	if agent == nil {
		if isNullOrEmptyP(box) {
			return spinUpError("no agent with free capacity")
		}
		return spinUpError(fmt.Sprintf("agent '%s' unavailable or full", *box))
	}

	reqId := dotnet.NewGuidN()

	// Decide a UNIQUE deployment id here, BEFORE the game launches, and pass it
	// as -DashboardDeploymentId. Only the watchdog may re-use an id (validated
	// above: external callers cannot supply one).
	finalArgs := strings.Join(strings.Fields(deref(args)), " ")
	var deploymentId string
	if m := deploymentIdRe.FindStringSubmatch(finalArgs); internal && m != nil {
		deploymentId = m[1]
	} else {
		deploymentId = dotnet.NewGuidN()
		finalArgs = trim(finalArgs + " -DashboardDeploymentId=" + deploymentId)
	}

	agentBox := agent.Box()
	token, rec, err := serverid.IssueSpec(serverid.Spec{Kind: serverid.KindServer, DeploymentId: deploymentId, StationId: stationId, Box: agentBox})
	if err != nil {
		securityLog(fmt.Sprintf("[HalcyonSocket] spin-up REFUSED: could not issue a server token: %v", err))
		r := spinUpError("could not issue a server token")
		r.Agent = agentBox
		return r
	}
	if !isNullOrWhiteSpaceP(name) {
		PendingNames.Set(deploymentId, trim(*name))
	}

	// The game receives the token both as -ServerApiKey (Mothership,
	// x-server-api-key) and -DashboardApiKey (:78, x-api-key). The watchdog
	// records finalArgs WITHOUT the token; a restart mints a new one.
	wireArgs := finalArgs + " -ServerApiKey=" + token + " -DashboardApiKey=" + token
	payload := stj.O(
		"type", "spinup",
		"id", reqId,
		"map", deref(mp),
		"region", deref(region),
		"args", wireArgs,
		"server_token", token,
	)
	json := stj.MarshalString(payload)
	s.hub.trackLaunch(reqId, agent)
	if !agent.SendLine(json) {
		s.hub.untrackLaunch(reqId)
		_, _ = serverid.Revoke(rec.IdHex(), "spin-up send failed")
		r := NewSpinUpResult()
		r.Success, r.Request_id, r.Agent, r.Error = false, reqId, agentBox, "send to agent failed"
		return r
	}

	// Tell the watchdog how to put this exact server back up later.
	s.Watchdog.OnSpinUpRequested(deploymentId, reqId, agentBox, mp, region, finalArgs, name)

	consoleWriteLine(fmt.Sprintf("[HalcyonSocket] spinup %s -> agent '%s' (map='%s', region='%s')", reqId, agentBox, deref(mp), deref(region)))
	r := NewSpinUpResult()
	r.Success, r.Request_id, r.Agent = true, reqId, agentBox
	return r
}

// GetAgents returns the connected boxes + capacity (dashboard box picker).
func (s *HalcyonSocketServer) GetAgents() []*AgentInfo {
	list := s.hub.Agents()
	out := make([]*AgentInfo, 0, len(list))
	for _, a := range list {
		out = append(out, &AgentInfo{
			Box:       a.Box(),
			Capacity:  a.Capacity(),
			Running:   a.Running(),
			Free:      a.Free(),
			Last_seen: a.LastSeen(),
		})
	}
	return out
}

// ListAgents is GET /agents.
func (s *HalcyonSocketServer) ListAgents(c *hosting.Context) hosting.Result {
	return hosting.Ok(&AgentListResult{Agents: s.GetAgents()})
}

// SpinUp is POST /spinup — direct/test entry.
func (s *HalcyonSocketServer) SpinUp(c *hosting.Context) hosting.Result {
	var body *SpinUpBody
	if !tryDeserialize(c.Request.Body(), &body) {
		body = nil
	}
	var b, m, r, a, n *string
	if body != nil {
		b, m, r, a, n = body.Box, body.Map, body.Region, body.Args, body.Name
	}
	return hosting.Ok(s.RequestSpinUp(b, m, r, a, n))
}

// WatchdogStatus is GET /watchdog.
func (s *HalcyonSocketServer) WatchdogStatus(c *hosting.Context) hosting.Result {
	return hosting.Ok(&WatchdogStatusResult{Servers: s.WatchdogRows()})
}

// WatchdogRows maps the watchdog state to rows, ordered by name (ordinal, stable).
func (s *HalcyonSocketServer) WatchdogRows() []*WatchdogRow {
	ws := s.Watchdog.Servers()
	rows := make([]*WatchdogRow, 0, len(ws))
	for _, w := range ws {
		rows = append(rows, &WatchdogRow{
			Deployment_id:   w.DeploymentId,
			Name:            w.Name,
			Box:             w.Box,
			Pid:             w.Pid,
			Players:         w.Players,
			Uptime_minutes:  int(int32(w.UptimeMs / 60000)), // (int)(ulong) unchecked
			Last_heartbeat:  w.LastHeartbeat,
			Dispatch_age_ms: w.DispatchAgeMs,
			Tick_age_ms:     w.TickAgeMs,
			Verdict:         w.LastVerdict,
			Restarts:        w.RestartsInWindow,
			Last_restart:    w.LastRestartAt,
			Retired:         w.Retired,
			Retired_reason:  w.RetiredReason,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool { return compareOrdinal(rows[i].Name, rows[j].Name) < 0 })
	return rows
}

// WatchdogRetire is POST /watchdog/retire — stop watching one deployment, so a
// DELIBERATE shutdown is not undone.
func (s *HalcyonSocketServer) WatchdogRetire(c *hosting.Context) hosting.Result {
	b := readWatchdogBody(c.Request)
	ok := b != nil
	if ok {
		reason := "stopped on purpose"
		if b.Reason != nil {
			reason = *b.Reason
		}
		ok = s.Watchdog.Retire(deref(b.Deployment_id), reason)
	}
	return hosting.Ok(&SuccessResult{Success: ok})
}

// WatchdogResume is POST /watchdog/resume — put it back under watch.
func (s *HalcyonSocketServer) WatchdogResume(c *hosting.Context) hosting.Result {
	b := readWatchdogBody(c.Request)
	return hosting.Ok(&SuccessResult{Success: b != nil && s.Watchdog.Resume(deref(b.Deployment_id))})
}

func readWatchdogBody(r *hosting.Request) *WatchdogBody {
	var b *WatchdogBody
	if !tryDeserialize(r.Body(), &b) {
		return nil
	}
	return b
}

// tryDeserialize mirrors `try { x = JsonSerializer.Deserialize<T>(body); } catch { ... }`:
// any failure (error or panic) reports false.
func tryDeserialize(body []byte, v any) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return stj.Unmarshal(body, v) == nil
}
