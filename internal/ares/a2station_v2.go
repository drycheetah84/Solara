// Solara extension (not in the original route table): the 50451 ("pod-cosmetics") client's
// /v1/users/log_in_with_key parses FLoginServerResponse = {api_key, user, permissions:[{fleet_id, permission}],
// bans}. Fleets replaced stations in 50451; Solara has no fleet model, so a permission's fleet_id is its
// station_id (the same id /v2/fleets and /v2/stations report as fleet_id).
package ares

import (
	"solara/internal/ares/models"
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
