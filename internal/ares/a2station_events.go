// Ports the server-event, ban and station-event sections of
// AUnrealFeatures.Ares/Servers/A2StationDbServer.cs: station server_events
// (v0/v1), bans, station events, signups and event config.
package ares

import (
	"sort"

	"solara/internal/ares/models"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/stj"
)

// a2EventsDescending mirrors .OrderByDescending(e => e.Timestamp) (stable).
func a2EventsDescending(events []*models.ServerEventDbObject) {
	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp.After(events[j].Timestamp) })
}

// a2MapServerEvents mirrors .Select((e, i) => new ServerEventResponse { Index = i + 1, ... }).
func a2MapServerEvents(events []*models.ServerEventDbObject) []*models.ServerEventResponse {
	out := []*models.ServerEventResponse{}
	for i, e := range events {
		out = append(out, &models.ServerEventResponse{
			Index:        i + 1,
			EventType:    e.EventType,
			DeploymentId: e.DeploymentId,
			EventData:    e.EventData,
			Timestamp:    e.Timestamp,
		})
	}
	return out
}

// a2StationDeploymentIds mirrors deploymentCollection?.Find(d => d.StationId == id)
// .Select(d => d.DeploymentId).ToHashSet().
func a2StationDeploymentIds(stationId string) (ids map[string]bool, hasNull bool) {
	ids = map[string]bool{}
	for _, d := range a2DeploymentsOfStation(&stationId) {
		if d.DeploymentId == nil {
			hasNull = true
		} else {
			ids[*d.DeploymentId] = true
		}
	}
	return ids, hasNull
}

// a2StationServerEvents mirrors eventCollection?.FindAll().Where(e => depIds.Contains(e.DeploymentId)).
func a2StationServerEvents(stationId string) []*models.ServerEventDbObject {
	depIds, hasNull := a2StationDeploymentIds(stationId)
	out := []*models.ServerEventDbObject{}
	if col := a2ServerEvents(); col != nil {
		for _, e := range col.FindAll() {
			if (e.DeploymentId == nil && hasNull) || (e.DeploymentId != nil && depIds[*e.DeploymentId]) {
				out = append(out, e)
			}
		}
	}
	return out
}

// GetServerEventsV0 handles GET /stations/{station_id}/server_events (deprecated v0).
func (s *A2StationDbServer) GetServerEventsV0(c *hosting.Context) hosting.Result {
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "500"))
	events := a2StationServerEvents(c.Param("station_id"))
	a2EventsDescending(events)
	if pageSize < 0 {
		pageSize = 0
	}
	if pageSize < len(events) {
		events = events[:pageSize]
	}
	return hosting.Ok(a2MapServerEvents(events))
}

// a2FilterEventType mirrors `if (!string.IsNullOrEmpty(eventType)) query = query.Where(e => e.EventType == eventType);`.
func a2FilterEventType(events []*models.ServerEventDbObject, eventType *string) []*models.ServerEventDbObject {
	if a2IsNullOrEmpty(eventType) {
		return events
	}
	out := []*models.ServerEventDbObject{}
	for _, e := range events {
		if a2EqPtr(e.EventType, eventType) {
			out = append(out, e)
		}
	}
	return out
}

// GetServerEventsV1 handles GET /v1/stations/{station_id}/server_events.
func (s *A2StationDbServer) GetServerEventsV1(c *hosting.Context) hosting.Result {
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "500"))
	page := a2ParseInt(a2QueryOr(c, "page", "1"))
	eventType := a2Query(c, "event_type")

	events := a2FilterEventType(a2StationServerEvents(c.Param("station_id")), eventType)
	a2EventsDescending(events)
	mapped := a2MapServerEvents(events)
	return hosting.Ok(models.Create[*models.ServerEventResponse, models.ServerEventResponsePage](mapped, pageSize, page))
}

