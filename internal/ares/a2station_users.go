// Ports the USERS section of AUnrealFeatures.Ares/Servers/A2StationDbServer.cs
// (GET/POST /users, /v1/users, /users/{user_id}, /v1/users/{user_id},
// log_in_server, log_in_with_key (+ v1 alias), /users/log_in, /v2/users/log_in,
// /users/{user_id}/api_key) and GET /v1/region.
package ares

import (
	"solara/internal/ares/models"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/mothership"
	"solara/internal/serverid"
	"solara/internal/stj"
)

// a2FindUser mirrors userCollection?.FindOne(u => u.UserId == id) (collation).
func a2FindUser(id *string) *models.UserDataResponse {
	col := a2Users()
	if col == nil {
		return nil
	}
	return col.FindOne(func(u *models.UserDataResponse) bool { return a2EqFoldPtr(u.UserId, id) })
}

// a2FuzzyFilterUsers mirrors
// Process.ExtractTop(searchString, users.Select(x => x.Username)).Select(x => x.Value).ToHashSet()
// followed by users.Where(u => matched.Contains(u.Username)).
func a2FuzzyFilterUsers(searchString string, users []*models.UserDataResponse) []*models.UserDataResponse {
	names := make([]*string, len(users))
	for i, u := range users {
		names[i] = u.Username
	}
	matched := map[string]bool{}
	for _, r := range a2ExtractTop(searchString, names) {
		matched[*r.Value] = true
	}
	out := []*models.UserDataResponse{}
	for _, u := range users {
		if u.Username != nil && matched[*u.Username] {
			out = append(out, u)
		}
	}
	return out
}

// ListAllUsersV0 handles GET /users (deprecated v0): raw array filtered by last_login.
func (s *A2StationDbServer) ListAllUsersV0(c *hosting.Context) hosting.Result {
	userCollection := a2Users()
	threshold := dotnet.MinValue
	if _, ok := c.Request.Queries()["last_login"]; ok {
		threshold = a2ParseDateTime(a2QueryOr(c, "last_login", ""))
	}
	out := []*models.UserDataResponse{}
	if userCollection != nil {
		for _, x := range userCollection.FindAll() {
			if !x.LastLogin.Before(threshold) {
				out = append(out, x)
			}
		}
	}
	return hosting.Ok(out)
}

