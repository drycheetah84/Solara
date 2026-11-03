// Ports AUnrealFeatures.Ares/Servers/DashboardServer.cs (continued): stations,
// deployments, events, spin-up, moderation (bans, roles, role assignment),
// EOS sessions, telemetry, WebSocket client monitor and station members.
package ares

import (
	"fmt"
	"strconv"
	"strings"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/eos"
	eosmodels "solara/internal/eos/models"
	"solara/internal/halcyonsocket"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/stj"
)

func dashActionError(msg string) *DashboardActionResult {
	return &DashboardActionResult{Success: false, Error: dashStrPtr(msg)}
}

func (s *AresDashboardServer) GetStations(c *hosting.Context) hosting.Result {
	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	deployments := docdb.GetCollection(Database, true, models.NewDeploymentDbObject)

	list := []*DashboardStation{}
	if stations != nil {
		scope := dashRequestScope(c.Request) // a scoped token sees only its stations
		for _, st := range stations.FindAll() {
			if scope != nil && (st.StationId == nil || !scope.AllowsStation(*st.StationId)) {
				continue
			}
			deps := []*models.DeploymentDbObject{}
			if deployments != nil {
				deps = deployments.Find(func(d *models.DeploymentDbObject) bool { return dashLiteEqPP(d.StationId, st.StationId) })
			}
			ds := NewDashboardStation()
			ds.StationId = st.StationId
			ds.StationName = st.StationName
			ds.CreatedAt = st.CreatedAt
			ds.Online = st.Online
			ds.LastOnline = st.LastOnline
			// Solara: the station row's LastOnline only moves on register / a posted event, while a live
			// server's heartbeats (every 5 s) move its deployment's LastEvent. Show the newest of the two
			// so "Last Seen" reflects the running server instead of the time it registered. Display only:
			// the stored station row and the game-facing station API are unchanged.
			for _, d := range deps {
				if d.LastEvent != nil && (ds.LastOnline == nil || d.LastEvent.After(*ds.LastOnline)) {
					le := *d.LastEvent
					ds.LastOnline = &le
				}
			}
			ds.Deployments = len(deps)
			for _, d := range deps {
				if d.Online {
					ds.OnlineDeps++
				}
			}
			for _, d := range deps {
				if d.Online {
					ds.PlayerCount += d.PlayerCount
				}
			}
			list = append(list, ds)
		}
	}
	return hosting.Ok(list)
}

// DeleteStationApi: DELETE /api/stations/{station_id}?force=true
//
// A "fleet" in the dashboard IS a station. A fleet with players on it right
// now is refused unless force=true; the count is reported so the UI can ask.
func (s *AresDashboardServer) DeleteStationApi(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stations != nil {
		station = stations.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.Ok(dashActionError("station not found"))
	}

	deploymentCol := docdb.GetCollection(Database, true, models.NewDeploymentDbObject)
	deps := []*models.DeploymentDbObject{}
	if deploymentCol != nil {
		deps = deploymentCol.Find(func(d *models.DeploymentDbObject) bool { return dashLiteEq(d.StationId, stationId) })
	}

	force := strings.EqualFold(c.Request.GetQueryParameter("force", ""), "true")
	livePlayers := 0
	for _, d := range deps {
		if d.Online {
			livePlayers += d.PlayerCount
		}
	}
	if livePlayers > 0 && !force {
		return hosting.Ok(dashActionError(fmt.Sprintf("fleet has %d player(s) online — confirm again to delete anyway", livePlayers)))
	}

	stations.Delete(station.Id)

	// Cascade. The EOS sessions matter as much as the rows: leaving them behind
	// keeps a deleted fleet advertised in the station browser.
	for _, dep := range deps {
		// ...and the servers themselves: a fleet's game servers must go with it.
		stop := "allocator not running"
		if inst := halcyonsocket.Instance; inst != nil {
			stop = inst.Watchdog.Stop(dashDeref(dep.DeploymentId), "station deleted from the dashboard")
		}
		logging.Console(fmt.Sprintf("[Deployments] station %s deleted, %s: %s", stationId, dashDeref(dep.DeploymentId), stop))
		eos.RemoveSessionsByDeployment(dashDeref(dep.DeploymentId))
		deploymentCol.Delete(dep.Id)
	}

	roleCol := docdb.GetCollection(Database, true, models.NewRoleResponse)
	if roleCol != nil {
		for _, role := range roleCol.Find(func(r *models.RoleResponse) bool { return dashLiteEq(r.StationId, stationId) }) {
			roleCol.Delete(role.Id)
		}
	}

	eventCol := docdb.GetCollection(Database, true, models.NewStationEventDbObject)
	if eventCol != nil {
		for _, ev := range eventCol.Find(func(e *models.StationEventDbObject) bool { return dashLiteEq(e.StationId, stationId) }) {
			eventCol.Delete(ev.Id)
		}
	}

	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(stationId)})
}

