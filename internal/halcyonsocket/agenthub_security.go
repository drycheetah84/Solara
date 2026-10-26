// Agent-protocol hardening for the :9100 hub (Solara addition, KNOWN_ISSUES
// F6 / SEC-SID; docs/SERVER_IDENTITY.md). Backwards compatible with the
// compiled HalcyonAllocatorAgent.exe, which cannot send a token:
//
//   - a line is capped at maxAgentLine bytes (also the unterminated buffer);
//     overflow disconnects;
//   - hello may carry "token": an agent token (Kind "agent") bound to the box
//     name it claims. An invalid / foreign / revoked token is refused;
//   - SOLARA_AGENT_ALLOWED_IPS (IPs / CIDRs, comma separated) authenticates
//     agents that cannot send a token;
//   - SOLARA_REQUIRE_AGENT_AUTH=1: an agent with neither is disconnected (and
//     nothing but hello is processed before an admitted hello). Otherwise it
//     is accepted with a warning (compat);
//   - a hello claiming a box name another live connection holds is refused,
//     unless it carries a valid token for that box (it then replaces the old
//     connection) or comes from the same remote IP (original behaviour: an
//     agent reconnecting before its stale socket was reaped);
//   - spunup/exited reports about launches this hub sent are accepted only
//     from the launching connection, another connection from the same IP, or
//     one holding a token for that box. Reports about launches the hub never
//     sent keep the original behaviour (the watchdog keys them by box/pid);
//   - a heartbeat re-validates the agent token (sliding expiry); a revoked or
//     expired token disconnects the agent.
package halcyonsocket

import (
	"fmt"
	"net"
	"os"
	"strings"

	"solara/internal/logging"
	"solara/internal/serverid"
)

// maxAgentLine caps one protocol line (and the pending, unterminated input).
const maxAgentLine = 64 * 1024

// maxTrackedLaunches bounds the launch-ownership maps.
const maxTrackedLaunches = 4096

// securityLog receives the hub's security messages. It is separate from
// consoleWriteLine so the original protocol log stays byte-identical.
var securityLog = func(s string) { logging.Console(s) }

// launchRec remembers which connection a launch was sent to.
type launchRec struct {
	conn *AgentConnection
	ip   string
	box  string
}

func agentAuthRequired() bool { return serverid.EnvTrue("SOLARA_REQUIRE_AGENT_AUTH") }

func remoteIP(c net.Conn) string {
	if c == nil || c.RemoteAddr() == nil {
		return ""
	}
	a := c.RemoteAddr().String()
	if host, _, err := net.SplitHostPort(a); err == nil {
		return host
	}
	return a
}

// ipAllowed reports whether ip is listed in SOLARA_AGENT_ALLOWED_IPS.
func ipAllowed(ip string) bool {
	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}
	for _, e := range strings.Split(os.Getenv("SOLARA_AGENT_ALLOWED_IPS"), ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.Contains(e, "/") {
			if _, n, err := net.ParseCIDR(e); err == nil && n.Contains(addr) {
				return true
			}
			continue
		}
		if p := net.ParseIP(e); p != nil && p.Equal(addr) {
			return true
		}
	}
	return false
}

// HelloOK reports whether this connection's hello was admitted.
func (a *AgentConnection) HelloOK() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.helloOK
}

// Authenticated reports whether the agent holds a token or an allowlisted IP.
func (a *AgentConnection) Authenticated() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.authBox != "" || a.allowlisted
}

// tokenFor: the connection holds a valid agent token for box.
func (a *AgentConnection) tokenFor(box string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.authBox != "" && strings.EqualFold(a.authBox, box)
}