// CreateOrUpdateUser handles POST /users ([FromBody] UserRequest serverRequest).
func (s *A2StationDbServer) CreateOrUpdateUser(c *hosting.Context) hosting.Result {
	serverRequest := a2BindUserRequest(c)
	userCollection := a2Users()
	if userCollection == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	s.HTTP.Logger.Warning(stj.MarshalString(serverRequest))
	selectedUser := userCollection.FindOne(func(x *models.UserDataResponse) bool { return a2EqFoldPtr(x.UserId, serverRequest.UserId) })
	if selectedUser == nil {
		u := models.NewUserDataResponse()
		u.UserId = serverRequest.UserId
		u.Username = a2Str("User")
		u.DiscordId = serverRequest.DiscordId
		u.Platform = serverRequest.Platform
		u.CreatedAt = dotnet.Now()
		u.LastLogin = dotnet.MinValue
		u.Roles = []*string{}
		u.Bans = []*models.BanRequest{}
		userCollection.Insert(u)
		return hosting.Ok(&models.SuccessBoolean{Success: true})
	}

	selectedUser.Username = serverRequest.Username
	selectedUser.DiscordId = serverRequest.DiscordId
	selectedUser.Platform = serverRequest.Platform
	userCollection.Update(selectedUser)
	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// a2BindUserRequest mirrors BindFromBody -> BindComplexTypeFromBody for
// UserRequest: for each property (reflection order) look up
// JsonBody[DataMember name] and assign value.ToString() (a JSON null value is
// a null object there, so .ToString() throws NullReferenceException).
func a2BindUserRequest(c *hosting.Context) *models.UserRequest {
	body := c.Request.JsonBody()
	r := models.NewUserRequest()
	bind := func(name string) *string {
		v, ok := body.TryGetProperty(name)
		if !ok {
			return nil
		}
		if v.ValueKind() == stj.Null {
			panic(a2NullReference())
		}
		return a2Str(v.ToString())
	}
	r.UserId = bind("user_id")
	r.Username = bind("username")
	r.DiscordId = bind("discord_id")
	r.Platform = bind("platform")
	return r
}

// ListAllUsersV1 handles GET /v1/users: paginated, fuzzy search.
func (s *A2StationDbServer) ListAllUsersV1(c *hosting.Context) hosting.Result {
	userCollection := a2Users()
	pageSize := a2ParseInt(a2QueryOr(c, "page_size", "100"))
	page := a2ParseInt(a2QueryOr(c, "page", "1"))
	searchString := a2QueryOr(c, "search_string", "")
	lastLogin := a2QueryOr(c, "last_login", "")

	users := []*models.UserDataResponse{}
	if userCollection != nil {
		users = userCollection.FindAll()
	}

	if lastLogin != "" {
		loginThreshold := a2ParseDateTime(lastLogin)
		filtered := []*models.UserDataResponse{}
		for _, u := range users {
			if !u.LastLogin.Before(loginThreshold) {
				filtered = append(filtered, u)
			}
		}
		users = filtered
	}

	filtered := users
	if searchString != "" {
		filtered = a2FuzzyFilterUsers(searchString, users)
	}

	return hosting.Ok(models.Create[*models.UserDataResponse, models.UsersResponsePage](filtered, pageSize, page))
}

// DeleteUser handles DELETE /users/{user_id}.
func (s *A2StationDbServer) DeleteUser(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	userCollection := a2Users()
	user := a2FindUser(&userId)
	if user == nil {
		return hosting.Ok(&models.SuccessBoolean{Success: false})
	}

	userCollection.Delete(user.Id)

	// remove api keys
	keyCollection := a2Keys()
	if keyCollection != nil {
		for _, key := range keyCollection.Find(func(k *models.UserApiKeyDbObject) bool { return a2EqFoldS(k.UserId, userId) }) {
			keyCollection.Delete(key.Id)
		}
	}

	return hosting.Ok(&models.SuccessBoolean{Success: true})
}

// GetUserV0 handles GET /users/{user_id} (deprecated v0): list of permissions.
func (s *A2StationDbServer) GetUserV0(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	user := a2FindUser(&userId)
	if user == nil {
		return hosting.Ok([]*models.StationPerm{})
	}
	return hosting.Ok(a2GetPermissionsForUser(user))
}

// GetUserV1 handles GET /v1/users/{user_id}: full user data with optional
// role/permission expansion.
func (s *A2StationDbServer) GetUserV1(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	user := a2FindUser(&userId)
	if user == nil {
		return hosting.Ok(nil)
	}

	includeRoles := a2ParseBool(a2QueryOr(c, "include_roles", "false"))
	includePerms := a2ParseBool(a2QueryOr(c, "include_permissions", "false"))
	includeBans := a2ParseBool(a2QueryOr(c, "include_bans", "false"))

	var roles []*models.RoleResponse
	if includeRoles || includePerms {
		roles = a2ExpandRoles(user.Roles)
	}

	return hosting.Ok(models.FromStorage(user, roles, includeBans))
}

// UserServerLogin handles POST /users/log_in_server: server-side login.
func (s *A2StationDbServer) UserServerLogin(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewLogInServerRequest)
	if body == nil || a2IsNullOrEmpty(body.UserId) {
		return hosting.Ok(&models.LogInServerResponse{ApiKey: a2Str(""), User: nil, Permissions: []*models.UserPermission{}})
	}

	userCollection := a2Users()
	user := a2FindUser(body.UserId)
	if user == nil {
		user = models.NewUserDataResponse()
		user.UserId = body.UserId
		user.Username = body.Username
		user.Platform = body.Platform
		user.CreatedAt = dotnet.Now()
		user.LastLogin = dotnet.Now()
		user.Roles = []*string{}
		user.Bans = []*models.BanRequest{}
		if userCollection != nil {
			userCollection.Insert(user)
		}
	} else {
		user.Username = body.Username
		user.Platform = body.Platform
		user.LastLogin = dotnet.Now()
		if userCollection != nil {
			userCollection.Update(user)
		}
	}

	apiKey := a2GenerateApiKey(user.UserId, "server")
	if keyCollection := a2Keys(); keyCollection != nil {
		k := models.NewUserApiKeyDbObject()
		k.UserId, k.ApiKey = user.UserId, a2Str(apiKey)
		keyCollection.Insert(k)
	}

	return hosting.Ok(&models.LogInServerResponse{
		ApiKey:      a2Str(apiKey),
		User:        &user.UserData,
		Permissions: a2GetPermissionsForUser(user),
	})
}

