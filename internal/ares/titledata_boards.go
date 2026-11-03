package ares

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"solara/internal/ares/models"
	"solara/internal/mothership"
)

// Solara: feed the live station's board image URLs into Mothership client title data. On 50451 the boards whose
// ImageLoader Source is MothershipTitleData (BoardTextureUrl1/2/11, ShuttlePosterUrl, ShuttleUrl4-6) read their
// URL from GET /v1/title-data/client; the dashboard's board upload stores them in the station config, so without
// this those boards never loaded. The station is the one behind the newest online deployment (title data is
// global, like the browser's default station).
func init() {
	mothership.TitleDataExtra = boardTitleData
}

// boardTitleDataStation picks the station whose boards go into the (global) client title data:
//  1. SOLARA_TITLEDATA_STATION, when it names a station with at least one board value (the main fleet);
//  2. otherwise the newest online deployment's station that has board values;
//  3. otherwise the deployment that reported most recently (the watchdog can leave a live, heartbeating
//     deployment flagged offline after a restart, seen 2026-11-01), again only one with board values.
//
// Before, it was simply the newest online deployment, so a fleet without boards booting last (Freerunners,
// 2026-11-03) blanked every client's boards.
func boardTitleDataStation() *models.StationDbObject {
	hasBoards := func(st *models.StationDbObject) bool {
		if st == nil || st.Config == nil {
			return false
		}
		for _, k := range AllBoardImageKeys {
			if v, ok := st.Config[k]; ok && v != nil && strings.TrimSpace(*v) != "" {
				return true
			}
		}
		return false
	}
	if id := strings.TrimSpace(os.Getenv("SOLARA_TITLEDATA_STATION")); id != "" {
		if st := a2FindStation(id); hasBoards(st) {
			return st
		}
	}
	all := a2Deployments().FindAll()
	online := []*models.DeploymentDbObject{}
	recent := []*models.DeploymentDbObject{}
	for _, d := range all {
		if a2IsNullOrEmpty(d.StationId) {
			continue
		}
		if d.Online {
			online = append(online, d)
		}
		if d.LastEvent != nil {
			recent = append(recent, d)
		}
	}
	sort.SliceStable(online, func(i, j int) bool { return online[i].CreatedAt.After(online[j].CreatedAt) })
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].LastEvent.After(*recent[j].LastEvent) })
	for _, list := range [][]*models.DeploymentDbObject{online, recent} {
		for _, d := range list {
			if st := a2FindStation(a2Deref(d.StationId)); hasBoards(st) {
				return st
			}
		}
	}
	return nil
}

func boardTitleData() (out map[string]string) {
	out = map[string]string{}
	defer func() { _ = recover() }() // a missing DB or collection just means no extras
	st := boardTitleDataStation()
	if st == nil {
		return out
	}
	for _, k := range append([]string{"StationAnnouncement"}, AllBoardImageKeys...) {
		if v, ok := st.Config[k]; ok && v != nil && strings.TrimSpace(*v) != "" {
			out[k] = *v
		}
	}
	return out
}

// AllBoardImageKeys (Solara): every image key a 50451 board or poster reads -- the dashboard's BoardConfigKeys
// plus BoardTextureUrl7-14 (in the config/stationConfig netvar schema) and the shuttle posters (title data only,
// from the maps' ImageLoader components). A station config value for any of them is forwarded.
var AllBoardImageKeys = func() []string {
	k := append([]string{}, BoardConfigKeys...)
	for i := 7; i <= 14; i++ {
		k = append(k, "BoardTextureUrl"+strconv.Itoa(i))
	}
	return append(k, "ShuttlePosterUrl", "ShuttleUrl4", "ShuttleUrl5", "ShuttleUrl6")
}()

// stationConfigNetvarKeys: the keys that exist under config/stationConfig in the 50451 netvar schema
// (NetVars/config.json) -- the shuttle posters are not there, so they go to title data only.
var stationConfigNetvarKeys = func() []string {
	out := []string{"StationAnnouncement"}
	for _, k := range AllBoardImageKeys {
		if !strings.HasPrefix(k, "Shuttle") {
			out = append(out, k)
		}
	}
	return out
}()
