// Ports the FLEETS and V2 CLIENT API sections of
// AUnrealFeatures.Ares/Servers/A2StationDbServer.cs: /v2/fleets,
// /v2/fleets/{fleet_id}, /v2/stations and the EOS-session -> deployment
// mappers. Model note: StationDbObject == v2 "fleet", DeploymentDbObject ==
// v2 "station".
package ares

import (
	"sort"
	"strconv"

	"solara/internal/ares/models"
	"solara/internal/dotnet"
	eosmodels "solara/internal/eos/models"
	"solara/internal/hosting"
)

// a2ToTypedConfig mirrors ToTypedConfig: bool, then double (NumberStyles.Any,
// invariant), else the string. null for a null/empty config. The original
// fills a Dictionary<string, object> by enumerating the source dictionary;
// Go map iteration is unordered, so keys are visited in sorted order (the
// encoder sorts map keys anyway).
func a2ToTypedConfig(config map[string]*string) map[string]any {
	if len(config) == 0 {
		return nil
	}
	keys := make([]string, 0, len(config))
	for k := range config {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make(map[string]any, len(config))
	for _, k := range keys {
		v := config[k]
		if v == nil {
			// bool.TryParse(null) / double.TryParse(null) are false: the
			// null value is kept.
			result[k] = nil
		} else if b, ok := a2TryParseBool(*v); ok {
			result[k] = b
		} else if d, ok := a2TryParseDouble(*v); ok {
			result[k] = d
		} else {
			result[k] = *v
		}
	}
	return result
}

// a2EosSessionToDeployment mirrors EosSessionToDeployment.
func a2EosSessionToDeployment(s *eosmodels.EosSessionInfo) *models.DeploymentDbObject {
	ip := s.IpAddress
	if s.Port != "" {
		ip = s.IpAddress + ":" + s.Port
	}
	d := models.NewDeploymentDbObject()
	d.DeploymentId = a2Str(s.Id)
	d.StationId = a2Coalesce(s.StationId, a2Str(s.Bucket))
	d.DeploymentName = a2Str(s.ServerName)
	d.IpAddress = a2Str(ip)
	d.Version = a2Str(strconv.Itoa(s.BuildId))
	d.CreatedAt = dotnet.UtcNow()
	d.Online = true
	if s.PublicPlayers == nil {
		panic(a2NullReference())
	}
	d.PlayerCount = len(s.PublicPlayers)
	d.Config = map[string]*string{
		"port":        a2Str(s.Port),
		"imgui_port":  a2Str(s.ImguiPort),
		"bucket":      a2Str(s.Bucket),
		"max_players": a2Str(strconv.Itoa(s.MaxPlayers)),
	}
	return d
}

// a2DeploymentToFleetStation mirrors DeploymentToFleetStation.
func a2DeploymentToFleetStation(d *models.DeploymentDbObject) *models.FleetStationResponse {
	r := &models.FleetStationResponse{
		StationId:           d.DeploymentId,
		FleetId:             d.StationId,
		SessionId:           d.DeploymentId,
		StationName:         d.DeploymentName,
		Region:              d.Region,
		Ip:                  d.IpAddress,
		Version:             d.Version,
		DeploymentCl:        d.Version,
		Created:             d.CreatedAt,
		Online:              d.Online,
		LastEvent:           d.LastEvent,
		PlayerCount:         d.PlayerCount,
		Disabled:            false,
		DistrictPopulations: nil,
	}
	if len(d.Config) > 0 {
		r.Config = a2ToTypedConfig(d.Config)
	}
	return r
}

// a2StationToFleet mirrors StationToFleet(s, allDeps, includeStations, includeConfig).
// allDeps == nil is the null IEnumerable.
func a2StationToFleet(s *models.StationDbObject, allDeps []*models.DeploymentDbObject, includeStations, includeConfig bool) *models.FleetResponse {
	r := &models.FleetResponse{FleetId: s.StationId, FleetName: s.StationName, Created: s.CreatedAt}
	// [2026-09-13] Only DERIVE online from deployments when some were loaded;
	// otherwise trust the station row.
	if len(allDeps) > 0 {
		for _, d := range allDeps {
			if d.Online {
				r.Online = true
				break
			}
		}
	} else {
		r.Online = s.Online
	}
	if includeStations && allDeps != nil {
		r.Stations = []*models.FleetStationResponse{}
		for _, d := range allDeps {
			r.Stations = append(r.Stations, a2DeploymentToFleetStation(d))
		}
	}
	if includeConfig && len(s.Config) > 0 {
		r.Config = a2ToTypedConfig(s.Config)
	}
	return r
}

// a2EosDepsOfStation mirrors GetEosSessions().Where(s => (s.StationId ?? s.Bucket) == id).Select(EosSessionToDeployment).
func a2EosDepsOfStation(sessions []*eosmodels.EosSessionInfo, id string) []*models.DeploymentDbObject {
	out := []*models.DeploymentDbObject{}
	for _, se := range sessions {
		if a2Deref(a2Coalesce(se.StationId, a2Str(se.Bucket))) == id {
			out = append(out, a2EosSessionToDeployment(se))
		}
	}
	return out
}

// a2DbDepsOfStation mirrors (deploymentCollection?.Find(d => d.StationId == id) ?? Enumerable.Empty).
func a2DbDepsOfStation(id *string) []*models.DeploymentDbObject {
	deps := a2DeploymentsOfStation(id)
	if deps == nil {
		deps = []*models.DeploymentDbObject{}
	}
	return deps
}

// FetchAllFleetsV2 handles GET /v2/fleets: paginated fleets.
func (s *A2StationDbServer) FetchAllFleetsV2(c *hosting.Context) hosting.Result {
	includeConfig := a2ParseBool(a2QueryOr(c, "include_config", "false"))
	includeStations := a2ParseBool(a2QueryOr(c, "include_stations", "false"))
	includeOffline := a2ParseBool(a2QueryOr(c, "include_offline_fleets", "true"))
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "100"))
	page := a2ParseInt(a2QueryOr(c, "page", "1"))

	fleets := []*models.StationDbObject{}
	if col := a2Stations(); col != nil {
		fleets = col.FindAll()
	}
	if !includeOffline {
		online := []*models.StationDbObject{}
		for _, f := range fleets {
			if f.Online {
				online = append(online, f)
			}
		}
		fleets = online
	}

	// ToLookup(s => s.StationId ?? s.Bucket, EosSessionToDeployment) — evaluated eagerly.
	sessions := a2GetEosSessions()
	eosByStation := map[string][]*models.DeploymentDbObject{}
	for _, se := range sessions {
		k := a2Deref(a2Coalesce(se.StationId, a2Str(se.Bucket)))
		eosByStation[k] = append(eosByStation[k], a2EosSessionToDeployment(se))
	}

	responses := []*models.FleetResponse{}
	for _, f := range fleets {
		deps := []*models.DeploymentDbObject{}
		if includeStations {
			deps = append(a2DbDepsOfStation(f.StationId), eosByStation[a2Deref(f.StationId)]...)
			if f.StationId == nil {
				deps = a2DbDepsOfStation(f.StationId) // Lookup[null] yields an empty grouping
			}
		}
		responses = append(responses, a2StationToFleet(f, deps, includeStations, includeConfig))
	}

	return hosting.Ok(models.Create[*models.FleetResponse, models.FleetResponsePage](responses, pageSize, page))
}