func dashToDeployment(d *models.DeploymentDbObject) *DashboardDeployment {
	dd := NewDashboardDeployment()
	dd.DeploymentId = d.DeploymentId
	dd.StationId = d.StationId
	dd.DeploymentName = d.DeploymentName
	dd.IpAddress = d.IpAddress
	dd.Region = d.Region
	dd.Version = d.Version
	dd.Online = d.Online
	dd.PlayerCount = d.PlayerCount
	dd.LastEvent = d.LastEvent
	dd.CreatedAt = d.CreatedAt
	return dd
}

func (s *AresDashboardServer) GetDeployments(c *hosting.Context) hosting.Result {
	deployments := docdb.GetCollection(Database, true, models.NewDeploymentDbObject)
	list := []*DashboardDeployment{}
	if deployments != nil {
		all := deployments.FindAll()
		if scope := dashRequestScope(c.Request); scope != nil { // a scoped token sees only its stations' deployments
			kept := all[:0]
			for _, d := range all {
				if d.StationId != nil && scope.AllowsStation(*d.StationId) {
					kept = append(kept, d)
				}
			}
			all = kept
		}
		lastEvent := func(d *models.DeploymentDbObject) dotnet.DateTime {
			if d.LastEvent == nil {
				return dotnet.MinValue
			}
			return *d.LastEvent
		}
		dashStableSort(all, func(a, b *models.DeploymentDbObject) bool { return lastEvent(a).After(lastEvent(b)) })
		if len(all) > 100 {
			all = all[:100]
		}
		for _, d := range all {
			list = append(list, dashToDeployment(d))
		}
	}
	return hosting.Ok(list)
}

func (s *AresDashboardServer) GetEvents(c *hosting.Context) hosting.Result {
	events := docdb.GetCollection(Database, true, models.NewServerEventDbObject)
	list := []*DashboardEvent{}
	if events != nil {
		all := events.FindAll()
		dashStableSort(all, func(a, b *models.ServerEventDbObject) bool { return a.Timestamp.After(b.Timestamp) })
		if len(all) > 100 {
			all = all[:100]
		}
		for _, e := range all {
			de := NewDashboardEvent()
			de.EventId = e.Id.String()
			de.DeploymentId = e.DeploymentId
			de.EventName = e.EventType
			de.CreatedAt = e.Timestamp
			list = append(list, de)
		}
	}
	return hosting.Ok(list)
}

// ── Station / Deployment / Event creation ────────────────────────────────

func (s *AresDashboardServer) CreateStationApi(c *hosting.Context) hosting.Result {
	body := dashDeserialize(c.Request.Body(), NewDashboardCreateStation)
	if body == nil || dashIsNullOrWhiteSpaceP(body.StationName) {
		return hosting.Ok(dashActionError("station_name required"))
	}

	stationId := dotnet.NewGuidN()
	if col := docdb.GetCollection(Database, true, models.NewStationDbObject); col != nil {
		st := models.NewStationDbObject()
		st.StationId = dashStrPtr(stationId)
		st.StationName = body.StationName
		st.CreatedAt = dotnet.UtcNow()
		st.Online = false
		st.Config = map[string]*string{}
		col.Insert(st)
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(stationId)})
}

func (s *AresDashboardServer) CreateDeploymentApi(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := dashDeserialize(c.Request.Body(), NewDashboardCreateDeployment)

	stationCol := docdb.GetCollection(Database, true, models.NewStationDbObject)
	if stationCol == nil || stationCol.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) }) == nil {
		return hosting.Ok(dashActionError("station not found"))
	}

	deploymentId := dotnet.NewGuidN()
	apiKey := dashboardGenerateApiKey(deploymentId, "service")

	if col := docdb.GetCollection(Database, true, models.NewDeploymentDbObject); col != nil {
		d := models.NewDeploymentDbObject()
		d.DeploymentId = dashStrPtr(deploymentId)
		d.StationId = dashStrPtr(stationId)
		if body != nil {
			d.DeploymentName = body.DeploymentName
			d.IpAddress = body.IpAddress
			d.Region = body.Region
			d.Version = body.Version
		} else {
			d.DeploymentName, d.IpAddress, d.Region, d.Version = nil, nil, nil, nil
		}
		d.CreatedAt = dotnet.UtcNow()
		d.Online = false
		d.PlayerCount = 0
		d.Config = map[string]*string{}
		d.ApiKeys = []string{apiKey}
		col.Insert(d)
	}

	return hosting.Ok(&DashboardLaunchResult{Success: true, Id: dashStrPtr(deploymentId), ApiKey: dashStrPtr(apiKey)})
}