// LogInWithKey handles POST /users/log_in_with_key: login using the api key
// from the x-api-key header (the server master key is accepted directly).
func (s *A2StationDbServer) LogInWithKey(c *hosting.Context) hosting.Result {
	apiKey := c.Request.GetHeaderValue("x-api-key", "")
	if apiKey == "" {
		return hosting.Ok(nil)
	}

	// Solara (SEC-SID): a per-server token (-DashboardApiKey=<token>) logs in
	// like the master key -- same response shape and "global" permissions, so
	// the game behaves identically -- but as the token's own identity.
	if serverid.LooksLikeToken(apiKey) {
		rec, ok := serverid.Validate(apiKey)
		if !ok || rec.Kind != serverid.KindServer {
			return hosting.Ok(nil)
		}
		return hosting.Ok(a2ServerLoginResponse(apiKey, "halcyon-server:"+rec.Deployment()))
	}

	// The dedicated server authenticates with the fixed launch-arg key (-DashboardApiKey=).
	// Solara: constant-time, and subject to the strict/compat static-key policy.
	if a2CtEq(apiKey, a2ServerMasterKey()) && serverid.StaticKeyAllowed("ServerMasterKey", "A2StationDb LogInWithKey") {
		return hosting.Ok(a2ServerLoginResponse(apiKey, "halcyon-server"))
	}

	keyCollection := a2Keys()
	var keyEntry *models.UserApiKeyDbObject
	if keyCollection != nil {
		keyEntry = keyCollection.FindOne(func(k *models.UserApiKeyDbObject) bool { return a2EqFoldS(k.ApiKey, apiKey) })
	}
	if keyEntry == nil {
		return hosting.Ok(nil)
	}

	user := a2FindUser(keyEntry.UserId)
	if user == nil {
		return hosting.Ok(nil)
	}

	return hosting.Ok(&models.LogInServerResponse{
		ApiKey:      a2Str(apiKey),
		User:        &user.UserData,
		Permissions: a2GetPermissionsForUser(user),
	})
}

// a2ServerLoginResponse is the master-key login response of the original
// (user "HalcyonServer", every permission on "global"), with userId as given.
func a2ServerLoginResponse(apiKey, userId string) *models.LogInServerResponse {
	srv := models.NewUserDataResponse()
	srv.UserId = a2Str(userId)
	srv.Username = a2Str("HalcyonServer")
	srv.Roles = []*string{}
	srv.Bans = []*models.BanRequest{}
	perms := []*models.UserPermission{}
	for _, p := range a2AllPermissions {
		perms = append(perms, &models.UserPermission{StationId: a2Str("global"), Permission: a2Str(p)})
	}
	return &models.LogInServerResponse{ApiKey: a2Str(apiKey), User: &srv.UserData, Permissions: perms}
}

