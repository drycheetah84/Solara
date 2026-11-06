// Ports the DEDICATED SERVER REGISTRATION section of
// AUnrealFeatures.Ares/Servers/A2StationDbServer.cs: POST /register_server,
// /update_player_count, /server_heartbeat, /v1/server/admin/purge_stations,
// /v1/server/admin/prune_sessions; plus the body-deserialization helpers the
// handlers share.
package ares

import (
	"fmt"
	"unicode/utf16"

	"solara/internal/ares/models"
	"solara/internal/dotnet"
	"solara/internal/eos"
	eosmodels "solara/internal/eos/models"
	"solara/internal/halcyonsocket"
	"solara/internal/hosting"
	"solara/internal/stj"
)

// a2Deserialize mirrors `try { body = JsonSerializer.Deserialize<T>(request.Body); } catch { body = null; }`:
// JSON null and any JsonException yield nil.
func a2Deserialize[T any](body []byte, newT func() *T) *T {
	el, err := stj.Parse(body)
	if err != nil || el.ValueKind() == stj.Null {
		return nil
	}
	v := newT()
	if err := stj.FromElement(el, v); err != nil {
		return nil
	}
	return v
}

// a2KV is one entry of a deserialized Dictionary<string, string> (in order).
type a2KV struct {
	Key   string
	Value *string
}

// a2DeserializeStringDict mirrors the guarded Deserialize<Dictionary<string, string>>:
// nil (with ok=false) on JSON null or any error. Duplicate keys: last wins.
func a2DeserializeStringDict(body []byte) ([]a2KV, bool) {
	el, err := stj.Parse(body)
	if err != nil || el.ValueKind() != stj.Object {
		return nil, false
	}
	var out []a2KV
	idx := map[string]int{}
	for _, p := range el.EnumerateObject() {
		var v *string
		switch p.Value.ValueKind() {
		case stj.String:
			v = p.Value.GetString()
		case stj.Null:
			v = nil
		default:
			return nil, false
		}
		if i, ok := idx[p.Name]; ok {
			out[i].Value = v
			continue
		}
		idx[p.Name] = len(out)
		out = append(out, a2KV{p.Name, v})
	}
	return out, true
}

// a2DeserializeStringList mirrors the guarded Deserialize<List<string>>.
func a2DeserializeStringList(body []byte) ([]*string, bool) {
	el, err := stj.Parse(body)
	if err != nil || el.ValueKind() != stj.Array {
		return nil, false
	}
	out := []*string{}
	for _, it := range el.EnumerateArray() {
		switch it.ValueKind() {
		case stj.String:
			out = append(out, it.GetString())
		case stj.Null:
			out = append(out, nil)
		default:
			return nil, false
		}
	}
	return out, true
}

// a2MergeConfig mirrors `foreach (var kv in data) cfg[kv.Key] = kv.Value;`
// (a JSON null value is kept as a null value).
func a2MergeConfig(cfg map[string]*string, data []a2KV) {
	for _, kv := range data {
		cfg[kv.Key] = kv.Value
	}
}

// a2RemoveKeys mirrors `foreach (var k in keys) cfg.Remove(k);` (Remove(null)
// throws ArgumentNullException).
func a2RemoveKeys(cfg map[string]*string, keys []*string) {
	for _, k := range keys {
		if k == nil {
			dotnet.Throw("System.ArgumentNullException", "Value cannot be null. (Parameter 'key')")
		}
		delete(cfg, *k)
	}
}

// a2Prefix mirrors s[..n] (ArgumentOutOfRangeException when too short).
func a2Prefix(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) < n {
		dotnet.Throw("System.ArgumentOutOfRangeException", "Index and length must refer to a location within the string. (Parameter 'length')")
	}
	return string(utf16.Decode(u[:n]))
}

// a2GetEosSessions mirrors GetEosSessions(): lock (Sessions) { Sessions.ToList(); }.
func a2GetEosSessions() []*eosmodels.EosSessionInfo { return eos.SessionsSnapshot() }

// ─── DEDICATED SERVER REGISTRATION ───────────────────────────────────────

