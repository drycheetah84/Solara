// Port of AUnrealFeatures.EOSSDK/EosGatewayServer.cs — endpoint methods
// (auth, SDK config, WebSocket hook, matchmaking, lobbies, user search,
// telemetry) and their private helpers.
package eos

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"solara/internal/dotnet"
	"solara/internal/eos/models"
	"solara/internal/hosting"
	"solara/internal/secrets"
	"solara/internal/stj"
)

func addEosHeaders(response *hosting.Response) {
	response.SetHeader("X-Epic-Correlation-Id", CORRELATION_ID)
	response.SetHeader("Server", SERVER_NAME)
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// ── Auth ─────────────────────────────────────────────────────────────────

// DeviceIdLogin: anonymous device-based login (Quest / Android). Body is
// form-encoded. Real Epic response is just { access_token }; we match that shape.
func (s *EosGatewayServer) DeviceIdLogin(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)

	authHeader := c.Request.GetHeaderValue("Authorization", "")
	clientId := ExtractClientId(authHeader, CLIENT_ID)
	clientIp := c.Request.Remote()

	form := c.Request.FormBody()
	deviceKey := form["deviceModel"]
	deviceName := form["displayName"]
	nonce := form["nonce"]
	if deviceKey == "" {
		deviceKey = "anonymous"
	}
	if deviceName == "" {
		deviceName = "Player"
	}
	if nonce == "" {
		nonce = dotnet.NewGuidN()
	}

	// Nonce is generated once per device by the EOS SDK and stored in secure storage —
	// it's the only truly unique per-device identifier we receive.
	uniqueKey := "device:" + nonce
	puid := ProductUserIdFor(uniqueKey)
	ouid := OrgUserIdFor(uniqueKey)
	accessToken := UserToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, ORG_ID,
		puid, ouid, nonce, deviceName, clientIp, "deviceid", "0", "other")

	userNamesSet(puid, deviceName)
	s.HTTP.Logger.Information("DeviceId login | name=" + deviceName + " device=" + deviceKey + " puid=" + puid)

	return hosting.Ok(stj.O("access_token", accessToken, "token_type", "bearer"))
}

