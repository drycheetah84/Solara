// Ports the STATIONS section of AUnrealFeatures.Ares/Servers/A2StationDbServer.cs:
// station CRUD, board image upload / serving, station config, station users,
// station roles and role assignment.
package ares

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"solara/internal/ares/models"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/stj"
)

// a2FindStation mirrors stationCollection?.FindOne(s => s.StationId == id) (collation).
func a2FindStation(id string) *models.StationDbObject {
	col := a2Stations()
	if col == nil {
		return nil
	}
	return col.FindOne(func(s *models.StationDbObject) bool { return a2EqFoldS(s.StationId, id) })
}

// a2DeploymentsOfStation mirrors deploymentCollection?.Find(d => d.StationId == id)
// (nil = the null IEnumerable when the collection is missing).
func a2DeploymentsOfStation(id *string) []*models.DeploymentDbObject {
	col := a2Deployments()
	if col == nil {
		return nil
	}
	return col.Find(func(d *models.DeploymentDbObject) bool { return a2EqFoldPtr(d.StationId, id) })
}

// CreateStation handles POST /stations/create.
func (s *A2StationDbServer) CreateStation(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewCreateStationRequest)
	if body == nil || a2IsNullOrEmpty(body.StationName) {
		return hosting.Ok(&models.CreateStationResponse{StationId: a2Str("")})
	}

	stationId := a2GenerateId()
	if !a2IsNullOrEmpty(body.StationId) {
		stationId = *body.StationId
	}
	stationCollection := a2Stations()
	if a2FindStation(stationId) != nil {
		return hosting.Ok(&models.CreateStationResponse{StationId: a2Str(stationId)})
	}

	if stationCollection != nil {
		st := models.NewStationDbObject()
		st.StationId = a2Str(stationId)
		st.StationName = body.StationName
		st.CreatedAt = dotnet.Now()
		st.Online = false
		st.Config = map[string]*string{}
		stationCollection.Insert(st)
	}

	a2EnsureStationRoles(stationId)
	return hosting.Ok(&models.CreateStationResponse{StationId: a2Str(stationId)})
}

// a2StationResponses mirrors the shared body of FetchAllStationsV0/V1.
func a2StationResponses(c *hosting.Context) []*models.StationResponse {
	includeConfig := a2ParseBool(a2QueryOr(c, "include_config", "false"))
	includeDeployments := a2ParseBool(a2QueryOr(c, "include_deployments", "false"))
	includeOffline := a2ParseBool(a2QueryOr(c, "include_offline_stations", "true"))
	return a2StationResponsesWith(includeConfig, includeDeployments, includeOffline)
}

func a2StationResponsesWith(includeConfig, includeDeployments, includeOffline bool) []*models.StationResponse {
	stations := []*models.StationDbObject{}
	if col := a2Stations(); col != nil {
		stations = col.FindAll()
	}
	out := []*models.StationResponse{}
	for _, st := range stations {
		if !includeOffline && !st.Online {
			continue
		}
		var deps []*models.DeploymentDbObject
		if includeDeployments {
			deps = a2DeploymentsOfStation(st.StationId)
		}
		out = append(out, a2StationToResponse(st, deps, includeDeployments, includeConfig))
	}
	return out
}

// FetchAllStationsV0 handles GET /stations (deprecated v0): array of StationResponse.
func (s *A2StationDbServer) FetchAllStationsV0(c *hosting.Context) hosting.Result {
	return hosting.Ok(a2StationResponses(c))
}

// FetchAllStationsV1 handles GET /v1/stations: paginated stations.
func (s *A2StationDbServer) FetchAllStationsV1(c *hosting.Context) hosting.Result {
	includeConfig := a2ParseBool(a2QueryOr(c, "include_config", "false"))
	includeDeployments := a2ParseBool(a2QueryOr(c, "include_deployments", "false"))
	includeOffline := a2ParseBool(a2QueryOr(c, "include_offline_stations", "true"))
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "100"))
	page := a2ParseInt(a2QueryOr(c, "page", "1"))
	responses := a2StationResponsesWith(includeConfig, includeDeployments, includeOffline)
	return hosting.Ok(models.Create[*models.StationResponse, models.StationResponsePage](responses, pageSize, page))
}