// RegisterServer handles POST /register_server: a dedicated server
// self-registers on spin-up. Creates a station + online deployment + roles,
// and registers the EOS matchmaking session.
func (s *A2StationDbServer) RegisterServer(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewRegisterServerRequest)
	if body == nil {
		return hosting.Ok(&models.RegisterServerResponse{Success: false})
	}

	ip := "127.0.0.1"
	if !a2IsNullOrWhiteSpace(body.Ip) {
		ip = *body.Ip
	}
	port := "7777"
	if !a2IsNullOrWhiteSpace(body.Port) {
		port = *body.Port
	}
	maxPlayers := 10
	if body.MaxPlayers > 0 {
		maxPlayers = body.MaxPlayers
	}

	// A caller-supplied deployment id (allocator, -DashboardDeploymentId) is
	// registered under THAT id and reuses its station on a re-register.
	depCol := a2Deployments()
	var existingDep *models.DeploymentDbObject
	if !a2IsNullOrWhiteSpace(body.DeploymentId) && depCol != nil {
		for _, d := range depCol.FindAll() {
			if a2EqPtr(d.DeploymentId, body.DeploymentId) {
				existingDep = d
				break
			}
		}
	}

	var deploymentId string
	if !a2IsNullOrWhiteSpace(body.DeploymentId) {
		deploymentId = *body.DeploymentId
	} else {
		deploymentId = dotnet.NewGuidN()
	}
	var stationId string
	if existingDep != nil && existingDep.StationId != nil {
		stationId = *existingDep.StationId
	} else if st, ok := halcyonsocket.PendingStations.TryRemove(deploymentId); ok && !a2IsBlank(st) && a2FindStation(st) != nil {
		stationId = st // [Solara] spun up for an existing station from the dashboard: join it
	} else {
		stationId = a2GenerateId()
	}
	var serverName string
	if a2IsNullOrWhiteSpace(body.ServerName) {
		serverName = "Server_" + a2Prefix(stationId, 6)
	} else {
		serverName = *body.ServerName
	}

	// A name typed in the dashboard "Spin Up" modal wins over the DLL's default.
	if dashName, ok := halcyonsocket.PendingNames.TryRemove(deploymentId); ok && !a2IsBlank(dashName) {
		serverName = dashName
	} else if existingDep != nil && !a2IsNullOrWhiteSpace(existingDep.DeploymentName) {
		serverName = *existingDep.DeploymentName
	}

	stationCol0 := a2Stations()
	if stationCol0 != nil {
		var found *models.StationDbObject
		for _, st := range stationCol0.FindAll() {
			if a2EqS(st.StationId, stationId) {
				found = st
				break
			}
		}
		if found == nil {
			now := dotnet.Now()
			st := models.NewStationDbObject()
			st.StationId = a2Str(stationId)
			st.StationName = a2Str(serverName)
			st.CreatedAt = dotnet.Now()
			st.Online = true
			st.LastOnline = &now
			st.Config = map[string]*string{}
			stationCol0.Insert(st)
		} else {
			a2MarkStationOnline(&stationId) // [Solara] e.g. a station created on the dashboard (starts offline)
		}
	}

	region := "NAE"
	if !a2IsNullOrWhiteSpace(body.Region) {
		region = *body.Region
	}
	createdAt := dotnet.Now()
	if existingDep != nil {
		createdAt = existingDep.CreatedAt
	}
	depRow := models.NewDeploymentDbObject()
	depRow.DeploymentId = a2Str(deploymentId)
	depRow.StationId = a2Str(stationId)
	depRow.DeploymentName = a2Str(serverName)
	depRow.IpAddress = a2Str(ip + ":" + port)
	depRow.Region = a2Str(region)
	depRow.Version = a2Str(a2BuildVersion)
	if !a2IsNullOrWhiteSpace(body.Version) {
		depRow.Version = body.Version // Solara: the build the server reports (50451 browsers filter on it)
	}
	depRow.CreatedAt = createdAt
	depRow.Online = true
	depRow.PlayerCount = 0
	depRow.Config = map[string]*string{}
	if depCol != nil {
		if existingDep != nil {
			// [Solara 2026-11-05] FIXED preserved bug: the original handed Update a row with a NEW ObjectId, so it matched
			// no document and the existing row was never updated -- above all its ip:port. The watchdog restarts a server
			// under the SAME deployment id, and the new process can come up on a different game port (UE takes the next
			// free one), so the station list kept advertising the old port. After a restart of several servers on one box
			// their ports could swap: everyone who picked Clubhouse landed on the Freerunners process (both are Station
			// Prime), and the list showed Freerunners ~40 / Clubhouse ~2 while players believed they were in Clubhouse.
			// Update the existing row in place: address, version, online, count; keep its station, name and config.
			existingDep.IpAddress = depRow.IpAddress
			existingDep.DeploymentName = depRow.DeploymentName
			if !a2IsNullOrWhiteSpace(body.Region) || a2IsNullOrWhiteSpace(existingDep.Region) {
				existingDep.Region = depRow.Region
			}
			existingDep.Version = depRow.Version
			existingDep.Online = true
			existingDep.PlayerCount = 0
			depCol.Update(existingDep)
		} else {
			depCol.Insert(depRow)
		}
	}

	a2EnsureStationRoles(stationId)

	// One session per deployment: drop any previous session for this deployment first.
	eos.RemoveSessionsByDeployment(deploymentId)
	sess := eosmodels.NewEosSessionInfo()
	sess.Id = dotnet.NewGuid()
	sess.ServerName = serverName
	sess.IpAddress = ip
	sess.Port = port
	sess.MaxPlayers = maxPlayers
	sess.Bucket = a2BuildVersion
	if !a2IsNullOrWhiteSpace(body.Version) {
		sess.Bucket = *body.Version // Solara: advertise the build the server reports
	}
	sess.BuildId = a2BuildIdNum
	sess.ImguiPort = a2Deref(body.ImguiPort)
	sess.StationId = a2Str(stationId)
	sess.DeploymentId = a2Str(deploymentId)
	sess.PublicPlayers = []string{}
	eos.UpsertSession(sess)

	s.HTTP.Logger.Warning(fmt.Sprintf("Dedicated server registered | station=%s deployment=%s %s:%s", stationId, deploymentId, ip, port))
	return hosting.Ok(&models.RegisterServerResponse{
		Success:      true,
		StationId:    a2Str(stationId),
		DeploymentId: a2Str(deploymentId),
	})
}

