// Solara extension (not in the original route table): the 50451 ("pod-cosmetics") client's
// /v1/users/log_in_with_key parses FLoginServerResponse = {api_key, user, permissions:[{fleet_id, permission}],
// bans}. Fleets replaced stations in 50451; Solara has no fleet model, so a permission's fleet_id is its
// station_id (the same id /v2/fleets and /v2/stations report as fleet_id).
package ares

import (
	"fmt"

	"solara/internal/ares/models"
	"solara/internal/dotnet"
	"solara/internal/hosting"
)

// v1 login-with-key for 50451: FLoginServerResponse = {api_key, user, permissions:[{fleet_id, permission}], bans}.
// Same data as the v0 response; each permission also carries fleet_id (== station_id) and bans is present.
type v1UserPermission struct {
	StationId  *string `json:"station_id"`
	FleetId    *string `json:"fleet_id"`
	Permission *string `json:"permission"`
}

type v1LogInServerResponse struct {
	ApiKey      *string             `json:"api_key"`
	User        *models.UserData    `json:"user"`
	Permissions []*v1UserPermission `json:"permissions"`
	Bans        []any               `json:"bans"`
}

func v1LoginFrom(r hosting.Result) hosting.Result {
	type contentHolder interface{ Content() any }
	ch, ok := r.(contentHolder)
	if !ok {
		return r
	}
	v0, ok := ch.Content().(*models.LogInServerResponse)
	if !ok || v0 == nil {
		return r
	}
	out := &v1LogInServerResponse{ApiKey: v0.ApiKey, User: v0.User, Permissions: []*v1UserPermission{}, Bans: []any{}}
	for _, p := range v0.Permissions {
		if p == nil {
			continue
		}
		// 50451 permission check (0x56BD700) grants a permission only when the entry's fleet equals the current
		// fleet or is FName None (empty) -- None is the wildcard. "global" would become FName "Global", match
		// nothing, and the player would hold every permission yet pass no check. Global perms go out empty.
		fleet := p.StationId
		if fleet == nil || *fleet == "" || a2EqFoldS(fleet, "global") {
			fleet = a2Str("")
		}
		out.Permissions = append(out.Permissions, &v1UserPermission{StationId: p.StationId, FleetId: fleet, Permission: p.Permission})
	}
	return hosting.Ok(out)
}

// v50StationItem is what the 50451 dashboard imports each /v2/stations item into: FJoinableStationResponse
// (344 B; StationDashboardManager+48, read by the session builder 0x567D930), NOT FStationResponse. Its config
// is a TMap<FString,FString> -- the session builder reads "max_players" / "num_reserved_slots" from it -- and it
// carries fleet_name, is_public and is_allowlist. A string config fails the whole page import (the browser then
// shows nothing at all); without is_public the station is not offered.
type v50StationItem struct {
	StationId           *string           `json:"station_id"`
	FleetId             *string           `json:"fleet_id"`
	FleetName           *string           `json:"fleet_name"`
	SessionId           *string           `json:"session_id"`
	StationName         *string           `json:"station_name"`
	Region              string            `json:"region"`
	Ip                  *string           `json:"ip"`
	Version             *string           `json:"version"`
	DeploymentCl        *string           `json:"deployment_cl"`
	Created             dotnet.DateTime   `json:"created"`
	Online              bool              `json:"online"`
	LastEvent           dotnet.DateTime   `json:"last_event"`
	PlayerCount         int               `json:"player_count"`
	Disabled            bool              `json:"disabled"`
	Config              map[string]string `json:"config"`
	DistrictPopulations map[string]int    `json:"district_populations"`
	IsPublic            bool              `json:"is_public"`
	IsAllowlist         bool              `json:"is_allowlist"`
}

type v50StationsPage struct {
	FleetId string            `json:"fleet_id"`
	Items   []*v50StationItem `json:"items"`
}

// a2IsV50Client: the 50451 browser sends version=<NetCL 5xxxx>.
func a2IsV50Client(version string) bool { return len(version) == 5 && version[0] == '5' }

// a2StationsPageFor50451 converts the v2 page to the 50451 import shape and drops an EOS-session duplicate of
// a registered deployment (same ip) -- the registered row carries the server's real reported version.
func a2StationsPageFor50451(p *models.StationsPageV2, version string) *v50StationsPage {
	out := &v50StationsPage{FleetId: p.FleetId, Items: []*v50StationItem{}}
	hasExact := map[string]bool{}
	for _, it := range p.Items {
		if it != nil && it.Ip != nil && it.Version != nil && *it.Version == version {
			hasExact[*it.Ip] = true
		}
	}
	for _, it := range p.Items {
		if it == nil {
			continue
		}
		if it.Ip != nil && hasExact[*it.Ip] && (it.Version == nil || *it.Version != version) {
			continue
		}
		cfg := map[string]string{"max_players": "75", "num_reserved_slots": "0"}
		for k, v := range it.Config {
			if v != nil {
				cfg[k] = fmt.Sprint(v)
			}
		}
		last := it.Created
		if it.LastEvent != nil {
			last = *it.LastEvent
		}
		fleetName := it.StationName
		if it.FleetId != nil {
			if st := a2FindStation(*it.FleetId); st != nil && st.StationName != nil {
				fleetName = st.StationName
			}
		}
		out.Items = append(out.Items, &v50StationItem{
			StationId: it.StationId, FleetId: it.FleetId, FleetName: fleetName, SessionId: it.SessionId,
			StationName: it.StationName, Region: a2Deref(it.Region), Ip: it.Ip, Version: it.Version,
			DeploymentCl: it.DeploymentCl, Created: it.Created, Online: it.Online, LastEvent: last,
			PlayerCount: it.PlayerCount, Disabled: it.Disabled, Config: cfg, DistrictPopulations: districtPopsFor(it.StationId, it.Ip),
			IsPublic: true, IsAllowlist: false,
		})
	}
	return out
}