// netAddYears mirrors DateTime.AddYears (Feb 29 clamps to Feb 28).
func netAddYears(t time.Time, years int) time.Time {
	y, m, d := t.Date()
	ny := y + years
	last := time.Date(ny, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
	if d > last {
		d = last
	}
	return time.Date(ny, m, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

func formatMs(t time.Time) string {
	return dotnet.FromTime(t.UTC(), dotnet.Utc).Format("yyyy-MM-ddTHH:mm:ss.fffZ")
}

// OAuthToken mirrors EosGatewayServer.OAuthToken.
func (s *EosGatewayServer) OAuthToken(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)

	form := c.Request.FormBody()
	grantType := form["grant_type"]
	externalAuthType := form["external_auth_type"]
	externalAuthToken, hasToken := form["external_auth_token"]
	nonce := form["nonce"]
	displayName := form["display_name"]

	authHeader, _ := c.Request.Header("Authorization")
	clientId := ExtractClientId(authHeader, CLIENT_ID)
	clientIp := c.Request.Remote()
	// client_credentials gets a 1-hour token (matching real EOS); user tokens get 10 years
	now := time.Now().UTC()
	clientExpiresAt := formatMs(now.Add(time.Hour))
	userExpiresAt := formatMs(netAddYears(now, 10))

	s.HTTP.Logger.Information("EOS OAuth | grant_type=" + grantType + " external_auth_type=" + externalAuthType + " display_name=" + displayName)

	userResponse := func(accessToken, puid, ouid, idToken string) hosting.Result {
		r := models.NewEosOAuthTokenResponse()
		r.AccessToken = accessToken
		r.ExpiresAt = userExpiresAt
		r.ExpiresIn = 315360000
		r.Nonce = strPtr(nonce)
		r.Features = Features
		r.OrganizationId = ORG_ID
		r.ProductId = PRODUCT_ID
		r.SandboxId = SANDBOX_ID
		r.DeploymentId = DEPLOYMENT_ID
		r.OrganizationUserId = strPtr(ouid)
		r.ProductUserId = strPtr(puid)
		r.ProductUserIdCreated = boolPtr(false)
		r.IdToken = strPtr(idToken)
		r.ClientId = strPtr(clientId)
		return hosting.Ok(r)
	}

	if grantType == "external_auth" && externalAuthType == "steam_encrypted_appticket" {
		var ticket *string
		if hasToken {
			ticket = &externalAuthToken
		}
		steamId := steamIdFromTicket(ticket)
		name := displayName
		if name == "" {
			name = "Player"
		}
		puid := ProductUserIdFor(steamId)
		ouid := OrgUserIdFor(steamId)
		accessToken := UserToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, ORG_ID, puid, ouid, nonce, name, clientIp,
			"steam", steamId, "steam")
		idToken := IdToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, puid, "steam", steamId, "steam")
		return userResponse(accessToken, puid, ouid, idToken)
	}

	if grantType == "external_auth" && externalAuthType == "deviceid_access_token" {
		// Derive PUID from the device JWT string itself — each device caches a unique JWT
		// (unique jti), so this is stable per device and avoids collisions from stale cached tokens.
		puid := ProductUserIdFor("devicejwt:" + externalAuthToken)
		name := displayName
		if name == "" {
			name = "Player"
		}
		if name != "" && name != "Player" {
			userNamesSet(puid, name)
		}
		ouid := OrgUserIdFor(puid)
		accessToken := UserToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, ORG_ID, puid, ouid, nonce, name, clientIp, "deviceid", "0", "other")
		idToken := IdToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, puid, "deviceid", "0", "other")
		return userResponse(accessToken, puid, ouid, idToken)
	}

	if grantType == "external_auth" && externalAuthType == "oculus_userid_nonce" {
		name := displayName
		if name == "" {
			name = "Player"
		}
		puid := ProductUserIdFor(name)
		ouid := OrgUserIdFor(name)
		accessToken := UserToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, ORG_ID, puid, ouid, nonce, name, clientIp, "oculus_app_id", "0", "other")
		idToken := IdToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, puid, "oculus_app_id", "0", "other")
		return userResponse(accessToken, puid, ouid, idToken)
	}

	if grantType == "external_auth" {
		return hosting.BadRequest(eosError("common.oauth.invalid_request",
			"Unsupported external_auth_type: "+externalAuthType, 1015))
	}

	if grantType != "" && grantType != "client_credentials" {
		return hosting.BadRequest(eosError("common.oauth.invalid_grant",
			"Unsupported grant_type: "+grantType, 1016))
	}

	clientToken := ClientToken(clientId, PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, ORG_ID)
	r := models.NewEosOAuthTokenResponse()
	r.AccessToken = clientToken
	r.ExpiresAt = clientExpiresAt
	r.ExpiresIn = 3599
	r.Features = Features
	r.OrganizationId = ORG_ID
	r.ProductId = PRODUCT_ID
	r.SandboxId = SANDBOX_ID
	r.DeploymentId = DEPLOYMENT_ID
	return hosting.Ok(r)
}

// Jwks mirrors EosGatewayServer.Jwks.
func (s *EosGatewayServer) Jwks(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	return hosting.Ok(GetJwks())
}

// TurnCredentials mirrors EosGatewayServer.TurnCredentials (HMAC-SHA1 TURN REST credentials).
func (s *EosGatewayServer) TurnCredentials(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)

	puid := ExtractPuidFromBearer(c.Request.GetHeaderValue("Authorization", ""))
	expiry := time.Now().Add(time.Hour).Unix()
	username := fmt.Sprintf("%d:%s", expiry, puid)

	m := hmac.New(sha1.New, []byte(secrets.Get("TURN_SECRET")))
	m.Write([]byte(username))
	password := base64.StdEncoding.EncodeToString(m.Sum(nil))

	r := models.NewEosTurnCredentialsResponse()
	r.Username = username
	r.Password = password
	r.Ttl = 3600000
	r.Uris = []string{
		"stun:94.72.120.104:3478",
		"turn:94.72.120.104:3478?transport=udp",
		"turn:94.72.120.104:3478?transport=tcp",
	}
	return hosting.Ok(r)
}