// admitHello applies the authentication and hijack rules to a hello. false =
// refuse (the reader disconnects the agent).
func (h *AgentHub) admitHello(conn *AgentConnection, box, token string) bool {
	who := fmt.Sprintf("agent '%s' from %s", box, conn.ip)
	tokenOK := false
	if token != "" {
		rec, ok := serverid.Validate(token)
		if !ok || rec.Kind != serverid.KindAgent || !strings.EqualFold(rec.BoxName(), box) {
			securityLog("[HalcyonSocket] REFUSED " + who + ": invalid, revoked or foreign agent token")
			return false
		}
		tokenOK = true
	}
	allow := ipAllowed(conn.ip)
	if !tokenOK && !allow {
		if agentAuthRequired() {
			securityLog("[HalcyonSocket] REFUSED " + who + ": no agent token and IP not in SOLARA_AGENT_ALLOWED_IPS (SOLARA_REQUIRE_AGENT_AUTH=1)")
			return false
		}
		conn.mu.Lock()
		warn := !conn.warned
		conn.warned = true
		conn.mu.Unlock()
		if warn {
			securityLog("[HalcyonSocket] WARNING: unauthenticated " + who + " accepted (compat mode; issue an agent token or set SOLARA_AGENT_ALLOWED_IPS)")
		}
	}

	// box-name hijack
	type other struct {
		id string
		c  *AgentConnection
	}
	var others []other
	h.mu.Lock()
	for _, id := range h.order {
		if c := h.agents[id]; c != conn && c.Box() == box {
			others = append(others, other{id, c})
		}
	}
	h.mu.Unlock()
	for _, o := range others {
		switch {
		case tokenOK:
			securityLog(fmt.Sprintf("[HalcyonSocket] %s holds a valid token for the box: replacing the other connection from %s", who, o.c.ip))
			h.remove(o.id)
			_ = o.c.Client.Close()
		case o.c.ip == conn.ip:
			// the same machine reconnecting before its stale socket was reaped
		default:
			securityLog(fmt.Sprintf("[HalcyonSocket] REFUSED %s: box name already connected from %s (box-name hijack)", who, o.c.ip))
			return false
		}
	}

	conn.mu.Lock()
	conn.helloOK = true
	conn.allowlisted = allow
	if tokenOK {
		conn.token, conn.authBox = token, box
	} else {
		conn.token, conn.authBox = "", ""
	}
	conn.mu.Unlock()
	return true
}

// recheckAgentToken re-validates a token-authenticated agent (slides its idle
// expiry). false = the token was revoked or expired: disconnect.
func (h *AgentHub) recheckAgentToken(conn *AgentConnection) bool {
	conn.mu.Lock()
	tok := conn.token
	conn.mu.Unlock()
	if tok == "" {
		return true
	}
	if rec, ok := serverid.Validate(tok); ok && rec.Kind == serverid.KindAgent {
		return true
	}
	securityLog(fmt.Sprintf("[HalcyonSocket] agent '%s' from %s: agent token revoked or expired -- disconnecting", conn.Box(), conn.ip))
	return false
}

// trackLaunch records that a spin-up request was sent to conn.
func (h *AgentHub) trackLaunch(reqId string, conn *AgentConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.launches) >= maxTrackedLaunches {
		for k := range h.launches { // drop an arbitrary old entry
			delete(h.launches, k)
			break
		}
	}
	h.launches[reqId] = launchRec{conn: conn, ip: conn.ip, box: conn.Box()}
}

// untrackLaunch forgets a launch that was never sent.
func (h *AgentHub) untrackLaunch(reqId string) {
	h.mu.Lock()
	delete(h.launches, reqId)
	h.mu.Unlock()
}

// owns: conn may report about a launch recorded as rec.
func (rec launchRec) ownedBy(conn *AgentConnection) bool {
	return rec.conn == conn || (rec.ip != "" && rec.ip == conn.ip) || conn.tokenFor(rec.box)
}

// acceptSpunUp checks a spunup report; on an accepted "ok" it records the pid
// as owned by the launcher.
func (h *AgentHub) acceptSpunUp(conn *AgentConnection, reqId, pid, status string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	rec, tracked := h.launches[reqId]
	if !tracked {
		return true // not ours: the watchdog ignores unknown request ids
	}
	if !rec.ownedBy(conn) {
		securityLog(fmt.Sprintf("[HalcyonSocket] IGNORED spunup id=%s from '%s' (%s): request was sent to '%s' (%s)", reqId, conn.Box(), conn.ip, rec.box, rec.ip))
		return false
	}
	delete(h.launches, reqId)
	if status == "ok" && pid != "" {
		if len(h.pids) >= maxTrackedLaunches {
			for k := range h.pids {
				delete(h.pids, k)
				break
			}
		}
		h.pids[rec.box+"/"+pid] = rec
	}
	return true
}

// acceptExited checks an exited report against the pid's launcher.
func (h *AgentHub) acceptExited(conn *AgentConnection, pid string) bool {
	key := conn.Box() + "/" + pid
	h.mu.Lock()
	defer h.mu.Unlock()
	rec, tracked := h.pids[key]
	if !tracked {
		return true // a pid we never launched: original behaviour
	}
	if !rec.ownedBy(conn) {
		securityLog(fmt.Sprintf("[HalcyonSocket] IGNORED exited pid=%s from '%s' (%s): launched via %s", pid, conn.Box(), conn.ip, rec.ip))
		return false
	}
	delete(h.pids, key)
	return true
}