// SpinUpDeployment asks HalcyonSocket to have an allocator agent LAUNCH a game
// server on a box; it then self-registers. Body: { box?, map?, region?, args?, name? }.
func (s *AresDashboardServer) SpinUpDeployment(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := dashDeserialize(c.Request.Body(), halcyonsocket.NewSpinUpBody)

	stationCol := docdb.GetCollection(Database, true, models.NewStationDbObject)
	if stationCol == nil || stationCol.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) }) == nil {
		return hosting.Ok(dashActionError("station not found"))
	}

	inst := halcyonsocket.Instance
	if inst == nil {
		return hosting.Ok(dashActionError("HalcyonSocket not running"))
	}

	var box, mp, region, args, name *string
	if body != nil {
		box, mp, region, args, name = body.Box, body.Map, body.Region, body.Args, body.Name
	}
	// Solara (SEC-SID): the minted server token is also bound to this station.
	result := inst.RequestSpinUpForStation(stationId, box, mp, region, args, name)
	return hosting.Ok(result) // { success, request_id, agent, error }
}

// GetAgents: connected allocator agents (game boxes) + their free capacity.
func (s *AresDashboardServer) GetAgents(c *hosting.Context) hosting.Result {
	agents := []*halcyonsocket.AgentInfo{}
	if inst := halcyonsocket.Instance; inst != nil {
		agents = inst.GetAgents()
	}
	return hosting.Ok(stj.O("agents", agents))
}

func (s *AresDashboardServer) CreateEventApi(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := dashDeserialize(c.Request.Body(), NewDashboardCreateEvent)
	if body == nil || dashIsNullOrWhiteSpaceP(body.Title) {
		return hosting.Ok(dashActionError("title required"))
	}

	eventId := dotnet.NewGuidN()
	if col := docdb.GetCollection(Database, true, models.NewStationEventDbObject); col != nil {
		e := models.NewStationEventDbObject()
		e.EventId = dashStrPtr(eventId)
		e.StationId = dashStrPtr(stationId)
		if dashIsNullOrWhiteSpaceP(body.DeploymentId) {
			e.DeploymentId = nil
		} else {
			e.DeploymentId = body.DeploymentId
		}
		e.Title = body.Title
		e.Description = dashStrPtr(dashDeref(body.Description))
		if body.StartTime.Ticks() == 0 { // == default
			e.StartTime = dotnet.UtcNow()
		} else {
			e.StartTime = body.StartTime
		}
		e.Duration = body.Duration
		e.Public = body.Public
		e.SignupsOpen = body.SignupsOpen
		e.Config = map[string]*string{}
		col.Insert(e)
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(eventId)})
}

func (s *AresDashboardServer) GetStationEventsApi(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	col := docdb.GetCollection(Database, true, models.NewStationEventDbObject)
	events := []*DashboardStationEvent{}
	if col != nil {
		found := col.Find(func(e *models.StationEventDbObject) bool { return dashLiteEq(e.StationId, stationId) })
		dashStableSort(found, func(a, b *models.StationEventDbObject) bool { return a.StartTime.After(b.StartTime) })
		for _, e := range found {
			de := NewDashboardStationEvent()
			de.EventId = e.EventId
			de.StationId = e.StationId
			de.DeploymentId = e.DeploymentId
			de.Title = e.Title
			de.Description = e.Description
			de.StartTime = e.StartTime
			de.Duration = e.Duration
			de.Public = e.Public
			de.SignupsOpen = e.SignupsOpen
			events = append(events, de)
		}
	}
	return hosting.Ok(events)
}