// a2OnlinePlayerTotal mirrors FindAll().Where(d => d.Online).Sum(d => d.PlayerCount)
// (checked int arithmetic: overflow throws).
func a2OnlinePlayerTotal() int {
	total := int64(0)
	if col := a2Deployments(); col != nil {
		for _, d := range col.FindAll() {
			if d.Online {
				total += int64(d.PlayerCount)
				if total > 2147483647 || total < -2147483648 {
					dotnet.Throw("System.OverflowException", "Arithmetic operation resulted in an overflow.")
				}
			}
		}
	}
	return int(total)
}

// GetStation handles GET /stations/{station_id} (also answers "player_count").
func (s *A2StationDbServer) GetStation(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	// handle player_count route in case it lands here
	if stationId == "player_count" {
		return hosting.Ok(a2OnlinePlayerTotal())
	}

	includeConfig := a2ParseBool(a2QueryOr(c, "include_config", "true"))
	includeDeployments := a2ParseBool(a2QueryOr(c, "include_deployments", "true"))

	station := a2FindStation(stationId)
	if station == nil {
		return hosting.Ok(nil)
	}
	var deps []*models.DeploymentDbObject
	if includeDeployments {
		deps = a2DeploymentsOfStation(&stationId)
	}
	return hosting.Ok(a2StationToResponse(station, deps, includeDeployments, includeConfig))
}

// GetPlayerCount handles GET /stations/player_count: sum of online player counts.
func (s *A2StationDbServer) GetPlayerCount(c *hosting.Context) hosting.Result {
	return hosting.Ok(a2OnlinePlayerTotal())
}

// UpdateStation handles PATCH /stations/{station_id}: update the station name.
func (s *A2StationDbServer) UpdateStation(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := a2Deserialize(c.Request.Body(), models.NewUpdateStationRequest)

	station := a2FindStation(stationId)
	if station == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	if body != nil && !a2IsNullOrEmpty(body.StationName) {
		station.StationName = body.StationName
	}
	a2Stations().Update(station)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// DeleteStation handles DELETE /stations/{station_id} (cascades to
// deployments, roles and station events).
func (s *A2StationDbServer) DeleteStation(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	station := a2FindStation(stationId)
	if station == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	a2Stations().Delete(station.Id)

	if col := a2Deployments(); col != nil {
		for _, dep := range col.Find(func(d *models.DeploymentDbObject) bool { return a2EqFoldS(d.StationId, stationId) }) {
			col.Delete(dep.Id)
		}
	}
	if col := a2Roles(); col != nil {
		for _, role := range col.Find(func(r *models.RoleResponse) bool { return a2EqFoldS(r.StationId, stationId) }) {
			col.Delete(role.Id)
		}
	}
	if col := a2StationEvents(); col != nil {
		for _, ev := range col.Find(func(e *models.StationEventDbObject) bool { return a2EqFoldS(e.StationId, stationId) }) {
			col.Delete(ev.Id)
		}
	}
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// --- BOARD IMAGE UPLOAD ------------------------------------------------------
//   POST /v1/board/upload?station_id=<id>&key=<BoardTextureUrl1|SignPlazaFront|...>
//   header:  x-api-key: <server master key>
//   body:    the raw image bytes (png/jpg/gif/webp)
// Writes boards/<station>_<key>_<unix>.<ext>, points Config[key] at /board/<file>.

// UploadBoardImage handles POST /v1/board/upload.
func (s *A2StationDbServer) UploadBoardImage(c *hosting.Context) hosting.Result {
	// Solara (SEC-SID): also a server token bound to this station (binding checked by a2Guard) or the dashboard admin key.
	if a2ServerToken(c) == nil && !a2MaintenanceAdmin(c, "A2StationDb UploadBoardImage") {
		return hosting.Unauthorized(&models.SuccessBoolean{Success: false})
	}

	stationId := a2QueryOr(c, "station_id", "")
	key := a2QueryOr(c, "key", "")
	if a2IsBlank(stationId) || a2IsBlank(key) {
		return hosting.BadRequest(stj.O("error", "station_id and key are required"))
	}
	if a2IndexOf(BoardConfigKeys, key) < 0 {
		return hosting.BadRequest(stj.O("error", "unknown board key", "allowed", BoardConfigKeys))
	}

	body := c.Request.Body()
	if len(body) == 0 {
		return hosting.BadRequest(stj.O("error", "empty body"))
	}
	if len(body) > a2MaxBoardBytes {
		return hosting.BadRequest(stj.O("error", "image too large", "maxBytes", a2MaxBoardBytes))
	}

	// Sniff the real format from magic bytes rather than trusting an extension.
	ext := a2SniffImageExtension(body)
	if ext == nil {
		return hosting.BadRequest(stj.O("error", "not a png/jpg/gif/webp image"))
	}

	stationCollection := a2Stations()
	station := a2FindStation(stationId)
	if station == nil {
		return hosting.NotFound(stj.O("error", "no such station", "station_id", stationId))
	}

	if err := os.MkdirAll(a2BoardUploadDir(), 0o755); err != nil {
		panic(err)
	}
	// Cache-busting suffix: boards are fetched by URL.
	safeStation := a2SanitiseForFileName(stationId)
	fileName := fmt.Sprintf("%s_%s_%d.%s", safeStation, key, time.Now().Unix(), *ext)
	fullPath := filepath.Join(a2BoardUploadDir(), fileName)

	// Drop this station+key's previous uploads so the folder cannot grow without bound.
	if err := func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = a2PanicError{p}
			}
		}()
		for _, stale := range a2GetFiles(a2BoardUploadDir(), safeStation+"_"+key+"_*") {
			if err := os.Remove(stale); err != nil {
				return err
			}
		}
		return nil
	}(); err != nil {
		s.HTTP.Logger.Warning("board upload: could not clear old files: " + a2ErrMessage(err))
	}

	if err := os.WriteFile(fullPath, body, 0o644); err != nil {
		panic(err)
	}

	baseUrl := a2BoardPublicBaseEnv()
	if a2IsBlank(baseUrl) {
		baseUrl = PublicBoardBase(c.Request.Host()) // never store a loopback URL
	}
	u := baseUrl + "/board/" + fileName

	station.Config[key] = a2Str(u)
	if stationCollection != nil {
		stationCollection.Update(station)
	}

	s.HTTP.Logger.Information(fmt.Sprintf("board upload: station=%s key=%s bytes=%d -> %s", stationId, key, len(body), u))
	return hosting.Ok(stj.O("success", true, "key", key, "url", u, "bytes", len(body)))
}

