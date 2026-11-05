// Ports the DEPLOYMENTS and Spec Editor sections of
// AUnrealFeatures.Ares/Servers/A2StationDbServer.cs: deployment CRUD, server
// events, deployment config (v0/v1, with station config + board defaults),
// GET /v1/deployments/{id}, netvar overrides and the v1/spec/* level API.
package ares

import (
	"strings"

	"solara/internal/ares/models"
	"solara/internal/dotnet"
	"solara/internal/eos"
	"solara/internal/halcyonsocket"
	"solara/internal/hosting"
	"solara/internal/logging"
)

// a2FindDeployment mirrors deploymentCollection?.FindOne(d => d.DeploymentId == id) (collation).
func a2FindDeployment(id string) *models.DeploymentDbObject {
	col := a2Deployments()
	if col == nil {
		return nil
	}
	return col.FindOne(func(d *models.DeploymentDbObject) bool { return a2EqFoldS(d.DeploymentId, id) })
}

// a2ToExpanded mirrors the ServerDeploymentResponse -> DeploymentExpanded projection.
func a2ToExpanded(d *models.ServerDeploymentResponse) *models.DeploymentExpanded {
	return &models.DeploymentExpanded{
		DeploymentRequest: models.DeploymentRequest{
			StationId:      d.StationId,
			DeploymentName: d.DeploymentName,
			IpAddress:      d.IpAddress,
			Region:         d.Region,
			Version:        d.Version,
		},
		CreatedAt:   d.CreatedAt,
		Online:      d.Online,
		LastEvent:   d.LastEventAt,
		PlayerCount: d.PlayerCount,
		Config:      d.Config,
	}
}

// CreateDeployment handles POST /stations/{station_id}/deployments.
func (s *A2StationDbServer) CreateDeployment(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := a2Deserialize(c.Request.Body(), models.NewDeploymentRequest)

	deploymentId := a2GenerateId()
	apiKey := a2GenerateApiKey(&deploymentId, "service")

	dep := models.NewDeploymentDbObject()
	dep.DeploymentId = a2Str(deploymentId)
	dep.StationId = a2Str(stationId)
	if body != nil {
		dep.DeploymentName = body.DeploymentName
		dep.IpAddress = body.IpAddress
		dep.Region = body.Region
		dep.Version = body.Version
	}
	dep.CreatedAt = dotnet.Now()
	dep.Online = false
	dep.PlayerCount = 0
	dep.Config = map[string]*string{}
	dep.ApiKeys = []string{apiKey}
	if col := a2Deployments(); col != nil {
		col.Insert(dep)
	}
	return hosting.Ok(&models.CreateDeploymentResponse{ApiKey: a2Str(apiKey), DeploymentId: a2Str(deploymentId)})
}

// a2StationDeploymentsWithEos mirrors
// (Find(d => d.StationId == id) ?? Empty).Concat(GetEosSessions().Where(...).Select(EosSessionToDeployment)).
func a2StationDeploymentsWithEos(stationId string) []*models.DeploymentDbObject {
	return append(a2DbDepsOfStation(&stationId), a2EosDepsOfStation(a2GetEosSessions(), stationId)...)
}

// GetStationDeploymentsV0 handles GET /stations/{station_id}/deployments (deprecated v0).
func (s *A2StationDbServer) GetStationDeploymentsV0(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	expanded := []*models.DeploymentExpanded{}
	for _, d := range a2StationDeploymentsWithEos(stationId) {
		expanded = append(expanded, a2ToExpanded(a2DeploymentToResponse(d)))
	}
	return hosting.Ok(&models.StationDeploymentResponse{StationId: a2Str(stationId), Deployments: expanded})
}

// GetStationDeploymentsV1 handles GET /v1/stations/{station_id}/deployments.
func (s *A2StationDbServer) GetStationDeploymentsV1(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "500"))
	page := a2ParseInt(a2QueryOr(c, "page", "1"))
	includeOffline := a2ParseBool(a2QueryOr(c, "include_offline", "false"))

	deployments := a2StationDeploymentsWithEos(stationId)
	if !includeOffline {
		online := []*models.DeploymentDbObject{}
		for _, d := range deployments {
			if d.Online {
				online = append(online, d)
			}
		}
		deployments = online
	}

	items := []*models.ServerDeploymentResponse{}
	for _, d := range deployments {
		items = append(items, a2DeploymentToResponse(d))
	}
	paged := models.Create[*models.ServerDeploymentResponse, models.ServerDeploymentResponsePage](items, pageSize, page)

	out := &models.DeploymentsResponsePage{StationId: a2Str(stationId)}
	out.Page = paged.Page
	out.Items = []*models.DeploymentExpanded{}
	for _, d := range paged.Items {
		out.Items = append(out.Items, a2ToExpanded(d))
	}
	return hosting.Ok(out)
}

