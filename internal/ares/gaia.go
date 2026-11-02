package ares

// Solara: force GAIA (the station announcer) to play a Wwise event for everyone.
//
// The game server DLL reports every Wwise event the game has loaded and polls for play requests; it then calls
// UA2AnnouncementManagerComponent::Broadcast_PlayAnnouncementEvent(event), a reliable NetMulticast, so every
// client plays it through the GAIA speakers (players who turned "Play GAIA" off in settings won't hear it).
//
//   game server -> POST /v1/deployments/{id}/gaia/events   text/plain, one event name per line (replaces the list)
//   game server -> GET  /v1/deployments/{id}/gaia/next     text/plain: the next queued event name, or empty
//   dashboard   -> GET  /api/stations/{id}/gaia            {deployment_id, events:[...], queued}
//   dashboard   -> POST /api/stations/{id}/gaia/play       {event}
//
// State is in memory: the server re-reports its events after every start, and queued plays are transient.

import (
	"sort"
	"strings"
	"sync"

	"solara/internal/ares/models"
	"solara/internal/hosting"
	"solara/internal/stj"
)

const gaiaMaxQueue = 20

var (
	gaiaMu     sync.Mutex
	gaiaEvents = map[string][]string{} // deployment id -> event names (sorted)
	gaiaQueue  = map[string][]string{} // deployment id -> pending plays (FIFO)
)

// PostGaiaEvents stores the event list a game server reports.
func (s *A2StationDbServer) PostGaiaEvents(c *hosting.Context) hosting.Result {
	dep := c.Param("deployment_id")
	if dep == "" {
		return hosting.BadRequest(stj.O("error", "deployment_id required"))
	}
	seen := map[string]bool{}
	list := []string{}
	for _, line := range strings.Split(string(c.Request.Body()), "\n") {
		n := strings.TrimSpace(line)
		if n == "" || len(n) > 200 || seen[n] {
			continue
		}
		seen[n] = true
		list = append(list, n)
		if len(list) >= 20000 {
			break
		}
	}
	sort.Strings(list)
	gaiaMu.Lock()
	gaiaEvents[dep] = list
	gaiaMu.Unlock()
	return hosting.Ok(stj.O("stored", len(list)))
}

// GetGaiaNext pops the next queued event for the game server ("" = nothing to play).
func (s *A2StationDbServer) GetGaiaNext(c *hosting.Context) hosting.Result {
	dep := c.Param("deployment_id")
	gaiaMu.Lock()
	next := ""
	if q := gaiaQueue[dep]; len(q) > 0 {
		next, gaiaQueue[dep] = q[0], q[1:]
	}
	gaiaMu.Unlock()
	return hosting.Configurable(200, "text/plain; charset=utf-8", []byte(next))
}

// gaiaDeploymentFor picks the station's deployment that reported events most recently-alive: the newest
// LastEvent among its deployments that have an event list (falls back to the newest deployment).
func gaiaDeploymentFor(stationId string) string {
	var best *models.DeploymentDbObject
	gaiaMu.Lock()
	defer gaiaMu.Unlock()
	for _, d := range a2DeploymentsOfStation(&stationId) {
		if d == nil || d.DeploymentId == nil || d.LastEvent == nil {
			continue
		}
		if _, has := gaiaEvents[*d.DeploymentId]; !has {
			continue
		}
		if best == nil || d.LastEvent.After(*best.LastEvent) {
			best = d
		}
	}
	if best == nil {
		return ""
	}
	return *best.DeploymentId
}

// GetGaia lists the GAIA-playable events of a station's live game server.
func (s *AresDashboardServer) GetGaia(c *hosting.Context) hosting.Result {
	dep := gaiaDeploymentFor(c.Param("station_id"))
	gaiaMu.Lock()
	ev := append([]string{}, gaiaEvents[dep]...)
	queued := len(gaiaQueue[dep])
	gaiaMu.Unlock()
	return hosting.Ok(stj.O("deployment_id", dep, "events", ev, "queued", queued))
}

// PlayGaia queues an event for the station's live game server.
func (s *AresDashboardServer) PlayGaia(c *hosting.Context) hosting.Result {
	dep := gaiaDeploymentFor(c.Param("station_id"))
	if dep == "" {
		return hosting.Ok(dashActionError("no live game server has reported its GAIA events yet"))
	}
	body, err := stj.Parse(c.Request.Body())
	ev := ""
	if err == nil {
		if v, ok := body.TryGetProperty("event"); ok && v.ValueKind() == stj.String {
			ev = strings.TrimSpace(*v.GetString())
		}
	}
	gaiaMu.Lock()
	defer gaiaMu.Unlock()
	known := false
	for _, n := range gaiaEvents[dep] {
		if n == ev {
			known = true
			break
		}
	}
	if !known {
		return hosting.Ok(dashActionError("unknown event (not reported by the game server)"))
	}
	if len(gaiaQueue[dep]) >= gaiaMaxQueue {
		return hosting.Ok(dashActionError("too many plays queued; wait a few seconds"))
	}
	gaiaQueue[dep] = append(gaiaQueue[dep], ev)
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(ev)})
}