// RtcRoomToken mirrors EosGatewayServer.RtcRoomToken.
func (s *EosGatewayServer) RtcRoomToken(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	roomName := c.Param("roomName")
	puid := ExtractPuidFromBearer(c.Request.GetHeaderValue("Authorization", ""))
	token := MakeToken(roomName, puid)
	return hosting.Ok(stj.O("token", token, "url", SERVER_URL))
}

// ── SDK Config ───────────────────────────────────────────────────────────

// DefaultEndpoints mirrors EosGatewayServer.DefaultEndpoints.
func (s *EosGatewayServer) DefaultEndpoints(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	return hosting.Ok(defaultEndpointsPayload)
}

// CustomEndpoints mirrors EosGatewayServer.CustomEndpoints.
func (s *EosGatewayServer) CustomEndpoints(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	s.HTTP.Logger.Information("SDK config requested for product: " + c.Param("product_id"))
	return hosting.Ok(customEndpointsPayload)
}

// ── WebSocket (STOMP) — Notifications + Lobby ────────────────────────────

// HandleWebSocketAsync mirrors EosGatewayServer.HandleWebSocketAsync.
func (s *EosGatewayServer) HandleWebSocketAsync(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.EscapedPath()
	logger := s.HTTP.Logger

	if IsLobbyPath(path) {
		logger.Information("WS[5000] lobby | " + path)
		auth, _ := hosting.NewRequest(r).Header("Authorization")
		ownerPuid := ExtractPuidFromBearer(auth)
		if ownerPuid == "" || ownerPuid == dummyPuid {
			w.Header().Set("Content-Length", "0")
			w.WriteHeader(401)
			return true
		}
		ws := acceptWebSocket(w, r, nil)
		func() {
			defer finishWebSocket(ws)
			LobbyWsHelperRunAsync(ws, path, ownerPuid, func(msg string) { logger.Information(msg) })
		}()
		return true
	}

	if !IsStompPath(path) {
		return false
	}

	logger.Information("WS[5000] stomp | " + path)
	ws2 := acceptWebSocket(w, r, stompSubprotocol(r))
	func() {
		defer finishWebSocket(ws2)
		defer func() {
			if p := recover(); p != nil {
				logger.Error("STOMP error | " + hosting.ExceptionMessage(p))
			}
		}()
		if err := StompHelperRunAsync(ws2, path, func(msg string) { logger.Information(msg) }); err != nil {
			logger.Error("STOMP error | " + hosting.ExceptionMessage(err))
		}
	}()
	return true
}

// stompSubprotocol mirrors `offered?.Split(',').Select(p => p.Trim()).FirstOrDefault() ?? "v12.stomp"`.
func stompSubprotocol(r *http.Request) *string {
	offered, ok := hosting.NewRequest(r).Header("Sec-WebSocket-Protocol")
	if !ok {
		return strPtr("v12.stomp")
	}
	return strPtr(strings.TrimSpace(strings.Split(offered, ",")[0]))
}

// ── Matchmaking ──────────────────────────────────────────────────────────

