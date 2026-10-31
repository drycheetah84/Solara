// Solara extension (not in the original route table): the 50451 ("pod-cosmetics") client's
// /v1/users/log_in_with_key parses FLoginServerResponse = {api_key, user, permissions:[{fleet_id, permission}],
// bans}. Fleets replaced stations in 50451; Solara has no fleet model, so a permission's fleet_id is its
// station_id (the same id /v2/fleets and /v2/stations report as fleet_id).
package ares

import (
	"encoding/json"

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
		out.Permissions = append(out.Permissions, &v1UserPermission{StationId: p.StationId, FleetId: p.StationId, Permission: p.Permission})
	}
	return hosting.Ok(out)
}

// v50StationItem is FStationResponse as the 50451 client imports it: config is an FString (the existing v2
// shape sends a JSON object, written for the 29932 client), district_populations a TMap<FString,int32>, and
// last_event an FDateTime. FJsonObjectConverter fails the whole item on a type mismatch, which left the 50451
// station browser with a fleet but no stations under it.
type v50StationItem struct {
	StationId           *string         `json:"station_id"`
	FleetId             *string         `json:"fleet_id"`
	SessionId           *string         `json:"session_id"`
	StationName         *string         `json:"station_name"`
	Region              string          `json:"region"`
	Ip                  *string         `json:"ip"`
	Version             *string         `json:"version"`
	DeploymentCl        *string         `json:"deployment_cl"`
	Created             dotnet.DateTime `json:"created"`
	Online              bool            `json:"online"`
	LastEvent           dotnet.DateTime `json:"last_event"`
	PlayerCount         int             `json:"player_count"`
	Disabled            bool            `json:"disabled"`
	Config              string          `json:"config"`
	DistrictPopulations map[string]int  `json:"district_populations"`
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
		cfg := "{}"
		if len(it.Config) > 0 {
			if b, err := json.Marshal(it.Config); err == nil {
				cfg = string(b)
			}
		}
		last := it.Created
		if it.LastEvent != nil {
			last = *it.LastEvent
		}
		out.Items = append(out.Items, &v50StationItem{
			StationId: it.StationId, FleetId: it.FleetId, SessionId: it.SessionId, StationName: it.StationName,
			Region: a2Deref(it.Region), Ip: it.Ip, Version: it.Version, DeploymentCl: it.DeploymentCl,
			Created: it.Created, Online: it.Online, LastEvent: last, PlayerCount: it.PlayerCount,
			Disabled: it.Disabled, Config: cfg, DistrictPopulations: map[string]int{},
		})
	}
	return out
}