// a2IndexOf mirrors Array.IndexOf(array, value).
func a2IndexOf(arr []string, v string) int {
	for i, x := range arr {
		if x == v {
			return i
		}
	}
	return -1
}

// a2IsPathRooted mirrors Path.IsPathRooted on the host OS.
func a2IsPathRooted(p string) bool {
	if a2CaseInsensitiveFS { // Windows
		if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "\\") {
			return true
		}
		return len(p) >= 2 && p[1] == ':' && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z'))
	}
	return strings.HasPrefix(p, "/")
}

// a2GetExtension mirrors Path.GetExtension (from the last '.', "" if none or
// if it ends the name).
func a2GetExtension(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		switch p[i] {
		case '.':
			if i == len(p)-1 {
				return ""
			}
			return p[i:]
		case '/', '\\':
			return ""
		}
	}
	return ""
}

// GetBoardImage handles GET /board/{file}: serve an uploaded board image.
func (s *A2StationDbServer) GetBoardImage(c *hosting.Context) hosting.Result {
	file := c.Param("file")
	// Path-traversal guard: only ever serve a bare filename out of the upload directory.
	if a2IsBlank(file) || strings.Contains(file, "..") ||
		strings.Contains(file, "/") || strings.Contains(file, "\\") || a2IsPathRooted(file) {
		return hosting.NotFound(nil)
	}

	fullPath := filepath.Join(a2BoardUploadDir(), file)
	if st, err := os.Stat(fullPath); err != nil || st.IsDir() {
		return hosting.NotFound(nil)
	}

	var mime string
	switch strings.ToLower(a2GetExtension(file)) {
	case ".png":
		mime = "image/png"
	case ".jpg":
		mime = "image/jpeg"
	case ".gif":
		mime = "image/gif"
	case ".webp":
		mime = "image/webp"
	default:
		mime = "application/octet-stream"
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		panic(err)
	}
	return hosting.Configurable(200, mime, data)
}

// GetBoardKeys handles GET /v1/board/keys: the uploadable board slots.
func (s *A2StationDbServer) GetBoardKeys(c *hosting.Context) hosting.Result {
	return hosting.Ok(BoardConfigKeys)
}