// MatchmakingFilter mirrors EosGatewayServer.MatchmakingFilter.
func (s *EosGatewayServer) MatchmakingFilter(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	deploymentID := c.Param("deployment_id")

	var filterReq *models.EosFilterRequest
	if err := stj.Unmarshal(c.Request.Body(), &filterReq); err != nil {
		return hosting.BadRequest(eosError("common.bad_request", "Invalid JSON body", 1001))
	}

	criteriaCount, maxResultsLog := 0, 100
	if filterReq != nil {
		if filterReq.Criteria != nil {
			criteriaCount = len(filterReq.Criteria)
		}
		maxResultsLog = filterReq.MaxResults
	}
	s.HTTP.Logger.Information(fmt.Sprintf("Matchmaking search | deployment=%s criteria=%d maxResults=%d", deploymentID, criteriaCount, maxResultsLog))

	sessionLock.Lock()
	snapshot := append([]*models.EosSessionInfo{}, Sessions...)
	sessionLock.Unlock()

	// Whitelisted stations are only advertised to the accounts on their list.
	callerName, callerPuid := IdentifyCaller(c.Request)
	if WhitelistedStationCount() > 0 {
		before := len(snapshot)
		var visible []*models.EosSessionInfo
		for _, x := range snapshot {
			if visibleTo(x, callerName, callerPuid) {
				visible = append(visible, x)
			}
		}
		snapshot = visible
		hidden := before - len(snapshot)
		if hidden > 0 {
			who := callerName
			if who == "" {
				who = "an unidentified caller"
			}
			s.HTTP.Logger.Information(fmt.Sprintf("Whitelist | hid %d session(s) from ", hidden) + who)
		}
	}

	var criteria []*models.EosFilterCriterion
	take := 100
	if filterReq != nil {
		criteria = filterReq.Criteria
		if filterReq.MaxResults > 0 {
			take = filterReq.MaxResults
		}
	}

	lastUpdated := utcNowString()
	sessions := []*models.EosMatchmakingSession{}
	for _, x := range snapshot {
		if len(sessions) >= take {
			break
		}
		if !matchesCriteria(x, criteria) {
			continue
		}
		sessionLock.Lock()
		players := append([]string{}, x.PublicPlayers...)
		sessionLock.Unlock()
		m := models.NewEosMatchmakingSession()
		m.Deployment = DEPLOYMENT_ID
		m.Id = x.Id
		m.Bucket = x.Bucket
		m.Settings = &models.EosSessionSettings{
			MaxPublicPlayers:     x.MaxPlayers,
			AllowInvites:         false,
			ShouldAdvertise:      true,
			AllowReadById:        true,
			AllowJoinViaPresence: true,
			AllowJoinInProgress:  true,
			AllowConferenceRoom:  false,
			CheckSanctions:       false,
			AllowMigration:       false,
			RejoinAfterKick:      "",
			Platforms:            nil,
		}
		m.MaxPublicPlayers = x.MaxPlayers
		m.OpenPublicPlayers = x.MaxPlayers - len(players)
		m.MaxPrivatePlayers = 0
		m.OpenPrivatePlayers = 0
		m.PublicPlayers = players
		m.PrivatePlayers = []string{}
		m.TotalPlayers = len(players)
		m.AllowJoinInProgress = true
		m.ShouldAdvertise = true
		m.IsDedicated = true
		m.UsesStats = false
		m.AllowInvites = false
		m.UsesPresence = false
		m.AllowJoinViaPresence = true
		m.AllowJoinViaPresenceFriendsOnly = false
		m.BuildUniqueId = x.BuildId
		m.Started = true
		m.LastUpdated = lastUpdated
		m.Attributes = &models.EosSessionAttributes{
			ServerName:         x.ServerName,
			BUsesStats:         false,
			NumPrivateConns:    0,
			UsesPresence:       false,
			Address:            x.IpAddress,
			PresenceSearch:     true,
			NumPublicConns:     x.MaxPlayers,
			PublicPort:         x.Port,
			BuildUniqueId:      x.BuildId,
			AntiCheatProtected: true,
			IsDedicated:        true,
			ImguiPort:          x.ImguiPort,
		}
		m.Owner = "Client_xyza7891d295O6HPItjLVgKsuU06DkR0"
		m.OwnerPlatformId = nil
		sessions = append(sessions, m)
	}

	payload := &models.EosMatchmakingResponse{Count: len(sessions), Sessions: sessions}
	bytes := stj.MustMarshal(payload)
	return hosting.Configurable(200, "application/json", bytes)
}

// ── Lobbies ──────────────────────────────────────────────────────────────

// LobbyFilter mirrors EosGatewayServer.LobbyFilter.
func (s *EosGatewayServer) LobbyFilter(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	deploymentID := c.Param("deployment_id")

	var req *models.EosLobbyFilterRequest
	if err := stj.Unmarshal(c.Request.Body(), &req); err != nil {
		return hosting.BadRequest(eosError("common.bad_request", "Invalid JSON body", 1001))
	}

	criteriaCount := 0
	if req != nil && req.Criteria != nil {
		criteriaCount = len(req.Criteria)
	}
	s.HTTP.Logger.Information(fmt.Sprintf("Lobby filter | deployment=%s criteria=%d", deploymentID, criteriaCount))

	var bytes []byte
	withLobbyLock(func() {
		snapshot := append([]*models.EosLobbySession{}, Lobbies...)

		minPlayers, maxResults := 0, 100
		var criteria []*models.EosFilterCriterion
		if req != nil {
			minPlayers = req.MinCurrentPlayers
			if req.MaxResults > 0 {
				maxResults = req.MaxResults
			}
			criteria = req.Criteria
		}

		matched := []*models.EosLobbySession{}
		for _, l := range snapshot {
			if len(matched) >= maxResults {
				break
			}
			if l.TotalPlayers >= minPlayers && matchesLobbyCriteria(l, criteria) {
				matched = append(matched, l)
			}
		}

		payload := &models.EosLobbyResponse{Count: len(matched), Sessions: matched}
		bytes = stj.MustMarshal(payload)
	})
	return hosting.Configurable(200, "application/json", bytes)
}