func (s *AresDashboardServer) DeleteEventApi(c *hosting.Context) hosting.Result {
	stationId, eventId := c.Param("station_id"), c.Param("event_id")
	col := docdb.GetCollection(Database, true, models.NewStationEventDbObject)
	var ev *models.StationEventDbObject
	if col != nil {
		ev = col.FindOne(func(e *models.StationEventDbObject) bool {
			return dashLiteEq(e.EventId, eventId) && dashLiteEq(e.StationId, stationId)
		})
	}
	if ev == nil {
		return hosting.Ok(dashActionError("event not found"))
	}
	col.Delete(ev.Id)
	return hosting.Ok(&DashboardActionResult{Success: true})
}

func (s *AresDashboardServer) GetStationDeploymentsApi(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	deps := []*DashboardDeployment{}
	if col := docdb.GetCollection(Database, true, models.NewDeploymentDbObject); col != nil {
		found := col.Find(func(d *models.DeploymentDbObject) bool { return dashLiteEq(d.StationId, stationId) })
		dashStableSort(found, func(a, b *models.DeploymentDbObject) bool { return a.CreatedAt.After(b.CreatedAt) })
		for _, d := range found {
			deps = append(deps, dashToDeployment(d))
		}
	}
	return hosting.Ok(deps)
}

// ── Moderation: Bans ─────────────────────────────────────────────────────

func dashFindUser(users *docdb.Collection[models.UserDataResponse], userId string) *models.UserDataResponse {
	if users == nil {
		return nil
	}
	return users.FindOne(func(u *models.UserDataResponse) bool { return dashLiteEq(u.UserId, userId) })
}

func (s *AresDashboardServer) BanUser(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	body := dashDeserialize(c.Request.Body(), NewDashboardBanRequest)
	if body == nil || dashIsNullOrWhiteSpaceP(body.StationId) {
		return hosting.Ok(dashActionError("station_id required"))
	}

	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	user := dashFindUser(users, userId)
	if user == nil {
		return hosting.Ok(dashActionError("user not found"))
	}

	if user.Bans == nil {
		user.Bans = []*models.BanRequest{}
	}
	ban := models.NewBanRequest()
	ban.UserId = dashStrPtr(userId)
	ban.StationId = body.StationId
	ban.Reason = dashStrPtr(dashDeref(body.Reason))
	if body.Duration > 0 {
		ban.Expiration = dotnet.UtcNow().AddSeconds(float64(body.Duration))
	} else {
		ban.Expiration = dotnet.MaxValue
	}
	ban.Revoked = false
	user.Bans = append(user.Bans, ban)
	users.Update(user)
	return hosting.Ok(&DashboardActionResult{Success: true})
}

func (s *AresDashboardServer) UnbanUser(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	body := dashDeserialize(c.Request.Body(), NewDashboardBanRequest)
	if body == nil || dashIsNullOrWhiteSpaceP(body.StationId) {
		return hosting.Ok(dashActionError("station_id required"))
	}

	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	user := dashFindUser(users, userId)
	if user == nil {
		return hosting.Ok(dashActionError("user not found"))
	}

	changed := false
	for _, ban := range user.Bans {
		if dashEqP(ban.StationId, *body.StationId) && !ban.Revoked {
			ban.Revoked = true
			changed = true
		}
	}
	if changed {
		users.Update(user)
	}
	return hosting.Ok(&DashboardActionResult{Success: changed})
}

func (s *AresDashboardServer) GetStationBans(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	now := dotnet.UtcNow()
	bans := []*DashboardBan{}
	if users != nil {
		for _, u := range users.FindAll() {
			for _, b := range u.Bans {
				if !(dashEqP(b.StationId, stationId) && !b.Revoked && b.Expiration.After(now)) {
					continue
				}
				db := NewDashboardBan()
				db.UserId = u.UserId
				db.Username = u.Username
				db.StationId = b.StationId
				db.Reason = b.Reason
				if b.Expiration.Equal(dotnet.MaxValue) {
					db.Expiration = nil
				} else {
					exp := b.Expiration
					db.Expiration = &exp
				}
				db.Revoked = b.Revoked
				bans = append(bans, db)
			}
		}
	}
	return hosting.Ok(bans)
}

// ── Moderation: Roles ────────────────────────────────────────────────────

func (s *AresDashboardServer) GetStationRolesApi(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	roles := docdb.GetCollection(Database, true, models.NewRoleResponse)
	list := []*DashboardRole{}
	if roles != nil {
		for _, r := range roles.Find(func(r *models.RoleResponse) bool { return dashLiteEq(r.StationId, stationId) }) {
			dr := NewDashboardRole()
			dr.RoleId = r.RoleId
			dr.StationId = r.StationId
			dr.RoleName = r.RoleName
			dr.RoleDescription = r.RoleDescription
			if r.Permissions != nil {
				dr.Permissions = r.Permissions
			} else {
				dr.Permissions = []*string{}
			}
			list = append(list, dr)
		}
	}
	return hosting.Ok(list)
}