// CreateDeploymentApiKey handles POST /deployments/{deployment_id}/api_key.
func (s *A2StationDbServer) CreateDeploymentApiKey(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	deployment := a2FindDeployment(deploymentId)
	if deployment == nil {
		return hosting.Ok(&models.ApiKeyResponse{ApiKey: a2Str("")})
	}
	newKey := a2GenerateApiKey(&deploymentId, "service")
	if deployment.ApiKeys == nil {
		deployment.ApiKeys = []string{}
	}
	deployment.ApiKeys = append(deployment.ApiKeys, newKey)
	a2Deployments().Update(deployment)
	return hosting.Ok(&models.ApiKeyResponse{ApiKey: a2Str(newKey)})
}

// a2MaxStoredEventBytes mirrors MaxStoredEventBytes: only small events are persisted.
const a2MaxStoredEventBytes = 8192

// AddServerEvent handles POST /deployments/{deployment_id}/server_events.
func (s *A2StationDbServer) AddServerEvent(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	body := a2Deserialize(c.Request.Body(), models.NewServerEventRequest)
	if body == nil || a2IsNullOrEmpty(body.EventType) {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	// Keep the latest full netvar/state dump for the dashboard's netvar editor.
	Record(deploymentId, body.EventType, body.EventData)

	// Only persist small events ("netvars" ~1.6 MB / "state" ~40 KB are skipped).
	if a2IsNullOrEmpty(body.EventData) || a2UTF16Len(*body.EventData) <= a2MaxStoredEventBytes {
		if col := a2ServerEvents(); col != nil {
			ev := models.NewServerEventDbObject()
			ev.DeploymentId = a2Str(deploymentId)
			ev.EventType = body.EventType
			ev.EventData = body.EventData
			ev.Timestamp = dotnet.Now()
			col.Insert(ev)
		}
	}

	// Update online status if this deployment has a real DB row (a synthetic id
	// such as "halcyon" has none; success is still acknowledged).
	deployment := a2FindDeployment(deploymentId)
	if deployment != nil {
		now := dotnet.Now()
		deployment.Online = true
		deployment.LastEvent = &now
		a2Deployments().Update(deployment)

		if stationCollection := a2Stations(); stationCollection != nil {
			station := stationCollection.FindOne(func(st *models.StationDbObject) bool {
				return a2EqFoldPtr(st.StationId, deployment.StationId)
			})
			if station != nil {
				lo := dotnet.Now()
				station.Online = true
				station.LastOnline = &lo
				stationCollection.Update(station)
			}
		}
	}

	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// UpdateDeployment handles PATCH /deployments/{deployment_id}.
func (s *A2StationDbServer) UpdateDeployment(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewDeploymentRequest)
	deployment := a2FindDeployment(c.Param("deployment_id"))
	if deployment == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	if body != nil {
		if body.DeploymentName != nil {
			deployment.DeploymentName = body.DeploymentName
		}
		if body.IpAddress != nil {
			deployment.IpAddress = body.IpAddress
		}
		if body.Region != nil {
			deployment.Region = body.Region
		}
		if body.Version != nil {
			deployment.Version = body.Version
		}
	}
	a2Deployments().Update(deployment)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// DeleteDeployment handles DELETE /deployments/{deployment_id}: stops the
// server via the watchdog, drops its EOS sessions, deletes the row and its
// server events.
func (s *A2StationDbServer) DeleteDeployment(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	deployment := a2FindDeployment(deploymentId)
	if deployment == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	stop := "allocator not running"
	if inst := halcyonsocket.Instance; inst != nil {
		stop = inst.Watchdog.Stop(deploymentId, "deployment deleted from the dashboard")
	}
	logging.Console("[Deployments] delete " + deploymentId + ": " + stop)
	eos.RemoveSessionsByDeployment(deploymentId)

	a2Deployments().Delete(deployment.Id)

	// delete all server events for this deployment
	if col := a2ServerEvents(); col != nil {
		for _, ev := range col.Find(func(e *models.ServerEventDbObject) bool { return a2EqFoldS(e.DeploymentId, deploymentId) }) {
			col.Delete(ev.Id)
		}
	}
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// a2AddStationConfig mirrors the shared "include station config" block:
// station keys (minus the nv. overrides), DefaultStationConfig, then the
// Halcyon board defaults, all via TryAdd.
func a2AddStationConfig(config map[string]*string, deployment *models.DeploymentDbObject, host string) {
	if deployment != nil {
		if stationCollection := a2Stations(); stationCollection != nil {
			station := stationCollection.FindOne(func(st *models.StationDbObject) bool {
				return a2EqFoldPtr(st.StationId, deployment.StationId)
			})
			if station != nil {
				for k, v := range station.Config {
					if !strings.HasPrefix(k, NetvarOverridePrefix) { // overrides go via netvar_overrides
						a2TryAdd(config, k, v)
					}
				}
			}
		}
	}
	// Halcyon board/sign URLs (served even for an unknown/fixed deployment id).
	for _, kv := range a2DefaultStationConfig {
		a2TryAdd(config, kv.Key, a2Str(kv.Value))
	}
	AddBoardDefaults(config, host)
}

// GetDeploymentConfigV0 handles GET /deployments/{deployment_id}/config (deprecated v0).
func (s *A2StationDbServer) GetDeploymentConfigV0(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	includeStation := a2ParseBool(a2QueryOr(c, "include_station_config", "true"))

	deployment := a2FindDeployment(deploymentId)
	if deployment == nil {
		for _, se := range a2GetEosSessions() {
			if d := a2EosSessionToDeployment(se); a2EqS(d.DeploymentId, deploymentId) {
				deployment = d
				break
			}
		}
	}

	config := map[string]*string{}
	if deployment != nil {
		config = a2CopyMap(deployment.Config)
	}
	if includeStation {
		a2AddStationConfig(config, deployment, c.Request.Host())
	}
	return hosting.Ok(config)
}

// GetDeploymentV1 handles GET /v1/deployments/{deployment_id}: what the
// game's dedicated server fetches with include_station_config=true.
func (s *A2StationDbServer) GetDeploymentV1(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	includeDeploymentConfig := a2ParseBool(a2QueryOr(c, "include_deployment_config", "true"))
	includeStationConfig := a2ParseBool(a2QueryOr(c, "include_station_config", "true"))

	deploymentCollection := a2Deployments()
	deployment := a2FindDeployment(deploymentId)
	if deployment == nil {
		var eosHit bool
		for _, se := range a2GetEosSessions() {
			if se.Id == deploymentId {
				deployment = a2EosSessionToDeployment(se)
				eosHit = true
				break
			}
		}
		if !eosHit {
			// Synthetic deployment so a FIXED launch arg (-DashboardDeploymentId=halcyon)
			// still resolves, carrying the station of the most recent online deployment.
			var latest *models.DeploymentDbObject
			if deploymentCollection != nil {
				latest = a2LatestOnlineDeployment(deploymentCollection.FindAll())
			}
			d := models.NewDeploymentDbObject()
			d.DeploymentId = a2Str(deploymentId)
			d.StationId, d.DeploymentName, d.Region, d.IpAddress = a2Str(""), a2Str("Halcyon"), a2Str("NAE"), a2Str("")
			if latest != nil {
				d.StationId = a2Coalesce(latest.StationId, a2Str(""))
				d.DeploymentName = a2Coalesce(latest.DeploymentName, a2Str("Halcyon"))
				d.Region = a2Coalesce(latest.Region, a2Str("NAE"))
				d.IpAddress = a2Coalesce(latest.IpAddress, a2Str(""))
			}
			d.Version = a2Str(a2BuildVersion)
			d.CreatedAt = dotnet.Now()
			d.Online = true
			d.PlayerCount = 0
			d.Config = map[string]*string{}
			deployment = d
		}
	}

	resp := a2DeploymentToResponse(deployment)
	// Station config (incl. the board defaults) is served whenever
	// include_station_config=true, independent of include_deployment_config.
	if resp.Config == nil {
		resp.Config = map[string]*string{}
	}
	if !includeDeploymentConfig {
		resp.Config = map[string]*string{}
	}
	if includeStationConfig {
		a2AddStationConfig(resp.Config, deployment, c.Request.Host())
	}
	return hosting.Ok(resp)
}

// GetNetvarOverrides handles GET /v1/deployments/{deployment_id}/netvar_overrides:
// plain text "v=<sha1>\nmodule\t...\nworld\t..." polled by the injected DLL.
func (s *A2StationDbServer) GetNetvarOverrides(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	deploymentCollection := a2Deployments()
	deployment := a2FindDeployment(deploymentId)
	stationId := ""
	if deployment != nil {
		stationId = a2Deref(deployment.StationId)
	}
	if stationId == "" {
		// Same fallback as GetDeploymentV1: the most recent online deployment's station.
		if deploymentCollection != nil {
			if latest := a2LatestOnlineDeployment(deploymentCollection.FindAll()); latest != nil {
				stationId = a2Deref(latest.StationId)
			}
		}
	}
	var station *models.StationDbObject
	if stationId != "" {
		station = a2FindStation(stationId)
	}

	// Solara: ?v=<version> is the game server's fast poll (game servers on Azure are not co-located with the
	// backend, so they cannot watch the files below): an unchanged set answers 200 with an empty body, and a poll
	// never rewrites files or re-pushes the browser whitelists. Without ?v= this is the original startup fetch.
	if have, ok := c.Request.Query("v"); ok {
		text := BuildText(station)
		if strings.HasPrefix(text, "v="+have+"\n") || text == "v="+have {
			return hosting.Configurable(200, "text/plain; charset=utf-8", nil)
		}
		return hosting.Configurable(200, "text/plain; charset=utf-8", []byte(text))
	}
	// The game server calls this once at startup; make sure the watched files exist too.
	EnsureAllWritten()
	PushAll() // and that the EOS gateway has every station's browser whitelist
	return hosting.Configurable(200, "text/plain; charset=utf-8", []byte(BuildText(station)))
}

// ─── Spec Editor saved levels (game server only) ─────────────────────────
// Plain text, TAB-separated fields. {name} is [A-Za-z0-9 _-]{1,64}
// (percent-encode spaces as %20); anything else is 400.

func a2SpecText(text string) hosting.Result {
	return hosting.Configurable(200, "text/plain; charset=utf-8", []byte(text))
}

func a2SpecErr(code int, text string) hosting.Result {
	return hosting.Configurable(code, "text/plain; charset=utf-8", []byte(text))
}

func a2SpecUnauthorized() hosting.Result { return a2SpecErr(401, "unauthorized") }

func a2SpecBadName() hosting.Result { return a2SpecErr(400, "bad level name") }

func a2Bit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// SpecListLevels handles GET /v1/spec/levels -> name\tautoload\tloaded\tupdated\tsize.
func (s *A2StationDbServer) SpecListLevels(c *hosting.Context) hosting.Result {
	var lines []string
	for _, l := range SpecLevelsStoreList() {
		lines = append(lines, l.Name+"\t"+a2Bit(l.Entry.Autoload)+"\t"+a2Bit(l.Entry.Loaded)+"\t"+l.Entry.Updated+"\t"+a2Itoa(l.Entry.Size))
	}
	return a2SpecText(strings.Join(lines, "\n"))
}

// SpecGetLevel handles GET /v1/spec/levels/{name} -> the level text (404 if missing).
func (s *A2StationDbServer) SpecGetLevel(c *hosting.Context) hosting.Result {
	name := c.Param("name")
	n := SpecLevelsStoreCleanName(&name)
	if n == nil {
		return a2SpecBadName()
	}
	text := SpecLevelsStoreReadText(*n)
	if text == nil {
		return a2SpecErr(404, "not found")
	}
	return a2SpecText(*text)
}

// SpecPutLevel handles PUT /v1/spec/levels/{name} (body = level text) -> "ok".
func (s *A2StationDbServer) SpecPutLevel(c *hosting.Context) hosting.Result {
	name := c.Param("name")
	n := SpecLevelsStoreCleanName(&name)
	if n == nil {
		return a2SpecBadName()
	}
	SpecLevelsStorePut(*n, c.Request.Body())
	return a2SpecText("ok")
}

// SpecSetLoaded handles POST /v1/spec/levels/{name}/loaded?value=0|1 -> "ok".
func (s *A2StationDbServer) SpecSetLoaded(c *hosting.Context) hosting.Result {
	name := c.Param("name")
	n := SpecLevelsStoreCleanName(&name)
	if n == nil {
		return a2SpecBadName()
	}
	v := strings.ToLower(a2Trim(a2QueryOr(c, "value", "")))
	var value *bool
	switch v {
	case "1", "true":
		t := true
		value = &t
	case "0", "false":
		f := false
		value = &f
	}
	if value == nil {
		return a2SpecErr(400, "value must be 0 or 1")
	}
	if !SpecLevelsStoreUpdate(*n, nil, value) {
		return a2SpecErr(404, "not found")
	}
	return a2SpecText("ok")
}

// SpecBoot handles POST /v1/spec/boot?deployment={id} -> names to load.
func (s *A2StationDbServer) SpecBoot(c *hosting.Context) hosting.Result {
	dep := a2Trim(a2QueryOr(c, "deployment", ""))
	return a2SpecText(strings.Join(SpecLevelsStoreBoot(dep), "\n"))
}

// SpecDesired handles GET /v1/spec/desired -> names with loaded == true.
func (s *A2StationDbServer) SpecDesired(c *hosting.Context) hosting.Result {
	return a2SpecText(strings.Join(SpecLevelsStoreDesired(), "\n"))
}

// SpecStatus handles POST /v1/spec/status?deployment={id} (body = loaded
// names, one per line) -> "ok".
func (s *A2StationDbServer) SpecStatus(c *hosting.Context) hosting.Result {
	dep := a2Trim(a2QueryOr(c, "deployment", ""))
	if dep == "" {
		return a2SpecErr(400, "deployment required")
	}
	var names []string
	for _, l := range strings.Split(a2DecodeUTF8NoBOM(c.Request.Body()), "\n") {
		l = strings.Trim(l, "\r \t")
		if len(l) > 0 {
			names = append(names, l)
		}
	}
	SpecLevelsStoreStatus(dep, names)
	return a2SpecText("ok")
}

// GetDeploymentConfigV1 handles GET /v1/deployments/{deployment_id}/config.
func (s *A2StationDbServer) GetDeploymentConfigV1(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	includeStation := a2ParseBool(a2QueryOr(c, "include_station_config", "true"))
	includeEvent := a2ParseBool(a2QueryOr(c, "include_event_config", "true"))

	deployment := a2FindDeployment(deploymentId)
	config := map[string]*string{}
	if deployment != nil {
		config = a2CopyMap(deployment.Config)
	}
	if includeStation {
		a2AddStationConfig(config, deployment, c.Request.Host())
	}

	if includeEvent {
		now := dotnet.Now()
		var active *models.StationEventDbObject
		if col := a2StationEvents(); col != nil {
			for _, e := range col.FindAll() {
				if deployment == nil {
					// Preserved bug: `e.StationId == deployment.StationId` dereferences
					// a null deployment as soon as any event exists.
					panic(a2NullReference())
				}
				if !a2EqPtr(e.StationId, deployment.StationId) {
					continue
				}
				if !(a2EqS(e.DeploymentId, deploymentId) || a2IsNullOrEmpty(e.DeploymentId)) {
					continue
				}
				if !e.StartTime.After(now) && !e.StartTime.AddSeconds(float64(e.Duration)).Before(now) {
					active = e
					break
				}
			}
		}
		if active != nil {
			for k, v := range active.Config {
				a2TryAdd(config, k, v)
			}
		}
	}
	return hosting.Ok(config)
}

// SetDeploymentConfigV1 handles POST /v1/deployments/{deployment_id}/config: merge keys.
func (s *A2StationDbServer) SetDeploymentConfigV1(c *hosting.Context) hosting.Result {
	data, ok := a2DeserializeStringDict(c.Request.Body())
	deployment := a2FindDeployment(c.Param("deployment_id"))
	if deployment == nil {
		return hosting.Ok(map[string]string{})
	}
	if ok {
		a2MergeConfig(deployment.Config, data)
	}
	a2Deployments().Update(deployment)
	return hosting.Ok(deployment.Config)
}

// DeleteDeploymentConfigV1 handles DELETE /v1/deployments/{deployment_id}/config: remove keys.
func (s *A2StationDbServer) DeleteDeploymentConfigV1(c *hosting.Context) hosting.Result {
	keys, ok := a2DeserializeStringList(c.Request.Body())
	deployment := a2FindDeployment(c.Param("deployment_id"))
	if deployment == nil {
		return hosting.Ok(map[string]string{})
	}
	if ok {
		a2RemoveKeys(deployment.Config, keys)
	}
	a2Deployments().Update(deployment)
	return hosting.Ok(deployment.Config)
}

// GetDeploymentServerEvents handles GET /v1/deployments/{deployment_id}/server_events.
func (s *A2StationDbServer) GetDeploymentServerEvents(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "500"))
	page := a2ParseInt(a2QueryOr(c, "page", "1"))
	eventType := a2Query(c, "event_type")

	events := []*models.ServerEventDbObject{}
	if col := a2ServerEvents(); col != nil {
		for _, e := range col.FindAll() {
			if a2EqS(e.DeploymentId, deploymentId) {
				events = append(events, e)
			}
		}
	}
	events = a2FilterEventType(events, eventType)
	a2EventsDescending(events)
	mapped := a2MapServerEvents(events)
	return hosting.Ok(models.Create[*models.ServerEventResponse, models.ServerEventResponsePage](mapped, pageSize, page))
}

// a2DecodeUTF8NoBOM mirrors Encoding.UTF8.GetString(bytes) (no BOM stripping;
// a BOM decodes to U+FEFF).
func a2DecodeUTF8NoBOM(b []byte) string { return strings.ToValidUTF8(string(b), "\xef\xbf\xbd") }