// LobbyCreate mirrors EosGatewayServer.LobbyCreate.
func (s *EosGatewayServer) LobbyCreate(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)

	var req *models.EosLobbyCreateRequest
	if err := stj.Unmarshal(c.Request.Body(), &req); err != nil {
		return hosting.BadRequest(eosError("common.bad_request", "Invalid JSON body", 1001))
	}

	auth, _ := c.Request.Header("Authorization")
	ownerPuid := ExtractPuidFromBearer(auth)

	lobby := models.NewEosLobbySession()
	lobby.Deployment = DEPLOYMENT_ID
	lobby.Id = dotnet.NewGuidN()
	lobby.Bucket = "main"
	if req != nil && req.Bucket != nil {
		lobby.Bucket = *req.Bucket
	}
	lobby.Settings = models.NewEosLobbySettings()
	maxPublic := 10
	if req != nil && req.Settings != nil {
		lobby.Settings = req.Settings
		maxPublic = req.Settings.MaxPublicPlayers
	}
	lobby.TotalPlayers = 1
	lobby.OpenPublicPlayers = maxPublic - 1
	lobby.PublicPlayers = []string{ownerPuid}
	lobby.Started = false
	lobby.LastUpdated = utcNowString()
	lobby.Attributes = models.NewDict[*string]()
	if req != nil && req.Attributes != nil {
		lobby.Attributes = *req.Attributes
	}
	lobby.Owner = ownerPuid
	lobby.OwnerPlatformId = 4000

	withLobbyLock(func() { Lobbies = append(Lobbies, lobby) })
	s.HTTP.Logger.Information("Lobby created | id=" + lobby.Id + " owner=" + ownerPuid)
	PersistLobbies()

	var bytes []byte
	withLobbyLock(func() { bytes = stj.MustMarshal(lobbyInfoEnvelope(lobby)) })
	return hosting.Configurable(200, "application/json", bytes)
}

// LobbyGet mirrors EosGatewayServer.LobbyGet.
func (s *EosGatewayServer) LobbyGet(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	lobbyID := c.Param("lobby_id")

	var bytes []byte
	found := false
	withLobbyLock(func() {
		if lobby := findLobby(lobbyID); lobby != nil {
			found = true
			bytes = stj.MustMarshal(lobbyInfoEnvelope(lobby))
		}
	})

	if !found {
		return hosting.NotFound(eosError("lobbies.not_found", "Lobby "+lobbyID+" not found", 10069))
	}
	return hosting.Configurable(200, "application/json", bytes)
}

// LobbyUpdate mirrors EosGatewayServer.LobbyUpdate.
func (s *EosGatewayServer) LobbyUpdate(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	lobbyID := c.Param("lobby_id")

	var req *models.EosLobbyUpdateRequest
	if err := stj.Unmarshal(c.Request.Body(), &req); err != nil {
		return hosting.BadRequest(eosError("common.bad_request", "Invalid JSON body", 1001))
	}

	var result hosting.Result
	withLobbyLock(func() {
		lobby := findLobby(lobbyID)
		if lobby == nil {
			result = hosting.NotFound(eosError("lobbies.not_found", "Lobby "+lobbyID+" not found", 10069))
			return
		}

		if req != nil && req.Settings != nil {
			lobby.Settings = req.Settings
		}
		if req != nil && req.Attributes != nil {
			for _, k := range req.Attributes.Keys() {
				v, _ := req.Attributes.TryGetValue(k)
				lobby.Attributes.Set(k, v)
			}
		}
		if req != nil && req.Started != nil {
			lobby.Started = *req.Started
		}
		lobby.LastUpdated = utcNowString()
		lobby.OpenPublicPlayers = lobby.Settings.MaxPublicPlayers - len(lobby.PublicPlayers)

		bytes := stj.MustMarshal(lobbyInfoEnvelope(lobby))
		persistLobbiesLocked()
		result = hosting.Configurable(200, "application/json", bytes)
	})
	return result
}

