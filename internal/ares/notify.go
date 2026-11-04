package ares

// Solara: send a custom text notification to chosen players (or everyone) on a station's live game server.
//
// The game client already has the display: UA2NotificationComponent::Client_ShowNotification(FString Message), a
// server->client RPC on every player's AVRPlayerController. Its implementation spawns the notification board in front
// of the player with the message as-is (no template, unlike the Mothership Warning/Mute websocket push). The game
// server DLL reports who is connected and polls for queued notifications, then calls that RPC for each target.
//
//   game server -> POST /v1/deployments/{id}/notify/players  text/plain, one "<player id>\t<name>" per line (replaces)
//   game server -> GET  /v1/deployments/{id}/notify/next     text/plain: "<targets>\n<message>" or empty;
//                                                             targets = "*" (everyone) or comma-separated player ids
//   dashboard   -> GET  /api/stations/{id}/notify            {deployment_id, deployments, players:[{id,name,server}], queued}
//   dashboard   -> POST /api/stations/{id}/notify            {to: "all" | ["<player id>", ...], message}
//   Both are fleet-wide (fleet.go): every live game server, whatever station the page is on.
//
// Everything is in memory: the player list is re-reported every few seconds and queued notifications are transient.

import (
	"strings"
	"sync"
	"time"

	"solara/internal/hosting"
	"solara/internal/stj"
)

const (
	notifyMaxQueue   = 20
	notifyMaxMessage = 400
	notifyStaleAfter = 2 * time.Minute // a player list older than this is treated as gone (server down)
)

// NotifyPlayer is one connected player as reported by the game server.
type NotifyPlayer struct {
	Id     string `json:"id"`
	Name   string `json:"name"`
	Server string `json:"server"` // deployment name, set in fleet listings
}

type notifyReport struct {
	players []NotifyPlayer
	at      time.Time
}

var (
	notifyMu      sync.Mutex
	notifyPlayers = map[string]notifyReport{} // deployment id -> last reported player list
	notifyQueue   = map[string][]string{}     // deployment id -> pending "<targets>\n<message>" (FIFO)
)

// PostNotifyPlayers stores the game server's current player list.
func (s *A2StationDbServer) PostNotifyPlayers(c *hosting.Context) hosting.Result {
	dep := c.Param("deployment_id")
	if dep == "" {
		return hosting.BadRequest(stj.O("error", "deployment_id required"))
	}
	seen := map[string]bool{}
	list := []NotifyPlayer{}
	for _, line := range strings.Split(string(c.Request.Body()), "\n") {
		id, name, _ := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 128 || seen[id] {
			continue
		}
		seen[id] = true
		name = strings.TrimSpace(name)
		if len(name) > 64 {
			name = name[:64]
		}
		list = append(list, NotifyPlayer{Id: id, Name: name})
		if len(list) >= 256 {
			break
		}
	}
	notifyMu.Lock()
	notifyPlayers[dep] = notifyReport{players: list, at: time.Now()}
	notifyMu.Unlock()
	return hosting.Ok(stj.O("stored", len(list)))
}

// GetNotifyNext pops the next queued notification for the game server ("" = nothing to show).
func (s *A2StationDbServer) GetNotifyNext(c *hosting.Context) hosting.Result {
	dep := c.Param("deployment_id")
	next := ""
	notifyMu.Lock()
	if q := notifyQueue[dep]; len(q) > 0 {
		next, notifyQueue[dep] = q[0], q[1:]
	}
	notifyMu.Unlock()
	return hosting.Configurable(200, "text/plain; charset=utf-8", []byte(next))
}

// notifyFleetLocked lists the live deployments with a fresh player report (caller holds notifyMu).
func notifyFleetLocked() []fleetDeployment {
	out := []fleetDeployment{}
	for _, d := range fleetLiveDeployments() {
		if r, ok := notifyPlayers[d.Id]; ok && time.Since(r.at) <= notifyStaleAfter {
			out = append(out, d)
		}
	}
	return out
}