// LogInWithKeyV1 handles POST /v1/users/log_in_with_key (alias of the v0 route).
func (s *A2StationDbServer) LogInWithKeyV1(c *hosting.Context) hosting.Result {
	// Solara: 50451 parses FLoginServerResponse (permissions keyed by fleet_id, plus bans).
	return v1LoginFrom(s.LogInWithKey(c))
}

// Solara: Mothership player records take their name from the station login (see mothership/playername.go).
func init() {
	mothership.UsernameLookup = func(externalId string) string {
		if u := a2FindUser(&externalId); u != nil && u.Username != nil {
			return *u.Username
		}
		return ""
	}
}

// a2UpsertLoginUser is the user upsert shared verbatim by the v0 and v2
// player-login handlers.
func a2UpsertLoginUser(userId *string, body *models.ClientLoginRequest) {
	if userId != nil && body != nil && body.Username != nil {
		defer mothership.SyncPlayerName(*userId, *body.Username) // Solara: real name onto the Mothership player
	}
	userCollection := a2Users()
	user := a2FindUser(userId)
	if user == nil {
		user = models.NewUserDataResponse()
		user.UserId = userId
		user.Username = body.Username
		user.Platform = body.Platform
		user.CreatedAt = dotnet.Now()
		user.LastLogin = dotnet.Now()
		user.Roles = []*string{}
		user.Bans = []*models.BanRequest{}
		if userCollection != nil {
			userCollection.Insert(user)
		}
	} else {
		user.Username = body.Username
		user.Platform = body.Platform
		user.LastLogin = dotnet.Now()
		if userCollection != nil {
			userCollection.Update(user)
		}
	}
}

// a2MintUserKey mirrors GenerateApiKey(userId) + inserting the key row.
func a2MintUserKey(userId *string) string {
	apiKey := a2GenerateApiKey(userId, "user")
	if keyCollection := a2Keys(); keyCollection != nil {
		k := models.NewUserApiKeyDbObject()
		k.UserId, k.ApiKey = userId, a2Str(apiKey)
		keyCollection.Insert(k)
	}
	return apiKey
}

// UserPlayerLogin handles POST /users/log_in: player login with nonce.
func (s *A2StationDbServer) UserPlayerLogin(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewClientLoginRequest)
	if body == nil {
		return hosting.Ok(&models.ClientLoginResponse{
			Success: false, ApiKey: a2Str(""), OrgScopedId: a2Str(""), ServerDeployments: []*models.ClientLoginDeployment{},
		})
	}

	// UserProof nonce check (skipped entirely while NonceVerify is off).
	if a2NonceVerify {
		nonceOk := a2VerifyNonceAny(body.PlatformId, body.Nonce)
		logging.Console("[A2DB][LOGIN] platformId='" + a2Deref(body.PlatformId) + "' nonceLen=" +
			a2Itoa(a2UTF16Len(a2Deref(body.Nonce))) + " nonceOk=" + dotnet.FormatBool(nonceOk) +
			" enforce=" + dotnet.FormatBool(a2NonceEnforce))
		if a2NonceEnforce && !nonceOk {
			return hosting.Ok(&models.ClientLoginResponse{
				Success: false, ApiKey: a2Str(""), OrgScopedId: a2Str(""), ServerDeployments: []*models.ClientLoginDeployment{},
			})
		}
	}

	userId := body.PlatformId
	a2UpsertLoginUser(userId, body)
	apiKey := a2MintUserKey(userId)

	deploymentCollection := a2Deployments()
	stationCollection := a2Stations()
	findStation := func(id *string) *models.StationDbObject {
		if stationCollection == nil {
			return nil
		}
		return stationCollection.FindOne(func(st *models.StationDbObject) bool { return a2EqFoldPtr(st.StationId, id) })
	}
	onlineDeployments := []*models.ClientLoginDeployment{}
	if deploymentCollection != nil {
		for _, d := range deploymentCollection.FindAll() {
			if !d.Online {
				continue
			}
			station := findStation(d.StationId)
			name := d.StationId
			if station != nil && station.StationName != nil {
				name = station.StationName
			}
			onlineDeployments = append(onlineDeployments, &models.ClientLoginDeployment{
				DeploymentId:   d.DeploymentId,
				StationId:      d.StationId,
				StationName:    name,
				DeploymentName: d.DeploymentName,
				IpAddress:      d.IpAddress,
				IsWhitelist:    false,
				IsPublic:       true,
				IsJoinable:     d.Online,
			})
		}
	}

	// merge in EOS matchmaking sessions as joinable deployments
	eosIds := map[string]bool{}
	for _, d := range onlineDeployments {
		if d.DeploymentId != nil {
			eosIds[*d.DeploymentId] = true
		}
	}
	for _, se := range a2GetEosSessions() {
		if eosIds[se.Id] {
			continue
		}
		stationId := a2Coalesce(se.StationId, a2Str(se.Bucket))
		station := findStation(stationId)
		name := a2Str(se.ServerName)
		if station != nil && station.StationName != nil {
			name = station.StationName
		}
		ip := se.IpAddress
		if se.Port != "" {
			ip = se.IpAddress + ":" + se.Port
		}
		onlineDeployments = append(onlineDeployments, &models.ClientLoginDeployment{
			DeploymentId:   a2Str(se.Id),
			StationId:      stationId,
			StationName:    name,
			DeploymentName: a2Str(se.ServerName),
			IpAddress:      a2Str(ip),
			IsWhitelist:    false,
			IsPublic:       true,
			IsJoinable:     true,
		})
	}

	// Join-gate: mark this org-scoped id active (MothershipServer.MarkDashboardSession).
	mothership.MarkDashboardSession(a2Deref(userId))

	return hosting.Ok(&models.ClientLoginResponse{
		Success:           true,
		ApiKey:            a2Str(apiKey),
		OrgScopedId:       userId,
		ServerDeployments: onlineDeployments,
	})
}