// BanStationUser handles POST /v1/stations/{station_id}/users/{user_id}/ban.
func (s *A2StationDbServer) BanStationUser(c *hosting.Context) hosting.Result {
	stationId, userId := c.Param("station_id"), c.Param("user_id")
	duration := a2ParseInt(a2QueryOr(c, "duration", "0"))
	body := a2Deserialize(c.Request.Body(), models.NewBanRequest)

	user := a2FindUser(&userId)
	if user == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	if user.Bans == nil {
		user.Bans = []*models.BanRequest{}
	}
	expiration := dotnet.MaxValue
	if duration > 0 {
		expiration = dotnet.Now().AddSeconds(float64(duration))
	}
	reason := a2Str("")
	if body != nil && body.Reason != nil {
		reason = body.Reason
	}
	user.Bans = append(user.Bans, &models.BanRequest{
		UserId:     a2Str(userId),
		StationId:  a2Str(stationId),
		Expiration: expiration,
		Reason:     reason,
		Revoked:    false,
	})
	a2Users().Update(user)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// UnbanStationUser handles PATCH /v1/stations/{station_id}/users/{user_id}/unban.
func (s *A2StationDbServer) UnbanStationUser(c *hosting.Context) hosting.Result {
	stationId, userId := c.Param("station_id"), c.Param("user_id")
	user := a2FindUser(&userId)
	if user == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	updated := false
	for _, ban := range user.Bans {
		if ban == nil {
			panic(a2NullReference())
		}
		if a2EqS(ban.StationId, stationId) && !ban.Revoked {
			ban.Revoked = true
			updated = true
		}
	}
	if updated {
		a2Users().Update(user)
	}
	return hosting.Ok(&models.SuccessBoolean{Success: updated})
}

// GetStationBans handles GET /v1/stations/{station_id}/bans.
func (s *A2StationDbServer) GetStationBans(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	includeRevoked := a2ParseBool(a2QueryOr(c, "include_revoked", "false"))
	includeExpired := a2ParseBool(a2QueryOr(c, "include_expired", "false"))
	now := dotnet.Now()

	allBans := []*models.BanRequest{}
	if col := a2Users(); col != nil {
		for _, u := range col.FindAll() {
			for _, b := range u.Bans {
				if b == nil {
					panic(a2NullReference())
				}
				if a2EqS(b.StationId, stationId) && (includeRevoked || !b.Revoked) && (includeExpired || b.Expiration.After(now)) {
					allBans = append(allBans, b)
				}
			}
		}
	}
	return hosting.Ok(&models.BanResponse{Bans: allBans})
}

// a2ToStationEvent mirrors the StationEventDbObject -> StationEvent projection.
func a2ToStationEvent(e *models.StationEventDbObject) *models.StationEvent {
	return &models.StationEvent{
		EventId:      e.EventId,
		StationId:    e.StationId,
		DeploymentId: a2Coalesce(e.DeploymentId, a2Str("")),
		Title:        e.Title,
		Description:  e.Description,
		StartTime:    e.StartTime,
		Duration:     e.Duration,
		Public:       e.Public,
		SignupsOpen:  e.SignupsOpen,
	}
}

// CreateStationEvent handles POST /v1/stations/{station_id}/event.
func (s *A2StationDbServer) CreateStationEvent(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	title := a2QueryOr(c, "title", "")
	description := a2QueryOr(c, "description", "")
	duration := a2ParseInt(a2QueryOr(c, "duration", "0"))
	deploymentId := a2Query(c, "deployment_id")
	startTimeStr := a2QueryOr(c, "start_time", dotnet.Now().FormatO())
	isPublic := a2ParseBool(a2QueryOr(c, "public", "true"))
	signupsOpen := a2ParseBool(a2QueryOr(c, "signups_open", "true"))

	config := map[string]*string{}
	if data, ok := a2DeserializeStringDict(c.Request.Body()); ok {
		a2MergeConfig(config, data)
	}

	eventId := a2GenerateId()
	startTime := a2ParseDateTime(startTimeStr)
	ev := models.NewStationEventDbObject()
	ev.EventId = a2Str(eventId)
	ev.StationId = a2Str(stationId)
	if !a2IsNullOrEmpty(deploymentId) {
		ev.DeploymentId = deploymentId
	}
	ev.Title = a2Str(title)
	ev.Description = a2Str(description)
	ev.StartTime = startTime
	ev.Duration = duration
	ev.Public = isPublic
	ev.SignupsOpen = signupsOpen
	ev.Config = config

	if col := a2StationEvents(); col != nil {
		col.Insert(ev)
	}
	return hosting.Ok(a2ToStationEvent(ev))
}

// GetStationEvents handles GET /v1/stations/{station_id}/events.
func (s *A2StationDbServer) GetStationEvents(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	getPast := a2ParseBool(a2QueryOr(c, "get_past_events", "false"))
	now := dotnet.Now()

	events := []*models.StationEventDbObject{}
	if col := a2StationEvents(); col != nil {
		events = col.Find(func(e *models.StationEventDbObject) bool { return a2EqFoldS(e.StationId, stationId) })
	}
	out := []*models.StationEvent{}
	for _, e := range events {
		if !getPast && e.StartTime.AddSeconds(float64(e.Duration)).Before(now) {
			continue
		}
		out = append(out, a2ToStationEvent(e))
	}
	return hosting.Ok(&models.StationEvents{Events: out})
}

// GetStationEvent handles GET /v1/events/{event_id}.
func (s *A2StationDbServer) GetStationEvent(c *hosting.Context) hosting.Result {
	eventId := c.Param("event_id")
	var ev *models.StationEventDbObject
	if col := a2StationEvents(); col != nil {
		ev = col.FindOne(func(e *models.StationEventDbObject) bool { return a2EqFoldS(e.EventId, eventId) })
	}
	if ev == nil {
		return hosting.Ok(nil)
	}
	return hosting.Ok(a2ToStationEvent(ev))
}

// a2FindStationEvent mirrors FindOne(e => e.EventId == event_id && e.StationId == station_id).
func a2FindStationEvent(eventId, stationId string) *models.StationEventDbObject {
	col := a2StationEvents()
	if col == nil {
		return nil
	}
	return col.FindOne(func(e *models.StationEventDbObject) bool {
		return a2EqFoldS(e.EventId, eventId) && a2EqFoldS(e.StationId, stationId)
	})
}

// DeleteStationEvent handles DELETE /v1/stations/{station_id}/event/{event_id}.
func (s *A2StationDbServer) DeleteStationEvent(c *hosting.Context) hosting.Result {
	stationId, eventId := c.Param("station_id"), c.Param("event_id")
	ev := a2FindStationEvent(eventId, stationId)
	if ev == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	a2StationEvents().Delete(ev.Id)

	// delete signups
	if col := a2Signups(); col != nil {
		for _, su := range col.Find(func(x *models.EventSignupDbObject) bool { return a2EqFoldS(x.EventId, eventId) }) {
			col.Delete(su.Id)
		}
	}
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// UpdateStationEvent handles PATCH /v1/stations/{station_id}/event/{event_id}.
func (s *A2StationDbServer) UpdateStationEvent(c *hosting.Context) hosting.Result {
	stationId, eventId := c.Param("station_id"), c.Param("event_id")
	body := a2Deserialize(c.Request.Body(), models.NewStationEvent)
	ev := a2FindStationEvent(eventId, stationId)
	if ev == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	if body != nil {
		if !a2IsNullOrEmpty(body.Title) {
			ev.Title = body.Title
		}
		if !a2IsNullOrEmpty(body.Description) {
			ev.Description = body.Description
		}
		if body.StartTime.Ticks() != 0 { // != default (Kind ignored)
			ev.StartTime = body.StartTime
		}
		if body.Duration > 0 {
			ev.Duration = body.Duration
		}
		ev.Public = body.Public
		ev.SignupsOpen = body.SignupsOpen
		if !a2IsNullOrEmpty(body.DeploymentId) {
			ev.DeploymentId = body.DeploymentId
		}
	}
	a2StationEvents().Update(ev)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// SignupStationEvent handles POST /v1/stations/{station_id}/event/{event_id}/users/{user_id}/signup.
func (s *A2StationDbServer) SignupStationEvent(c *hosting.Context) hosting.Result {
	eventId, userId := c.Param("event_id"), c.Param("user_id")
	var data *string
	if body := c.Request.Body(); len(body) > 0 {
		if el, err := stj.Parse(body); err == nil {
			switch el.ValueKind() {
			case stj.String:
				data = el.GetString()
			case stj.Null:
				data = nil
			}
		}
	}

	var ev *models.StationEventDbObject
	if col := a2StationEvents(); col != nil {
		ev = col.FindOne(func(e *models.StationEventDbObject) bool { return a2EqFoldS(e.EventId, eventId) })
	}
	if ev == nil || !ev.SignupsOpen {
		return hosting.Ok(&models.EventSignupResponse{UserSignedUp: false, SignupId: a2Str("")})
	}

	signupId := a2GenerateId()
	signupCollection := a2Signups()

	// check if already signed up
	if signupCollection != nil {
		existing := signupCollection.FindOne(func(x *models.EventSignupDbObject) bool {
			return a2EqFoldS(x.EventId, eventId) && a2EqFoldS(x.UserId, userId)
		})
		if existing != nil {
			return hosting.Ok(&models.EventSignupResponse{UserSignedUp: true, SignupId: existing.SignupId})
		}
		su := models.NewEventSignupDbObject()
		su.SignupId = a2Str(signupId)
		su.EventId = a2Str(eventId)
		su.UserId = a2Str(userId)
		su.Timestamp = dotnet.Now()
		su.Data = data
		signupCollection.Insert(su)
	}
	return hosting.Ok(&models.EventSignupResponse{UserSignedUp: true, SignupId: a2Str(signupId)})
}

// GetStationEventSignups handles GET /v1/stations/{station_id}/event/{event_id}/signups.
func (s *A2StationDbServer) GetStationEventSignups(c *hosting.Context) hosting.Result {
	eventId := c.Param("event_id")
	signups := []*models.StationEventSignupInfo{}
	if col := a2Signups(); col != nil {
		for _, x := range col.Find(func(x *models.EventSignupDbObject) bool { return a2EqFoldS(x.EventId, eventId) }) {
			signups = append(signups, &models.StationEventSignupInfo{
				SignupId: x.SignupId, UserId: x.UserId, Timestamp: x.Timestamp, Data: x.Data,
			})
		}
	}
	return hosting.Ok(&models.StationEventSignups{Signups: signups})
}

// DeleteStationEventSignup handles DELETE /v1/stations/{station_id}/event/{event_id}/signup?signup_id=.
func (s *A2StationDbServer) DeleteStationEventSignup(c *hosting.Context) hosting.Result {
	eventId := c.Param("event_id")
	signupId := a2QueryOr(c, "signup_id", "")
	col := a2Signups()
	var signup *models.EventSignupDbObject
	if col != nil {
		signup = col.FindOne(func(x *models.EventSignupDbObject) bool {
			return a2EqFoldS(x.SignupId, signupId) && a2EqFoldS(x.EventId, eventId)
		})
	}
	if signup == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	col.Delete(signup.Id)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// GetStationEventConfig handles GET /v1/stations/{station_id}/event/{event_id}/config.
func (s *A2StationDbServer) GetStationEventConfig(c *hosting.Context) hosting.Result {
	ev := a2FindStationEvent(c.Param("event_id"), c.Param("station_id"))
	if ev == nil || ev.Config == nil {
		return hosting.Ok(map[string]string{})
	}
	return hosting.Ok(ev.Config)
}

// SetStationEventConfig handles POST /v1/stations/{station_id}/event/{event_id}/config: merge keys.
func (s *A2StationDbServer) SetStationEventConfig(c *hosting.Context) hosting.Result {
	data, ok := a2DeserializeStringDict(c.Request.Body())
	ev := a2FindStationEvent(c.Param("event_id"), c.Param("station_id"))
	if ev == nil {
		return hosting.Ok(map[string]string{})
	}
	if ok {
		a2MergeConfig(ev.Config, data)
	}
	a2StationEvents().Update(ev)
	return hosting.Ok(ev.Config)
}

// DeleteStationEventConfig handles DELETE /v1/stations/{station_id}/event/{event_id}/config: remove keys.
func (s *A2StationDbServer) DeleteStationEventConfig(c *hosting.Context) hosting.Result {
	keys, ok := a2DeserializeStringList(c.Request.Body())
	ev := a2FindStationEvent(c.Param("event_id"), c.Param("station_id"))
	if ev == nil {
		return hosting.Ok(map[string]string{})
	}
	if ok {
		a2RemoveKeys(ev.Config, keys)
	}
	a2StationEvents().Update(ev)
	return hosting.Ok(ev.Config)
}