// UpdatePlayerCount handles POST /update_player_count: the server's REAL
// connected-player count; reconcile the EOS session and mirror it onto the DB row.
func (s *A2StationDbServer) UpdatePlayerCount(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewUpdatePlayerCountRequest)
	if body == nil || a2IsNullOrWhiteSpace(body.DeploymentId) {
		return hosting.Ok(stj.O("success", false, "error", "deployment_id required"))
	}

	count := body.PlayerCount
	if count < 0 {
		count = 0
	}
	matched := eos.SetSessionPlayerCount(*body.DeploymentId, count)
	recordDistrictPops(*body.DeploymentId, c.Request.Body()) // Solara: optional district_populations (district_pops.go)

	depCol := a2Deployments()
	if depCol != nil {
		var dep *models.DeploymentDbObject
		for _, d := range depCol.FindAll() {
			if a2EqPtr(d.DeploymentId, body.DeploymentId) {
				dep = d
				break
			}
		}
		if dep != nil {
			now := dotnet.Now()
			dep.PlayerCount = count
			dep.LastEvent = &now
			depCol.Update(dep)
		}
	}

	return hosting.Ok(stj.O("success", true, "deployment_id", body.DeploymentId, "player_count", count, "session_matched", matched))
}

// ServerHeartbeat handles POST /server_heartbeat: the watchdog's liveness
// feed (unauthenticated on purpose, like /register_server).
func (s *A2StationDbServer) ServerHeartbeat(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewServerHeartbeatRequest)
	if body == nil || a2IsNullOrWhiteSpace(body.DeploymentId) {
		return hosting.Ok(stj.O("success", false, "error", "deployment_id required"))
	}

	if inst := halcyonsocket.Instance; inst != nil {
		inst.Watchdog.OnHeartbeat(*body.DeploymentId, body.Pid, body.Seq, body.DispatchAgeMs, body.TickAgeMs,
			body.UptimeMs, body.Players, nil)
	}

	// [Solara 2026-11-05] The heartbeat (every 5 s) carries the server's connected-player count. Apply it, so the
	// station list follows players within seconds when they move between servers; /update_player_count alone came
	// every ~3 minutes and left them listed on the old server meanwhile. Only a count the server actually sent (and
	// measured: -1 = not yet) is applied: a missing field would deserialize as 0 and wipe a real count.
	players, hasPlayers := 0, false
	if el, err := stj.Parse(c.Request.Body()); err == nil && el.ValueKind() == stj.Object {
		if _, ok := el.TryGetProperty("players"); ok && body.Players >= 0 {
			players, hasPlayers = body.Players, true
		}
	}
	if hasPlayers {
		eos.SetSessionPlayerCount(*body.DeploymentId, players)
	}

	// Keep the deployment row's freshness stamp moving too.
	depCol := a2Deployments()
	if depCol != nil {
		for _, d := range depCol.FindAll() {
			if a2EqPtr(d.DeploymentId, body.DeploymentId) {
				now := dotnet.Now()
				d.LastEvent = &now
				if hasPlayers {
					d.PlayerCount = players
				}
				depCol.Update(d)
				a2MarkStationOnline(d.StationId) // [Solara] a heartbeating server's station is online
				break
			}
		}
	}

	return hosting.Ok(stj.O("success", true))
}