// GetStationConfig handles GET /stations/{station_id}/config.
func (s *A2StationDbServer) GetStationConfig(c *hosting.Context) hosting.Result {
	station := a2FindStation(c.Param("station_id"))
	if station == nil || station.Config == nil {
		return hosting.Ok(map[string]string{})
	}
	return hosting.Ok(station.Config)
}

// SetStationConfig handles POST /stations/{station_id}/config: merge keys.
func (s *A2StationDbServer) SetStationConfig(c *hosting.Context) hosting.Result {
	data, ok := a2DeserializeStringDict(c.Request.Body())
	station := a2FindStation(c.Param("station_id"))
	if station == nil {
		return hosting.Ok(map[string]string{})
	}
	if ok {
		a2MergeConfig(station.Config, data)
	}
	a2Stations().Update(station)
	return hosting.Ok(station.Config)
}

// DeleteStationConfig handles DELETE /stations/{station_id}/config: remove keys.
func (s *A2StationDbServer) DeleteStationConfig(c *hosting.Context) hosting.Result {
	keys, ok := a2DeserializeStringList(c.Request.Body())
	station := a2FindStation(c.Param("station_id"))
	if station == nil {
		return hosting.Ok(map[string]string{})
	}
	if ok {
		a2RemoveKeys(station.Config, keys)
	}
	a2Stations().Update(station)
	return hosting.Ok(station.Config)
}

// GetStationOnline handles GET /stations/{station_id}/online.
func (s *A2StationDbServer) GetStationOnline(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	isOnline := false
	if col := a2Deployments(); col != nil {
		for _, d := range col.FindAll() {
			if a2EqS(d.StationId, stationId) && d.Online {
				isOnline = true
				break
			}
		}
	}
	return hosting.Ok(isOnline)
}

// a2UsersInStation mirrors the shared body of GetAllUsersInStationV0/V1.
func a2UsersInStation(stationId string) []*models.UserDataResponse {
	// HashSet<string> (ordinal; may hold null).
	stationRoleIds := a2StrSet{}
	if col := a2Roles(); col != nil {
		for _, r := range col.Find(func(r *models.RoleResponse) bool { return a2EqFoldS(r.StationId, stationId) }) {
			stationRoleIds.Add(r.RoleId)
		}
	}
	out := []*models.UserDataResponse{}
	if col := a2Users(); col != nil {
		for _, u := range col.FindAll() {
			for _, rid := range u.Roles {
				if stationRoleIds.Contains(rid) {
					out = append(out, u)
					break
				}
			}
		}
	}
	return out
}

// GetAllUsersInStationV0 handles GET /stations/{station_id}/users (deprecated v0).
func (s *A2StationDbServer) GetAllUsersInStationV0(c *hosting.Context) hosting.Result {
	return hosting.Ok(a2UsersInStation(c.Param("station_id")))
}

// GetAllUsersInStationV1 handles GET /v1/stations/{station_id}/users.
func (s *A2StationDbServer) GetAllUsersInStationV1(c *hosting.Context) hosting.Result {
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "100"))
	page := a2ParseInt(a2QueryOr(c, "page", "1"))
	searchString := a2QueryOr(c, "search_string", "")

	users := a2UsersInStation(c.Param("station_id"))
	if searchString != "" {
		users = a2FuzzyFilterUsers(searchString, users)
	}
	return hosting.Ok(models.Create[*models.UserDataResponse, models.UsersResponsePage](users, pageSize, page))
}

// a2GlobalAdmins mirrors GlobalAdmins: Admin on EVERY station regardless of grants.
var a2GlobalAdmins = map[string]bool{
	"8721004894622429": true, "28147570424869138": true, "28379411968414630": true,
	"27847584954897406": true, "27931067463212467": true,
}

