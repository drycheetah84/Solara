package ares

// Solara: fleet-wide dashboard actions.
//
// GAIA plays, custom notifications and netvar (station config) edits used to target one station's "freshest"
// deployment, so a station running several game servers (e.g. "Halcyon" + "Halcyon 2") only reached one of them, and
// other stations' servers never got anything. The dashboard now sends these to every live game server in the fleet.
// A deployment is live while it keeps heartbeating (LastEvent is refreshed every few seconds by the game server).

import (
	"time"

	"solara/internal/ares/models"
	"solara/internal/hosting"
)

// fleetLiveWindow: a deployment that has sent nothing for this long is treated as down.
const fleetLiveWindow = 3 * time.Minute

// fleetDeployment is one live game server.
type fleetDeployment struct {
	Id      string
	Name    string
	Station string
}

// fleetLiveDeployments lists every deployment that heartbeated within fleetLiveWindow, newest heartbeat first.
func fleetLiveDeployments() []fleetDeployment {
	col := a2Deployments()
	if col == nil {
		return nil
	}
	// compare instants: dates read back from the DB are Local-kind, so raw ticks are off by the UTC offset
	cutoff := time.Now().Add(-fleetLiveWindow)
	var live []*models.DeploymentDbObject
	for _, d := range col.FindAll() {
		if d == nil || d.DeploymentId == nil || *d.DeploymentId == "" || d.LastEvent == nil || d.LastEvent.Time().Before(cutoff) {
			continue
		}
		live = append(live, d)
	}
	// newest heartbeat first (stable: insertion order breaks ties)
	for i := 1; i < len(live); i++ {
		for j := i; j > 0 && live[j].LastEvent.Time().After(live[j-1].LastEvent.Time()); j-- {
			live[j], live[j-1] = live[j-1], live[j]
		}
	}
	out := make([]fleetDeployment, 0, len(live))
	for _, d := range live {
		out = append(out, fleetDeployment{Id: *d.DeploymentId, Name: a2Deref(d.DeploymentName), Station: a2Deref(d.StationId)})
	}
	return out
}

// fleetIsLive reports whether a deployment id is currently heartbeating.
func fleetIsLive(dep string) bool {
	for _, d := range fleetLiveDeployments() {
		if d.Id == dep {
			return true
		}
	}
	return false
}

// dashFleetScope reports whether a dashboard request asked for a fleet-wide change (?fleet=1).
func dashFleetScope(c *hosting.Context) bool {
	v, ok := c.Request.Query("fleet")
	return ok && (v == "1" || v == "true")
}