// LobbyJoin mirrors EosGatewayServer.LobbyJoin.
func (s *EosGatewayServer) LobbyJoin(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	lobbyID, productUserID := c.Param("lobby_id"), c.Param("product_user_id")

	var result hosting.Result
	withLobbyLock(func() {
		lobby := findLobby(lobbyID)
		if lobby == nil {
			result = hosting.NotFound(eosError("lobbies.not_found", "Lobby "+lobbyID+" not found", 10069))
			return
		}

		if !containsString(lobby.PublicPlayers, productUserID) {
			lobby.PublicPlayers = append(lobby.PublicPlayers, productUserID)
			lobby.TotalPlayers = len(lobby.PublicPlayers)
			lobby.OpenPublicPlayers = max(0, lobby.Settings.MaxPublicPlayers-lobby.TotalPlayers)
			lobby.LastUpdated = utcNowString()
		}
	})
	if result != nil {
		return result
	}

	s.HTTP.Logger.Information("Lobby join | id=" + lobbyID + " player=" + productUserID)
	BroadcastAsync(lobbyID, stj.O(
		"name", "memberjoined",
		"payload", stj.O("lobbyId", lobbyID, "puid", productUserID, "platformId", 4000, "platform", "steam", "allowCrossplay", true, "name", "memberjoined"),
	))
	return hosting.Configurable(200, "application/json", nil)
}

// LobbyLeave mirrors EosGatewayServer.LobbyLeave.
func (s *EosGatewayServer) LobbyLeave(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	lobbyID, productUserID := c.Param("lobby_id"), c.Param("product_user_id")

	withLobbyLock(func() {
		lobby := findLobby(lobbyID)
		if lobby != nil {
			lobby.PublicPlayers = removeFirst(lobby.PublicPlayers, productUserID)
			lobby.TotalPlayers = len(lobby.PublicPlayers)
			lobby.OpenPublicPlayers = max(0, lobby.Settings.MaxPublicPlayers-lobby.TotalPlayers)
			lobby.LastUpdated = utcNowString()

			// destroy lobby if empty or owner left
			if lobby.TotalPlayers == 0 || lobby.Owner == productUserID {
				removeLobby(lobby)
			}
		}
	})

	s.HTTP.Logger.Information("Lobby leave | id=" + lobbyID + " player=" + productUserID)
	BroadcastAsync(lobbyID, stj.O(
		"name", "memberleft",
		"payload", stj.O("lobbyId", lobbyID, "puid", productUserID, "name", "memberleft"),
	))
	return hosting.Configurable(200, "application/json", nil)
}

func lobbyInfoEnvelope(lobby *models.EosLobbySession) stj.Obj {
	return stj.O(
		"publicData", lobby,
		"privateData", nil,
		"memberData", nil,
		"room", nil,
		"requestId", nil,
		"name", "lobbyinfo",
	)
}

func eosError(code, message string, numericCode int) stj.Obj {
	return stj.O(
		"errorCode", "errors.com.epicgames."+code,
		"errorMessage", message,
		"numericErrorCode", numericCode,
		"messageVarList", []string{},
		"correlationId", CORRELATION_ID,
	)
}