// UserPlayerLoginV2 handles POST /v2/users/log_in (A2 build 29932): same
// upsert + key mint + nonce handling, slim response.
func (s *A2StationDbServer) UserPlayerLoginV2(c *hosting.Context) hosting.Result {
	body := a2Deserialize(c.Request.Body(), models.NewClientLoginRequest)
	if body == nil || a2IsNullOrEmpty(body.PlatformId) {
		r := models.NewClientLoginV2Response()
		r.Success = false
		return hosting.Ok(r)
	}

	if a2NonceVerify {
		nonceOk := a2VerifyNonceAny(body.PlatformId, body.Nonce)
		logging.Console("[A2DB][LOGIN v2] platformId='" + *body.PlatformId + "' nonceOk=" + dotnet.FormatBool(nonceOk) +
			" enforce=" + dotnet.FormatBool(a2NonceEnforce))
		if a2NonceEnforce && !nonceOk {
			r := models.NewClientLoginV2Response()
			r.Success = false
			return hosting.Ok(r)
		}
	}

	userId := body.PlatformId
	a2UpsertLoginUser(userId, body)
	apiKey := a2MintUserKey(userId)

	mothership.MarkDashboardSession(*userId) // join-gate: see /users/log_in
	return hosting.Ok(&models.ClientLoginV2Response{Success: true, ApiKey: apiKey, OrgScopedId: *userId})
}

// CreateUserApiKey handles POST /users/{user_id}/api_key.
func (s *A2StationDbServer) CreateUserApiKey(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	if a2FindUser(&userId) == nil {
		return hosting.Ok(&models.ApiKeyResponse{ApiKey: a2Str("")})
	}
	apiKey := a2MintUserKey(&userId)
	return hosting.Ok(&models.ApiKeyResponse{ApiKey: a2Str(apiKey)})
}

// GetRegion handles GET /v1/region: self-host => fixed NA/US.
func (s *A2StationDbServer) GetRegion(c *hosting.Context) hosting.Result {
	return hosting.Ok(&models.RegionResponse{ContinentCode: "NA", CountryCode: "US"})
}
