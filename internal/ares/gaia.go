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
// The reported event lists are also kept in the database (one row per deployment, older deployments of the same
// station pruned), so a backend restart no longer forgets them: the game server only reports at its own boot and
// when its loaded-event count changes. Queued plays stay in memory (transient). When the backend has no list for a
// polling deployment that asks with ?report=1, gaia/next answers "!report" and the DLL re-reports its list.

import (
	"sort"
	"strings"
	"sync"
	"time"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

const gaiaMaxQueue = 20

var (
	gaiaMu     sync.Mutex
	gaiaEvents = map[string][]string{} // deployment id -> event names (sorted)
	gaiaQueue  = map[string][]string{} // deployment id -> pending plays (FIFO)
	gaiaLoaded bool                    // gaiaEvents seeded from the database
)

// GaiaEventsDbObject is one deployment's reported event list.
type GaiaEventsDbObject struct {
	docdb.AstraDbObject
	DeploymentId string   `json:"deployment_id"`
	StationId    string   `json:"station_id"`
	Events       []string `json:"events"`
	ReportedAt   int64    `json:"reported_at"`
}

func NewGaiaEventsDbObject() *GaiaEventsDbObject { return &GaiaEventsDbObject{} }

func gaiaStore() *docdb.Collection[GaiaEventsDbObject] {
	return docdb.GetCollection(Database, true, NewGaiaEventsDbObject)
}

// gaiaLoadLocked seeds the in-memory lists from the database once (caller holds gaiaMu).
func gaiaLoadLocked() {
	if gaiaLoaded {
		return
	}
	gaiaLoaded = true
	if col := gaiaStore(); col != nil {
		for _, r := range col.FindAll() {
			if r.DeploymentId != "" {
				if _, has := gaiaEvents[r.DeploymentId]; !has {
					gaiaEvents[r.DeploymentId] = r.Events
				}
			}
		}
	}
}

// gaiaPersist saves a deployment's list and drops the stored lists of the station's other deployments.
func gaiaPersist(dep string, list []string) {
	col := gaiaStore()
	if col == nil {
		return
	}
	station := ""
	if d := a2FindDeployment(dep); d != nil && d.StationId != nil {
		station = *d.StationId
	}
	if row := col.FindOne(func(r *GaiaEventsDbObject) bool { return r.DeploymentId == dep }); row != nil {
		row.Events, row.StationId, row.ReportedAt = list, station, time.Now().UnixMilli()
		col.Update(row)
	} else {
		col.Insert(&GaiaEventsDbObject{DeploymentId: dep, StationId: station, Events: list, ReportedAt: time.Now().UnixMilli()})
	}
	if station == "" {
		return
	}
	for _, r := range col.Find(func(r *GaiaEventsDbObject) bool { return r.StationId == station && r.DeploymentId != dep }) {
		col.Delete(r.Id)
		gaiaMu.Lock()
		delete(gaiaEvents, r.DeploymentId)
		gaiaMu.Unlock()
	}
}

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
	gaiaLoadLocked()
	gaiaEvents[dep] = list
	gaiaMu.Unlock()
	gaiaPersist(dep, list)
	return hosting.Ok(stj.O("stored", len(list)))
}

// GetGaiaNext pops the next queued event for the game server ("" = nothing to play).
func (s *A2StationDbServer) GetGaiaNext(c *hosting.Context) hosting.Result {
	dep := c.Param("deployment_id")
	gaiaMu.Lock()
	gaiaLoadLocked()
	next := ""
	if q := gaiaQueue[dep]; len(q) > 0 {
		next, gaiaQueue[dep] = q[0], q[1:]
	} else if _, has := gaiaEvents[dep]; !has && dep != "" && gaiaWantsReportSignal(c) {
		// We hold no event list for this server: ask it to report again. Only for a DLL that polls with ?report=1;
		// an older DLL would treat "!report" as an event name and re-poll without waiting.
		next = "!report"
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
	gaiaLoadLocked()
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

func gaiaWantsReportSignal(c *hosting.Context) bool {
	v, ok := c.Request.Query("report")
	return ok && v == "1"
}