func eqNullable(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func matchesLobbyCriteria(lobby *models.EosLobbySession, criteria []*models.EosFilterCriterion) bool {
	if len(criteria) == 0 {
		return true
	}
	for _, c := range criteria {
		// criteria keys look like "attributes.VERSION_s" — strip the prefix
		attrKey := c.Key
		if hasPrefixFold(c.Key, "attributes.") {
			attrKey = c.Key[11:]
		}

		attrVal, ok := lobby.Attributes.TryGetValue(attrKey)
		if !ok {
			return false
		}

		match := true
		switch c.Op {
		case "EQUAL":
			match = eqNullable(attrVal, c.Value.GetString())
		case "NOT_EQUAL":
			match = !eqNullable(attrVal, c.Value.GetString())
		}
		if !match {
			return false
		}
	}
	return true
}

// ExtractPuidFromBearer mirrors EosGatewayServer.ExtractPuidFromBearer: the
// productUserId claim of a Bearer JWT, "" when the claim is absent/null, and
// the dummy PUID for a missing/non-Bearer/undecodable header.
func ExtractPuidFromBearer(authHeader string) string {
	ensureGatewayStatic()
	if authHeader == "" || !hasPrefixFold(authHeader, "Bearer ") {
		return dummyPuid
	}
	out := dummyPuid
	tryRun(func() {
		parts := strings.Split(authHeader[7:], ".")
		if len(parts) < 2 {
			return
		}
		pad := len(parts[1]) % 4
		padded := parts[1]
		if pad != 0 {
			padded += strings.Repeat("=", 4-pad)
		}
		raw, err := fromBase64String(strings.NewReplacer("-", "+", "_", "/").Replace(padded))
		if err != nil {
			panic(err)
		}
		root := stj.MustParse([]byte(utf8GetString(raw)))
		if v, ok := root.TryGetProperty("productUserId"); ok {
			out = strOrEmpty(v.GetString())
		} else {
			out = ""
		}
	})
	return out
}

// steamIdFromTicket mirrors SteamIdFromTicket: SHA-256 of the ASCII ticket,
// low 52 bits of the first 8 bytes (little-endian), "7656119" + D10.
func steamIdFromTicket(ticketHex *string) string {
	if ticketHex == nil || *ticketHex == "" {
		return "76561190000000001"
	}
	ascii := make([]byte, 0, len(*ticketHex))
	for _, r := range *ticketHex {
		if r > 0x7F {
			ascii = append(ascii, '?')
		} else {
			ascii = append(ascii, byte(r))
		}
	}
	hash := sha256.Sum256(ascii)
	num := binary.LittleEndian.Uint64(hash[:8]) & 0x000FFFFFFFFFFFFF // 52-bit
	return fmt.Sprintf("7656119%010d", num)
}

func matchesCriteria(s *models.EosSessionInfo, criteria []*models.EosFilterCriterion) bool {
	if len(criteria) == 0 {
		return true
	}
	for _, c := range criteria {
		val := resolveKey(s, c.Key)
		if !evalOp(val, c.Op, c.Value) {
			return false
		}
	}
	return true
}

func resolveKey(s *models.EosSessionInfo, key string) any {
	switch key {
	case "bucket":
		return s.Bucket
	case "attributes.SERVERNAME_s":
		return s.ServerName
	case "attributes.ADDRESS_s":
		return s.IpAddress
	case "attributes.PUBLICPORT_s":
		return s.Port
	case "attributes.NUMPUBLICCONNECTIONS_l":
		return s.MaxPlayers
	case "attributes.BUILDUNIQUEID_l":
		return s.BuildId
	case "attributes.IMGUIPORT_s":
		return s.ImguiPort
	case "attributes.BISDEDICATED_b":
		return true
	case "attributes.BANTICHEATPROTECTED_b":
		return true
	case "attributes.PRESENCESEARCH_b":
		return true
	case "attributes.BUSESSTATS_b":
		return false
	case "attributes.USESPRESENCE_b":
		return false
	case "attributes.NUMPRIVATECONNECTIONS_l":
		return 0
	}
	return nil
}

func evalOp(val any, op string, crit stj.Element) (result bool) {
	defer func() {
		if p := recover(); p != nil {
			result = false
		}
	}()
	tryInt := func() (int, bool) {
		ci, ok := crit.TryGetInt32()
		return int(ci), ok
	}
	switch op {
	case "EQUAL":
		switch v := val.(type) {
		case string:
			return eqNullable(&v, crit.GetString())
		case int:
			ci, ok := tryInt()
			return ok && v == ci
		case bool:
			return (crit.ValueKind() == stj.True && v) || (crit.ValueKind() == stj.False && !v)
		}
		return false
	case "NOT_EQUAL":
		switch v := val.(type) {
		case string:
			return !eqNullable(&v, crit.GetString())
		case int:
			ci, ok := tryInt()
			return !ok || v != ci
		}
		return false
	case "GREATER_THAN_OR_EQUAL":
		if v, isInt := val.(int); isInt {
			ci, ok := tryInt()
			return ok && v >= ci
		}
		return false
	case "GREATER_THAN":
		if v, isInt := val.(int); isInt {
			ci, ok := tryInt()
			return ok && v > ci
		}
		return false
	case "LESS_THAN_OR_EQUAL":
		if v, isInt := val.(int); isInt {
			ci, ok := tryInt()
			return ok && v <= ci
		}
		return false
	case "LESS_THAN":
		if v, isInt := val.(int); isInt {
			ci, ok := tryInt()
			return ok && v < ci
		}
		return false
	}
	return true
}

// JoinSession mirrors EosGatewayServer.JoinSession.
func (s *EosGatewayServer) JoinSession(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	sessionID, productUserID := c.Param("session_id"), c.Param("product_user_id")

	sessionLock.Lock()
	for _, x := range Sessions {
		if x.Id == sessionID {
			if !containsString(x.PublicPlayers, productUserID) {
				x.PublicPlayers = append(x.PublicPlayers, productUserID)
			}
		} else if containsString(x.PublicPlayers, productUserID) {
			// [Solara 2026-11-05] A player is on one game server at a time. When they move servers the client does not
			// always send the leave for the old session, so they stayed listed there too (the station list then
			// showed e.g. Freerunners full of players who had gone to Clubhouse). Joining a session ends any other.
			x.PublicPlayers = removeFirst(x.PublicPlayers, productUserID)
		}
	}
	sessionLock.Unlock()

	s.HTTP.Logger.Information("Player joined session | session=" + sessionID + " player=" + productUserID)
	return hosting.Configurable(200, "application/json", nil)
}

// LeaveSession mirrors EosGatewayServer.LeaveSession.
func (s *EosGatewayServer) LeaveSession(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	sessionID, productUserID := c.Param("session_id"), c.Param("product_user_id")

	sessionLock.Lock()
	for _, x := range Sessions {
		if x.Id == sessionID {
			x.PublicPlayers = removeFirst(x.PublicPlayers, productUserID)
			break
		}
	}
	sessionLock.Unlock()

	s.HTTP.Logger.Information("Player left session | session=" + sessionID + " player=" + productUserID)
	return hosting.Configurable(200, "application/json", nil)
}

// ── User search ──────────────────────────────────────────────────────────

// ProductUserSearch mirrors EosGatewayServer.ProductUserSearch.
func (s *EosGatewayServer) ProductUserSearch(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)

	bodyStr := utf8GetString(c.Request.Body())

	result := stj.Obj{}
	tryRun(func() {
		root := stj.MustParse([]byte(bodyStr))
		ids, ok := root.TryGetProperty("productUserIds")
		if !ok {
			return
		}
		for _, el := range ids.EnumerateArray() {
			p := el.GetString()
			if p == nil || *p == "" {
				continue
			}
			puid := *p
			displayName, known := userNamesGet(puid)
			if !known {
				displayName = utf16Prefix(puid, 8)
			}
			result.Set(puid, stj.O(
				"accounts", []stj.Obj{stj.O(
					"accountId", puid,
					"displayName", displayName,
					"identityProviderId", "deviceid",
					"lastLogin", dotnet.UtcNow().Format("yyyy-MM-ddTHH:mm:ssZ"),
				)},
			))
		}
	})

	s.HTTP.Logger.Information("ProductUser search | resolved " + strconv.Itoa(len(result)) + " puids")
	return hosting.Ok(stj.O("productUsers", result))
}

// ── Telemetry / DataRouter ────────────────────────────────────────────────

// Telemetry mirrors EosGatewayServer.Telemetry.
func (s *EosGatewayServer) Telemetry(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	raw := utf8GetString(c.Request.Body())
	PushTelemetry("telemetry.data", raw)
	return hosting.Ok(stj.O())
}

// DataRouterEvent mirrors EosGatewayServer.DataRouterEvent.
func (s *EosGatewayServer) DataRouterEvent(c *hosting.Context) hosting.Result {
	addEosHeaders(c.Response)
	tryRun(func() {
		var payload *models.EosTelemetryRequest
		if err := stj.Unmarshal(c.Request.Body(), &payload); err != nil {
			panic(err)
		}
		if payload != nil && payload.Events != nil {
			for _, ev := range payload.Events {
				PushTelemetry(ev.EventName, stj.MarshalString(ev))
			}
		}
	})
	return hosting.Ok(stj.O())
}
