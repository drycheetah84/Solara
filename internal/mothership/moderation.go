// Ports the moderation endpoints of AUnrealFeatures.AAMothership/MothershipServer.cs:
// ClientCreateReport, ServerCreateReport, ServerCreateBan, ServerBulkBans,
// ListModeration, GetModerationAutomation, PostModerationAutomation and
// DeleteModerationAutomation.
package mothership

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

func bans() *docdb.Collection[models.MothershipBanDbObject] {
	return col(models.NewMothershipBanDbObject)
}

func reports() *docdb.Collection[models.MothershipReportDbObject] {
	return col(models.NewMothershipReportDbObject)
}

func mutes() *docdb.Collection[models.MothershipMuteDbObject] {
	return col(models.NewMothershipMuteDbObject)
}

// ClientCreateReport submits a moderation report from a client (valid JWT).
func (s *MothershipServer) ClientCreateReport(c *hosting.Context) hosting.Result {
	if !isClient(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	report := models.NewMothershipReportDbObject()
	report.ReportId = strp(newId())
	report.ReportedUserId = strp(str(body, "reported_user_id"))
	report.ReporterUserId = strp("")
	report.Category = intOr(body, "category", 0)
	report.Reason = strp("")
	report.Platform = strp(str(body, "platform"))
	report.ModdedClient = boolOr(body, "modded_client", false)
	report.Metadata = strp(str(body, "metadata"))
	report.CreatedAt = dotnet.UtcNow()
	reports().Insert(report)
	return jsonT(report, 200)
}

// ServerCreateReport submits a moderation report from a server.
func (s *MothershipServer) ServerCreateReport(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	report := models.NewMothershipReportDbObject()
	report.ReportId = strp(newId())
	report.ReportedUserId = strp(str(body, "reported_user_id"))
	report.ReporterUserId = strp(str(body, "reporter_user_id"))
	report.Category = intOr(body, "category", 0)
	report.Reason = strp(str(body, "reason"))
	report.Platform = strp("")
	report.ModdedClient = false
	report.Metadata = strp(str(body, "metadata"))
	report.CreatedAt = dotnet.UtcNow()
	reports().Insert(report)
	return jsonT(report, 200)
}

// ServerCreateBan bans a player for a specified duration.
func (s *MothershipServer) ServerCreateBan(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	minutes := intOr(body, "duration_minutes", 0)
	ban := models.NewMothershipBanDbObject()
	ban.BanId = strp(newId())
	ban.TitleId = strp(str(body, "title_id"))
	ban.EnvId = strp(str(body, "env_id"))
	ban.PlayerId = strp(str(body, "player_id"))
	ban.Category = intOr(body, "category", 0)
	ban.Reason = strp(str(body, "reason"))
	ban.DurationMinutes = minutes
	ban.OrgWide = boolOr(body, "org_wide", false)
	ban.Metadata = strp(str(body, "metadata"))
	ban.CreatedAt = dotnet.UtcNow()
	ban.ExpiresAt = dotnet.UtcNow().AddMinutes(float64(minutes))
	bans().Insert(ban)
	return jsonAnon(stj.O("Results", []*models.MothershipBanDbObject{ban}), 200)
}

// ServerBulkBans bans multiple players in a single request.
func (s *MothershipServer) ServerBulkBans(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	result := []*models.MothershipBanDbObject{}
	cl := bans()
	if playersEl, ok := body.TryGetValue("players"); ok && playersEl.ValueKind() == stj.Array {
		for _, p := range playersEl.EnumerateArray() {
			minutes := 0
			if dm, ok := p.TryGetProperty("duration_minutes"); ok {
				minutes = int(dm.GetInt32())
			}
			ban := models.NewMothershipBanDbObject()
			ban.BanId = strp(newId())
			ban.PlayerId = strp(strProp(p, "player_id"))
			if cat, ok := p.TryGetProperty("category"); ok {
				ban.Category = int(cat.GetInt32())
			}
			ban.Reason = strp(strProp(p, "reason"))
			ban.DurationMinutes = minutes
			ban.CreatedAt = dotnet.UtcNow()
			ban.ExpiresAt = dotnet.UtcNow().AddMinutes(float64(minutes))
			cl.Insert(ban)
			result = append(result, ban)
		}
	}
	return jsonAnon(stj.O("Results", result), 200)
}

// ListModeration lists bans and/or reports, optionally filtered by IDs.
func (s *MothershipServer) ListModeration(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	banId, hasBan := c.Request.Query("ban_id")
	playerId, hasPlayer := c.Request.Query("player_id")
	reportId, hasReport := c.Request.Query("report_id")

	if hasBan {
		ban := bans().FindOne(func(b *models.MothershipBanDbObject) bool { return dbEq(b.BanId, banId) })
		if ban != nil {
			return jsonT(ban, 200)
		}
	} else if hasReport {
		report := reports().FindOne(func(r *models.MothershipReportDbObject) bool { return dbEq(r.ReportId, reportId) })
		if report != nil {
			return jsonAnon(stj.O("Report", report), 200)
		}
	} else if hasPlayer {
		list := bans().Find(func(b *models.MothershipBanDbObject) bool { return dbEq(b.PlayerId, playerId) })
		return jsonAnon(stj.O("Results", list), 200)
	} else {
		allBans := bans().FindAll()
		allReports := reports().FindAll()
		return jsonAnon(stj.O("bans", allBans, "reports", allReports), 200)
	}
	return hosting.NotFound(nil)
}

// ServerActiveBan (Solara extension) answers "is this player banned right now?" for the game server DLL, which
// enforces bans when a player logs in to the station (the 50451 client build has the server-side check compiled
// out). GET /v1/moderation/server/active_ban?player_id=X -> {"banned":bool,"reason":..,"expires_at":..}; the ban
// that expires last wins. Admin/server auth like the other server moderation routes.
func (s *MothershipServer) ServerActiveBan(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	playerId, ok := c.Request.Query("player_id")
	if !ok || playerId == "" {
		return jsonAnon(stj.O("banned", false), 200)
	}
	now := dotnet.UtcNow()
	var active *models.MothershipBanDbObject
	for _, b := range bans().Find(func(b *models.MothershipBanDbObject) bool { return dbEq(b.PlayerId, playerId) }) {
		if b.ExpiresAt.After(now) && (active == nil || b.ExpiresAt.After(active.ExpiresAt)) {
			active = b
		}
	}
	if active == nil {
		return jsonAnon(stj.O("banned", false), 200)
	}
	return jsonAnon(stj.O("banned", true, "reason", active.Reason, "expires_at", active.ExpiresAt), 200)
}

// GetModerationAutomation returns all bans, mutes and reports.
func (s *MothershipServer) GetModerationAutomation(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	b := bans().FindAll()
	m := mutes().FindAll()
	r := reports().FindAll()
	return jsonAnon(stj.O("bans", b, "mutes", m, "reports", r), 200)
}

// PostModerationAutomation creates a mute (the only supported action).
func (s *MothershipServer) PostModerationAutomation(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	action := str(body, "action")
	if action == "mute" {
		mute := models.NewMothershipMuteDbObject()
		mute.MuteId = strp(newId())
		mute.PlayerId = strp(str(body, "player_id"))
		mute.DurationMinutes = intOr(body, "duration_minutes", 0)
		mute.CreatedAt = dotnet.UtcNow()
		mutes().Insert(mute)
		return jsonT(mute, 200)
	}
	return errorResult("Invalid action", 400)
}

// DeleteModerationAutomation removes a mute by ID.
func (s *MothershipServer) DeleteModerationAutomation(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	muteId, ok := c.Request.Query("mute_id")
	if !ok {
		return errorResult("Mute not found", 404)
	}
	cl := mutes()
	mute := cl.FindOne(func(m *models.MothershipMuteDbObject) bool { return dbEq(m.MuteId, muteId) })
	if mute == nil {
		return errorResult("Mute not found", 404)
	}
	cl.Delete(mute.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

// ServerUnban (Solara extension) lifts a player's bans: POST /v1/moderation/server/unban {"player_id":X} ends every
// still-active ban of that player now (ExpiresAt = now; the record is kept). -> {"lifted":n}
func (s *MothershipServer) ServerUnban(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	playerId := str(parseBody(c.Request), "player_id")
	if playerId == "" {
		return errorResult("player_id required", 400)
	}
	now := dotnet.UtcNow()
	cl, n := bans(), 0
	for _, b := range cl.Find(func(b *models.MothershipBanDbObject) bool { return dbEq(b.PlayerId, playerId) }) {
		if b.ExpiresAt.After(now) {
			b.ExpiresAt = now
			cl.Update(b)
			n++
		}
	}
	return jsonAnon(stj.O("lifted", n), 200)
}
