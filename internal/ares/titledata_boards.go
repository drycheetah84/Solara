package ares

import (
	"strconv"
	"strings"

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

func boardTitleData() (out map[string]string) {
	out = map[string]string{}
	defer func() { _ = recover() }() // a missing DB or collection just means no extras
	all := a2Deployments().FindAll()
	dep := a2LatestOnlineDeployment(all)
	if dep == nil {
		// The watchdog can leave a live, heartbeating deployment flagged offline after a restart (seen
		// 2026-11-01): fall back to the deployment that reported most recently.
		for _, d := range all {
			if a2IsNullOrEmpty(d.StationId) || d.LastEvent == nil {
				continue
			}
			if dep == nil || d.LastEvent.After(*dep.LastEvent) {
				dep = d
			}
		}
	}
	if dep == nil {
		return out
	}
	st := a2FindStation(a2Deref(dep.StationId))
	if st == nil || st.Config == nil {
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