// PurgeStations handles POST /v1/server/admin/purge_stations: wipe ALL
// stations + deployments + roles and the in-memory EOS sessions. Gated by the
// server master key (x-api-key).
func (s *A2StationDbServer) PurgeStations(c *hosting.Context) hosting.Result {
	if !a2MaintenanceAdmin(c, "A2StationDb PurgeStations") { // Solara: static key policy + dashboard admin key (SEC-SID)
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	st, dp, rl := 0, 0, 0
	if col := a2Stations(); col != nil {
		st = col.DeleteAll()
	}
	if col := a2Deployments(); col != nil {
		dp = col.DeleteAll()
	}
	if col := a2Roles(); col != nil {
		rl = col.DeleteAll()
	}
	es := eos.ClearSessions()
	s.HTTP.Logger.Warning(fmt.Sprintf("PURGE stations=%d deployments=%d roles=%d eos_sessions=%d", st, dp, rl, es))
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// PruneSessions handles POST /v1/server/admin/prune_sessions: drop GHOST EOS
// sessions (and mark their DB rows offline) while keeping deployments that
// heartbeated within ?window_min=N (default 5). Gated by the master key.
func (s *A2StationDbServer) PruneSessions(c *hosting.Context) hosting.Result {
	if !a2MaintenanceAdmin(c, "A2StationDb PruneSessions") { // Solara: static key policy + dashboard admin key (SEC-SID)
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	windowMin := 5
	if w, ok := a2TryParseInt(a2Str(a2QueryOr(c, "window_min", "5"))); ok && w > 0 {
		windowMin = w
	}
	cutoff := dotnet.Now().AddMinutes(float64(-windowMin))

	depCol := a2Deployments()
	deps := []*models.DeploymentDbObject{}
	if depCol != nil {
		deps = depCol.FindAll()
	}
	keep := map[string]bool{}
	for _, d := range deps {
		if d.LastEvent != nil && !d.LastEvent.Before(cutoff) {
			if d.DeploymentId == nil {
				// HashSet<string> accepts null; eos.PruneSessionsExcept takes non-null ids
				continue
			}
			keep[*d.DeploymentId] = true
		}
	}
	keepCount := len(keep)
	keepHasNull := false
	for _, d := range deps {
		if d.LastEvent != nil && !d.LastEvent.Before(cutoff) && d.DeploymentId == nil {
			keepHasNull = true
		}
	}
	if keepHasNull {
		keepCount++
	}

	dropped := eos.PruneSessionsExcept(keep)

	// Mark the now-sessionless (stale) deployment rows offline too.
	offlined := 0
	for _, d := range deps {
		inKeep := (d.DeploymentId == nil && keepHasNull) || (d.DeploymentId != nil && keep[*d.DeploymentId])
		if !inKeep && d.Online {
			d.Online = false
			d.PlayerCount = 0
			depCol.Update(d)
			offlined++
		}
	}

	s.HTTP.Logger.Warning(fmt.Sprintf("PRUNE sessions: kept %d live deployment(s), dropped %d ghost session(s), offlined %d row(s)", keepCount, dropped, offlined))
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// a2MarkStationOnline flips a station to online when one of its servers registers or heartbeats. A station made on
// the dashboard starts offline, and only a posted deployment event used to set it online, so a live "Ranked" fleet
// stayed offline in /v2/fleets and the station browser. Writes only when the flag actually changes.
func a2MarkStationOnline(stationId *string) {
	if stationId == nil || a2IsBlank(*stationId) {
		return
	}
	col := a2Stations()
	if col == nil {
		return
	}
	st := col.FindOne(func(x *models.StationDbObject) bool { return a2EqFoldPtr(x.StationId, stationId) })
	if st == nil || st.Online {
		return
	}
	now := dotnet.Now()
	st.Online = true
	st.LastOnline = &now
	col.Update(st)
}