func (s *AresDashboardServer) CreateStationRoleApi(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := dashDeserialize(c.Request.Body(), NewDashboardCreateRole)
	if body == nil || dashIsNullOrWhiteSpaceP(body.RoleName) {
		return hosting.Ok(dashActionError("role_name required"))
	}

	roleId := dotnet.NewGuidN()
	role := models.NewRoleResponse()
	role.RoleId = dashStrPtr(roleId)
	role.StationId = dashStrPtr(stationId)
	role.RoleName = body.RoleName
	role.RoleDescription = dashStrPtr(dashDeref(body.RoleDescription))
	role.Permissions = []*string{}
	if col := docdb.GetCollection(Database, true, models.NewRoleResponse); col != nil {
		col.Insert(role)
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(roleId)})
}

func (s *AresDashboardServer) DeleteStationRoleApi(c *hosting.Context) hosting.Result {
	stationId, roleId := c.Param("station_id"), c.Param("role_id")
	roles := docdb.GetCollection(Database, true, models.NewRoleResponse)
	var role *models.RoleResponse
	if roles != nil {
		role = roles.FindOne(func(r *models.RoleResponse) bool {
			return dashLiteEq(r.RoleId, roleId) && dashLiteEq(r.StationId, stationId)
		})
	}
	if role == nil {
		return hosting.Ok(dashActionError("role not found"))
	}

	roles.Delete(role.Id)

	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	if users != nil {
		for _, u := range users.FindAll() {
			// in-memory LINQ: ordinary (case-sensitive) Contains
			if u.Roles == nil || !dashListContains(u.Roles, roleId) {
				continue
			}
			u.Roles, _ = dashListRemove(u.Roles, roleId)
			users.Update(u)
		}
	}
	return hosting.Ok(&DashboardActionResult{Success: true})
}

func (s *AresDashboardServer) UpdateRolePermissionsApi(c *hosting.Context) hosting.Result {
	stationId, roleId := c.Param("station_id"), c.Param("role_id")
	body := dashDeserialize(c.Request.Body(), NewDashboardSetPermissions)

	roles := docdb.GetCollection(Database, true, models.NewRoleResponse)
	var role *models.RoleResponse
	if roles != nil {
		role = roles.FindOne(func(r *models.RoleResponse) bool {
			return dashLiteEq(r.RoleId, roleId) && dashLiteEq(r.StationId, stationId)
		})
	}
	if role == nil {
		return hosting.Ok(dashActionError("role not found"))
	}

	if body != nil && body.Permissions != nil {
		role.Permissions = body.Permissions
	}
	roles.Update(role)
	return hosting.Ok(&DashboardActionResult{Success: true})
}

// ── Moderation: Role assignment ──────────────────────────────────────────

func (s *AresDashboardServer) AssignRoleApi(c *hosting.Context) hosting.Result {
	userId, roleId := c.Param("user_id"), c.Param("role_id")
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	user := dashFindUser(users, userId)
	if user == nil {
		return hosting.Ok(dashActionError("user not found"))
	}

	if user.Roles == nil {
		user.Roles = []*string{}
	}
	if !dashListContains(user.Roles, roleId) {
		user.Roles = append(user.Roles, dashStrPtr(roleId))
		users.Update(user)
	}
	return hosting.Ok(&DashboardActionResult{Success: true})
}

func (s *AresDashboardServer) RemoveRoleApi(c *hosting.Context) hosting.Result {
	userId, roleId := c.Param("user_id"), c.Param("role_id")
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	user := dashFindUser(users, userId)
	if user == nil {
		return hosting.Ok(dashActionError("user not found"))
	}

	removed := false
	if user.Roles != nil {
		user.Roles, removed = dashListRemove(user.Roles, roleId)
	}
	if removed {
		users.Update(user)
	}
	return hosting.Ok(&DashboardActionResult{Success: removed})
}

// ── EOS Sessions (in-memory, managed here, consumed by EosGatewayServer) ──

func (s *AresDashboardServer) GetSessionsApi(c *hosting.Context) hosting.Result {
	return hosting.Ok(dashSessionsSnapshot())
}