// GetStationUserRoles handles GET /stations/{station_id}/users/{user_id}/roles.
func (s *A2StationDbServer) GetStationUserRoles(c *hosting.Context) hosting.Result {
	stationId, userId := c.Param("station_id"), c.Param("user_id")
	a2EnsureStationRoles(stationId)

	roleList := []*models.RoleResponse{}
	if col := a2Roles(); col != nil {
		roleList = col.FindAll()
	}

	// Global admins get the station's Admin role automatically on any station.
	if a2GlobalAdmins[userId] {
		roles := []*models.RoleResponse{}
		for _, r := range roleList {
			if a2EqS(r.StationId, stationId) && a2EqS(r.RoleName, "Admin") {
				roles = append(roles, r)
				break
			}
		}
		return hosting.Ok(&models.RolesResponse{Roles: roles})
	}

	// FindAll().FirstOrDefault: captured-variable FindOne hangs this DB.
	var user *models.UserDataResponse
	if col := a2Users(); col != nil {
		for _, u := range col.FindAll() {
			if a2EqS(u.UserId, userId) {
				user = u
				break
			}
		}
	}
	if user == nil {
		return hosting.Ok(&models.RolesResponse{Roles: []*models.RoleResponse{}})
	}

	if col := a2Roles(); col != nil {
		roleList = col.FindAll()
	}
	roles := []*models.RoleResponse{}
	for _, rid := range user.Roles {
		for _, r := range roleList {
			if a2EqPtr(r.RoleId, rid) && a2EqS(r.StationId, stationId) {
				roles = append(roles, r)
				break
			}
		}
	}
	return hosting.Ok(&models.RolesResponse{Roles: roles})
}

// a2AddRole mirrors `user.Roles ??= new(); if (!user.Roles.Contains(id)) { Add; Update; }`.
func a2AddRole(user *models.UserDataResponse, roleId *string) {
	if user.Roles == nil {
		user.Roles = []*string{}
	}
	if !a2ContainsStr(user.Roles, roleId) {
		user.Roles = append(user.Roles, roleId)
		if col := a2Users(); col != nil {
			col.Update(user)
		}
	}
}