// GetNotify lists who is on every live game server in the fleet right now (each player tagged with its server).
// deployment_id is the most recently heartbeating server (kept for older dashboards).
func (s *AresDashboardServer) GetNotify(c *hosting.Context) hosting.Result {
	notifyMu.Lock()
	defer notifyMu.Unlock()
	fleet := notifyFleetLocked()
	players := []NotifyPlayer{}
	deps := []stj.Obj{}
	queued := 0
	for _, d := range fleet {
		for _, p := range notifyPlayers[d.Id].players {
			p.Server = d.Name
			players = append(players, p)
		}
		queued += len(notifyQueue[d.Id])
		deps = append(deps, stj.O("id", d.Id, "name", d.Name, "players", len(notifyPlayers[d.Id].players)))
	}
	dep := ""
	if len(fleet) > 0 {
		dep = fleet[0].Id
	}
	return hosting.Ok(stj.O("deployment_id", dep, "deployments", deps, "players", players, "queued", queued))
}

// SendNotify queues a notification for chosen players (on whichever servers they are on) or for everyone on every
// live game server in the fleet.
func (s *AresDashboardServer) SendNotify(c *hosting.Context) hosting.Result {
	body, err := stj.Parse(c.Request.Body())
	if err != nil {
		return hosting.Ok(dashActionError("invalid JSON body"))
	}
	msg := ""
	if v, ok := body.TryGetProperty("message"); ok && v.ValueKind() == stj.String {
		msg = strings.TrimSpace(strings.ReplaceAll(*v.GetString(), "\r", ""))
	}
	if msg == "" {
		return hosting.Ok(dashActionError("message is empty"))
	}
	if len([]rune(msg)) > notifyMaxMessage {
		return hosting.Ok(dashActionError("message is too long (400 characters max)"))
	}
	all := false
	var ids []string
	if v, ok := body.TryGetProperty("to"); ok {
		switch v.ValueKind() {
		case stj.String:
			all = strings.EqualFold(strings.TrimSpace(*v.GetString()), "all")
		case stj.Array:
			for _, e := range v.EnumerateArray() {
				if e.ValueKind() == stj.String {
					if id := strings.TrimSpace(*e.GetString()); id != "" && !strings.ContainsAny(id, ",\n") {
						ids = append(ids, id)
					}
				}
			}
		}
	}
	if !all && len(ids) == 0 {
		return hosting.Ok(dashActionError("pick at least one player, or send to everyone"))
	}
	notifyMu.Lock()
	defer notifyMu.Unlock()
	fleet := notifyFleetLocked()
	if len(fleet) == 0 {
		return hosting.Ok(dashActionError("no live game server has reported its players yet"))
	}
	plan := map[string]string{} // deployment -> targets
	order := []string{}
	for _, d := range fleet {
		targets := "*"
		if !all {
			online := map[string]bool{}
			for _, p := range notifyPlayers[d.Id].players {
				online[p.Id] = true
			}
			keep := []string{}
			for _, id := range ids {
				if online[id] {
					keep = append(keep, id)
				}
			}
			if len(keep) == 0 {
				continue
			}
			targets = strings.Join(keep, ",")
		}
		plan[d.Id] = targets
		order = append(order, d.Id)
	}
	if len(order) == 0 {
		return hosting.Ok(dashActionError("none of the chosen players are on a server any more"))
	}
	for _, dep := range order {
		if len(notifyQueue[dep]) >= notifyMaxQueue {
			return hosting.Ok(dashActionError("too many notifications queued; wait a few seconds"))
		}
	}
	for _, dep := range order {
		notifyQueue[dep] = append(notifyQueue[dep], plan[dep]+"\n"+msg)
	}
	sent := "*"
	if !all {
		sent = strings.Join(ids, ",")
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(sent)})
}
