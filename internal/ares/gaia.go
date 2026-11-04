package ares

// Solara: force GAIA (the station announcer) to play a Wwise event for everyone.
//
// The game server DLL reports every Wwise event the game has loaded and polls for play requests; it then calls
// UA2AnnouncementManagerComponent::Broadcast_PlayAnnouncementEvent(event), a reliable NetMulticast, so every
// client plays it through the GAIA speakers (players who turned "Play GAIA" off in settings won't hear it).
//
//   game server -> POST /v1/deployments/{id}/gaia/events   text/plain, one event name per line (replaces the list)
//   game server -> GET  /v1/deployments/{id}/gaia/next     text/plain: the next queued event name, or empty
//   dashboard   -> GET  /api/stations/{id}/gaia            {deployment_id, deployments:[{id,name}], events:[...], queued}
//                                                          fleet-wide: every live server, events = union of their lists
//   dashboard   -> POST /api/stations/{id}/gaia/play       {event} -> queued on every live server that has it (fleet.go)
//
// The reported event lists are also kept in the database (one row per deployment, older deployments of the same
// station pruned once they stop heartbeating), so a backend restart no longer forgets them: the game server only reports at its own boot and
// when its loaded-event count changes. Queued plays stay in memory (transient). When the backend has no list for a
// polling deployment that asks with ?report=1, gaia/next answers "!report" and the DLL re-reports its list.

import (
	"sort"
	"strings"
	"sync"
	"time"

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
	// Only prune the station's dead deployments: a station can run several live servers at once, and dropping a live
	// one's list made it re-report ("!report"), which then dropped this one's -- the two kept knocking each other out.
	for _, r := range col.Find(func(r *GaiaEventsDbObject) bool {
		return r.StationId == station && r.DeploymentId != dep && !fleetIsLive(r.DeploymentId)
	}) {
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

// gaiaFleetLocked lists the live deployments that have reported an event list (caller holds gaiaMu).
func gaiaFleetLocked() []fleetDeployment {
	gaiaLoadLocked()
	out := []fleetDeployment{}
	for _, d := range fleetLiveDeployments() {
		if _, has := gaiaEvents[d.Id]; has {
			out = append(out, d)
		}
	}
	return out
}

// GetGaia lists the GAIA-playable events across every live game server in the fleet (the union of their lists).
// deployment_id is the most recently heartbeating one (kept for older dashboards); deployments lists them all.
func (s *AresDashboardServer) GetGaia(c *hosting.Context) hosting.Result {
	gaiaMu.Lock()
	defer gaiaMu.Unlock()
	fleet := gaiaFleetLocked()
	seen := map[string]bool{}
	ev := []string{}
	queued := 0
	deps := []stj.Obj{}
	for _, d := range fleet {
		for _, n := range gaiaEvents[d.Id] {
			if !seen[n] {
				seen[n] = true
				ev = append(ev, n)
			}
		}
		queued += len(gaiaQueue[d.Id])
		deps = append(deps, stj.O("id", d.Id, "name", d.Name))
	}
	sort.Strings(ev)
	dep := ""
	if len(fleet) > 0 {
		dep = fleet[0].Id
	}
	return hosting.Ok(stj.O("deployment_id", dep, "deployments", deps, "events", ev, "queued", queued))
}

// PlayGaia queues an event on every live game server in the fleet that has it loaded.
func (s *AresDashboardServer) PlayGaia(c *hosting.Context) hosting.Result {
	body, err := stj.Parse(c.Request.Body())
	ev := ""
	if err == nil {
		if v, ok := body.TryGetProperty("event"); ok && v.ValueKind() == stj.String {
			ev = strings.TrimSpace(*v.GetString())
		}
	}
	gaiaMu.Lock()
	defer gaiaMu.Unlock()
	fleet := gaiaFleetLocked()
	if len(fleet) == 0 {
		return hosting.Ok(dashActionError("no live game server has reported its GAIA events yet"))
	}
	var targets []string
	full := 0
	for _, d := range fleet {
		known := false
		for _, n := range gaiaEvents[d.Id] {
			if n == ev {
				known = true
				break
			}
		}
		if !known {
			continue
		}
		if len(gaiaQueue[d.Id]) >= gaiaMaxQueue {
			full++
			continue
		}
		targets = append(targets, d.Id)
	}
	if len(targets) == 0 {
		if full > 0 {
			return hosting.Ok(dashActionError("too many plays queued; wait a few seconds"))
		}
		return hosting.Ok(dashActionError("unknown event (not reported by the game server)"))
	}
	for _, dep := range targets {
		gaiaQueue[dep] = append(gaiaQueue[dep], ev)
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(ev)})
}

func gaiaWantsReportSignal(c *hosting.Context) bool {
	v, ok := c.Request.Query("report")
	return ok && v == "1"
}