// AddStationUserRole handles POST /stations/{station_id}/users/{user_id}/roles/{role_id}.
func (s *A2StationDbServer) AddStationUserRole(c *hosting.Context) hosting.Result {
	userId, roleId := c.Param("user_id"), c.Param("role_id")
	user := a2FindUser(&userId)
	if user == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	a2AddRole(user, &roleId)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// GrantStationUserRoleByName handles POST /stations/{station_id}/users/{user_id}/grant/{role_name}:
// assign a role by NAME (Admin / Event Manager / Helper).
func (s *A2StationDbServer) GrantStationUserRoleByName(c *hosting.Context) hosting.Result {
	stationId, userId, roleName := c.Param("station_id"), c.Param("user_id"), c.Param("role_name")
	a2EnsureStationRoles(stationId)

	// FindAll().FirstOrDefault evaluates in memory (ordinal).
	var role *models.RoleResponse
	if col := a2Roles(); col != nil {
		for _, r := range col.FindAll() {
			if a2EqS(r.StationId, stationId) && a2EqS(r.RoleName, roleName) {
				role = r
				break
			}
		}
	}
	if role == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	var user *models.UserDataResponse
	if col := a2Users(); col != nil {
		for _, u := range col.FindAll() {
			if a2EqS(u.UserId, userId) {
				user = u
				break
			}
		}
	}
	if user == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	a2AddRole(user, role.RoleId)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// DeleteStationUserRole handles DELETE /stations/{station_id}/users/{user_id}/role/{role_id}.
func (s *A2StationDbServer) DeleteStationUserRole(c *hosting.Context) hosting.Result {
	userId, roleId := c.Param("user_id"), c.Param("role_id")
	user := a2FindUser(&userId)
	if user == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	removed := false
	if user.Roles != nil {
		user.Roles, removed = a2RemoveStr(user.Roles, &roleId)
	}
	if removed {
		a2Users().Update(user)
	}
	return hosting.Ok(&models.SuccessBoolean{Success: removed})
}

// AddStationUserRoleByName handles POST /v1/stations/{station_id}/user_roles:
// add a role by username.
func (s *A2StationDbServer) AddStationUserRoleByName(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewUserRoleByNameRequest)
	if body == nil {
		return hosting.Ok(&models.UserRoleByNameResponse{Success: false, UserExists: false})
	}
	var user *models.UserDataResponse
	if col := a2Users(); col != nil {
		user = col.FindOne(func(u *models.UserDataResponse) bool { return a2EqFoldPtr(u.Username, body.Username) })
	}
	if user == nil {
		return hosting.Ok(&models.UserRoleByNameResponse{Success: false, UserExists: false})
	}
	a2AddRole(user, body.RoleId)
	return hosting.Ok(&models.UserRoleByNameResponse{Success: true, UserExists: true})
}

// GetStationRoles handles GET /stations/{station_id}/roles.
func (s *A2StationDbServer) GetStationRoles(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	a2EnsureStationRoles(stationId)
	roles := []*models.RoleResponse{}
	if col := a2Roles(); col != nil {
		roles = col.Find(func(r *models.RoleResponse) bool { return a2EqFoldS(r.StationId, stationId) })
	}
	return hosting.Ok(&models.RolesResponse{Roles: roles})
}

// a2FindStationRole mirrors FindOne(r => r.RoleId == role_id && r.StationId == station_id).
func a2FindStationRole(roleId, stationId string) *models.RoleResponse {
	col := a2Roles()
	if col == nil {
		return nil
	}
	return col.FindOne(func(r *models.RoleResponse) bool {
		return a2EqFoldS(r.RoleId, roleId) && a2EqFoldS(r.StationId, stationId)
	})
}

// CreateStationRole handles POST /stations/{station_id}/roles.
func (s *A2StationDbServer) CreateStationRole(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := a2Deserialize(c.Request.Body(), models.NewCreateRoleRequest)
	if body == nil || a2IsNullOrWhiteSpace(body.RoleName) {
		return hosting.Ok(nil)
	}

	roleId := a2GenerateId()
	role := models.NewRoleResponse()
	role.RoleId = a2Str(roleId)
	role.StationId = a2Str(stationId)
	role.RoleName = body.RoleName
	role.RoleDescription = a2Coalesce(body.RoleDescription, a2Str(""))
	role.Permissions = []*string{}
	if col := a2Roles(); col != nil {
		col.Insert(role)
	}

	return hosting.Ok(&models.RoleData{
		RoleId:          a2Str(roleId),
		StationId:       a2Str(stationId),
		RoleName:        role.RoleName,
		RoleDescription: role.RoleDescription,
	})
}

// DeleteStationRole handles DELETE /stations/{station_id}/roles/{role_id}.
func (s *A2StationDbServer) DeleteStationRole(c *hosting.Context) hosting.Result {
	stationId, roleId := c.Param("station_id"), c.Param("role_id")
	role := a2FindStationRole(roleId, stationId)
	if role == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	a2Roles().Delete(role.Id)

	// remove role from all users who have it
	if col := a2Users(); col != nil {
		for _, u := range col.FindAll() {
			if u.Roles != nil && a2ContainsStr(u.Roles, &roleId) {
				u.Roles, _ = a2RemoveStr(u.Roles, &roleId)
				col.Update(u)
			}
		}
	}
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// UpdateStationRole handles PATCH /stations/{station_id}/roles/{role_id}.
func (s *A2StationDbServer) UpdateStationRole(c *hosting.Context) hosting.Result {
	stationId, roleId := c.Param("station_id"), c.Param("role_id")
	body := a2Deserialize(c.Request.Body(), models.NewCreateRoleRequest)
	role := a2FindStationRole(roleId, stationId)
	if role == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	if body != nil {
		if !a2IsNullOrWhiteSpace(body.RoleName) {
			role.RoleName = body.RoleName
		}
		if body.RoleDescription != nil {
			role.RoleDescription = body.RoleDescription
		}
	}
	a2Roles().Update(role)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// AssignPerms handles PATCH /stations/{station_id}/roles/{role_id}/permissions.
func (s *A2StationDbServer) AssignPerms(c *hosting.Context) hosting.Result {
	stationId, roleId := c.Param("station_id"), c.Param("role_id")
	body := a2Deserialize(c.Request.Body(), models.NewAssignPermsBody)
	role := a2FindStationRole(roleId, stationId)
	if role == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}
	if body != nil && body.Permissions != nil {
		role.Permissions = body.Permissions
	}
	a2Roles().Update(role)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// GetRoleUsers handles GET /v1/stations/{station_id}/roles/{role_id}/users.
func (s *A2StationDbServer) GetRoleUsers(c *hosting.Context) hosting.Result {
	roleId := c.Param("role_id")
	users := []*models.UserDataResponse{}
	if col := a2Users(); col != nil {
		for _, u := range col.FindAll() {
			if u.Roles != nil && a2ContainsStr(u.Roles, &roleId) {
				users = append(users, u)
			}
		}
	}
	return hosting.Ok(&models.UsersResponse{Users: users})
}
