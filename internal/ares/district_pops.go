// Solara addition: per-district player counts for the 50451 station browser.
//
// The station browser (WBP_StationBrowser.UpdateStationInfoText) lists every entry of a station's
// district_populations map as "<district>: <n> drifters", and its district sort buttons
// (AA2StationBrowser comparator 0x5670F80) rank stations by the keys "driftball", "parkour", "club", "scrap",
// "spine" and "space". The live service filled that map from the game server's "state" server event
// (FServerEventState.DistrictPopulations); 50451 game servers no longer send it, and the original backend never
// filled the field either (it is always null). Our game-server DLL now counts its players per district
// (AAxPlayerState.District, which each client sets through Server_SetDistrict as it moves between presence zones)
// and adds the map to its /update_player_count report. It is kept here in memory per deployment; an entry that has
// not been refreshed for districtPopsTTL is dropped (the server stopped reporting, e.g. it exited).
package ares

import (
	"strings"
	"sync"
	"time"

	"solara/internal/stj"
)

const districtPopsTTL = 2 * time.Minute

type districtPopsEntry struct {
	pops map[string]int
	ip   string
	at   time.Time
}

var (
	districtPopsMu sync.Mutex
	districtPops   = map[string]*districtPopsEntry{} // deployment id (lower-case) -> latest report
)

// recordDistrictPops stores a deployment's district counts from an /update_player_count body. A body without a
// district_populations object leaves the stored entry alone (older DLLs do not send it).
func recordDistrictPops(deploymentId string, body []byte) {
	doc, err := stj.Parse(body)
	if err != nil || doc.ValueKind() != stj.Object {
		return
	}
	v, ok := doc.TryGetProperty("district_populations")
	if !ok || v.ValueKind() != stj.Object {
		return
	}
	pops := map[string]int{}
	for _, p := range v.EnumerateObject() {
		name := strings.ToLower(strings.TrimSpace(p.Name))
		if name == "" || len(name) > 32 || p.Value.ValueKind() != stj.Number {
			continue
		}
		if n, ok := p.Value.TryGetInt32(); ok && n > 0 && n <= 1000 {
			pops[name] = int(n)
		}
		if len(pops) >= 32 {
			break
		}
	}
	ip := ""
	if d := a2FindDeployment(deploymentId); d != nil && d.IpAddress != nil {
		ip = *d.IpAddress
	}
	districtPopsMu.Lock()
	districtPops[strings.ToLower(deploymentId)] = &districtPopsEntry{pops: pops, ip: ip, at: time.Now()}
	districtPopsMu.Unlock()
}

// districtPopsFor returns a copy of the latest district counts for a station-list item, matched by its deployment
// id and, for an EOS-session row of the same server, by its ip:port. Never nil (the 50451 import expects an object).
func districtPopsFor(id, ip *string) map[string]int {
	out := map[string]int{}
	now := time.Now()
	districtPopsMu.Lock()
	defer districtPopsMu.Unlock()
	var e *districtPopsEntry
	if id != nil {
		e = districtPops[strings.ToLower(*id)]
	}
	if (e == nil || now.Sub(e.at) > districtPopsTTL) && ip != nil && *ip != "" {
		e = nil
		for _, c := range districtPops {
			if c.ip == *ip && now.Sub(c.at) <= districtPopsTTL {
				e = c
				break
			}
		}
	}
	if e == nil || now.Sub(e.at) > districtPopsTTL {
		return out
	}
	for k, n := range e.pops {
		out[k] = n
	}
	return out
}