// GetFleetV2 handles GET /v2/fleets/{fleet_id}: a single fleet.
func (s *A2StationDbServer) GetFleetV2(c *hosting.Context) hosting.Result {
	fleetId := c.Param("fleet_id")
	includeConfig := a2ParseBool(a2QueryOr(c, "include_config", "true"))
	includeStations := a2ParseBool(a2QueryOr(c, "include_stations", "true"))

	fleet := a2FindStation(fleetId)
	if fleet == nil {
		return hosting.Ok(nil)
	}
	deps := []*models.DeploymentDbObject{}
	if includeStations {
		deps = append(a2DbDepsOfStation(&fleetId), a2EosDepsOfStation(a2GetEosSessions(), fleetId)...)
	}
	return hosting.Ok(a2StationToFleet(fleet, deps, includeStations, includeConfig))
}

// FetchAllStationsV2 handles GET /v2/stations: the 29932 browse list; every
// deployment (DB + live EOS session) as an FStationResponse item, deduped by
// deployment id. The version param is deliberately not used as a filter.
func (s *A2StationDbServer) FetchAllStationsV2(c *hosting.Context) hosting.Result {
	includeOffline := a2ParseBool(a2QueryOr(c, "include_offline", "true"))

	deps := []*models.DeploymentDbObject{}
	if col := a2Deployments(); col != nil {
		deps = col.FindAll()
	}
	for _, se := range a2GetEosSessions() {
		deps = append(deps, a2EosSessionToDeployment(se))
	}
	if !includeOffline {
		online := []*models.DeploymentDbObject{}
		for _, d := range deps {
			if d.Online {
				online = append(online, d)
			}
		}
		deps = online
	}

	// Dedupe by deployment id (a registered server also shows as an EOS session).
	seen := map[string]bool{}
	items := []*models.FleetStationResponse{}
	for _, d := range deps {
		if !a2IsNullOrEmpty(d.DeploymentId) && !seen[*d.DeploymentId] {
			seen[*d.DeploymentId] = true
			items = append(items, a2DeploymentToFleetStation(d))
		}
	}
	return hosting.Ok(&models.StationsPageV2{FleetId: "", Items: items})
}