func (s *AresDashboardServer) AddSessionApi(c *hosting.Context) hosting.Result {
	body := dashDeserialize(c.Request.Body(), eosmodels.NewEosSessionInfo)
	if body == nil {
		return hosting.Ok(dashActionError("invalid body"))
	}

	if dashIsNullOrWhiteSpace(body.Id) {
		body.Id = dotnet.NewGuidN()[:16]
	}

	eos.UpsertSession(body)
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(body.Id)})
}

func (s *AresDashboardServer) RemoveSessionApi(c *hosting.Context) hosting.Result {
	removed := eos.RemoveSession(c.Param("session_id"))
	return hosting.Ok(&DashboardActionResult{Success: removed})
}

// ── Telemetry ─────────────────────────────────────────────────────────────

// dashTryParseInt mirrors int.TryParse(string?) (NumberStyles.Integer).
func dashTryParseInt(s string, present bool) (int, bool) {
	if !present {
		return 0, false
	}
	t := strings.Trim(s, " \t\n\v\f\r")
	n, err := strconv.ParseInt(t, 10, 32)
	if err != nil {
		return 0, false
	}
	return int(n), true
}

func (s *AresDashboardServer) GetTelemetry(c *hosting.Context) hosting.Result {
	limitStr, present := c.Request.Query("limit")
	limit, ok := dashTryParseInt(limitStr, present)
	if !ok {
		limit = 200
	}
	events := []stj.Obj{}
	for _, e := range eos.GetTelemetry(limit) {
		events = append(events, stj.O(
			"id", e.Id,
			"eventName", e.EventName,
			"payload", e.Payload,
			"timestamp", e.Timestamp.FormatO(),
		))
	}
	return hosting.Ok(events)
}

// ── WebSocket client monitor ───────────────────────────────────────────────

func (s *AresDashboardServer) GetWsClients(c *hosting.Context) hosting.Result {
	clients := []stj.Obj{}
	for _, cl := range eos.GetClients() {
		var disconnected *string
		if d := cl.Disconnected(); d != nil {
			disconnected = dashStrPtr(d.FormatO())
		}
		clients = append(clients, stj.O(
			"session_id", cl.SessionId,
			"path", cl.Path,
			"connected_at", cl.ConnectedAt.FormatO(),
			"disconnected_at", disconnected,
			"message_count", cl.MessageCount(),
		))
	}
	return hosting.Ok(clients)
}

func (s *AresDashboardServer) GetWsMessages(c *hosting.Context) hosting.Result {
	client := eos.GetClient(c.Param("session_id"))
	if client == nil {
		return hosting.NotFound(nil)
	}
	msgs := []stj.Obj{}
	for _, m := range client.GetMessages() {
		msgs = append(msgs, stj.O(
			"direction", m.Direction,
			"command", m.Command,
			"headers", m.Headers,
			"body", m.Body,
			"timestamp", m.Timestamp.FormatO(),
		))
	}
	return hosting.Ok(msgs)
}

// ── Station members (users with any role for this station) ───────────────

func (s *AresDashboardServer) GetStationMembers(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	roles := docdb.GetCollection(Database, true, models.NewRoleResponse)
	// HashSet<string> (ordinal)
	stationRoleIds := a2StrSet{} // may hold null
	if roles != nil {
		for _, r := range roles.Find(func(r *models.RoleResponse) bool { return dashLiteEq(r.StationId, stationId) }) {
			stationRoleIds.Add(r.RoleId)
		}
	}

	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	members := []*DashboardMember{}
	if users != nil {
		for _, u := range users.FindAll() {
			if u.Roles == nil {
				continue
			}
			mine := []*string{}
			for _, r := range u.Roles {
				if stationRoleIds.Contains(r) {
					mine = append(mine, r)
				}
			}
			if len(mine) == 0 {
				continue
			}
			m := NewDashboardMember()
			m.UserId = u.UserId
			m.Username = u.Username
			m.Platform = u.Platform
			m.Roles = mine
			members = append(members, m)
		}
	}
	return hosting.Ok(members)
}

// dashSessionsSnapshot mirrors `lock (EosGatewayServer.Sessions) { snapshot = Sessions.ToList(); }`.
// The original locked the List object itself while EosGatewayServer's writers
// lock a separate _sessionLock; eos.SessionsSnapshot takes the gateway lock so the copy is race-free.
func dashSessionsSnapshot() []*eosmodels.EosSessionInfo {
	return eos.SessionsSnapshot()
}
