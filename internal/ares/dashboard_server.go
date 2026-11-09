// Ports AUnrealFeatures.Ares/Servers/DashboardServer.cs (class
// AresDashboardServer, :8080 — the dashboard's read API, login, telemetry):
// constructor, route table, UI/SSO, stats, users/admin, board upload,
// netvars, station config, spec levels and the station whitelist. The
// remaining handlers live in dashboard_handlers.go and dashboard_drafters.go
// (the partial class in DrafterPermissions.cs).
package ares

import (
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/stj"
)

const (
	dashboardHostname = "localhost"
	dashboardPort     = 8080
)

// dashboardHtml mirrors the static DashboardHtml: the embedded
// Dashboard/index.html resource (the C# fallback "<h1>Dashboard HTML not
// found</h1>" cannot occur because the file is compiled in).
//
//go:embed assets/index.html
var dashboardHtml string

// AresDashboardServer mirrors the AstraHttpServer subclass.
type AresDashboardServer struct {
	HTTP *hosting.Server
}

// NewAresDashboardServer mirrors the constructor (base(HOSTNAME, PORT) +
// AddPreprocessor<DashboardAuthPreprocessor>()), registers the attribute
// endpoints in reflection order and starts listening.
func NewAresDashboardServer() (*AresDashboardServer, error) {
	s := newAresDashboardServer()
	dashboardStartupWarnings() // SEC-DASH-4
	if err := s.HTTP.Start(); err != nil {
		return nil, err
	}
	return s, nil
}

// newAresDashboardServer builds the server without binding (tests).
func newAresDashboardServer() *AresDashboardServer {
	s := &AresDashboardServer{HTTP: hosting.NewServer("AresDashboardServer", dashboardHostname, dashboardPort)}
	s.HTTP.BindAddress = os.Getenv("DASHBOARD_BIND_ADDRESS") // SEC-DASH-5 ("" = all interfaces)
	// Gate /api/* on this server only (dashboard).
	s.HTTP.AddPreprocessor(&DashboardAuthPreprocessor{})
	h := s.HTTP
	h.Handle("GET", "/", "Index", s.Index)
	h.Handle("GET", "/auth/login", "AuthLogin", s.AuthLogin)
	h.Handle("GET", "/auth/meta", "AuthMetaCallback", s.AuthMetaCallback)
	h.Handle("GET", "/api/stats", "GetStats", s.GetStats)
	h.Handle("GET", "/api/users", "GetUsers", s.GetUsers)
	h.Handle("POST", "/api/users/{user_id}/grant_admin", "GrantAdmin", s.GrantAdmin)
	h.Handle("DELETE", "/api/users/{user_id}/grant_admin", "RevokeAdmin", s.RevokeAdmin)
	h.Handle("POST", "/api/stations/{station_id}/board", "UploadStationBoard", s.UploadStationBoard)
	h.Handle("GET", "/api/board/keys", "BoardKeys", s.BoardKeys)
	h.Handle("GET", "/api/stations/{station_id}/board", "GetStationBoard", s.GetStationBoard)
	h.Handle("DELETE", "/api/stations/{station_id}/board", "ClearStationBoard", s.ClearStationBoard)
	h.Handle("GET", "/api/netvars/dumps", "ListNetvarDumps", s.ListNetvarDumps)
	h.Handle("GET", "/api/netvars/dump", "GetNetvarDump", s.GetNetvarDump)
	h.Handle("GET", "/api/stations/{station_id}/config", "GetStationConfig", s.GetStationConfig)
	h.Handle("PATCH", "/api/stations/{station_id}/config", "PatchStationConfig", s.PatchStationConfig)
	h.Handle("GET", "/api/spec/levels", "GetSpecLevels", s.GetSpecLevels)
	h.Handle("GET", "/api/spec/levels/{name}", "GetSpecLevel", s.GetSpecLevel)
	h.Handle("PATCH", "/api/spec/levels/{name}", "PatchSpecLevel", s.PatchSpecLevel)
	h.Handle("DELETE", "/api/spec/levels/{name}", "DeleteSpecLevel", s.DeleteSpecLevel)
	h.Handle("GET", "/api/stations/{station_id}/whitelist", "GetStationWhitelist", s.GetStationWhitelist)
	h.Handle("PUT", "/api/stations/{station_id}/whitelist", "PutStationWhitelist", s.PutStationWhitelist)
	h.Handle("DELETE", "/api/stations/{station_id}/config", "DeleteStationConfigKey", s.DeleteStationConfigKey)
	h.Handle("GET", "/api/stations", "GetStations", s.GetStations)
	h.Handle("DELETE", "/api/stations/{station_id}", "DeleteStationApi", s.DeleteStationApi)
	h.Handle("GET", "/api/deployments", "GetDeployments", s.GetDeployments)
	h.Handle("GET", "/api/events", "GetEvents", s.GetEvents)
	h.Handle("POST", "/api/stations", "CreateStationApi", s.CreateStationApi)
	h.Handle("POST", "/api/stations/{station_id}/deployments", "CreateDeploymentApi", s.CreateDeploymentApi)
	h.Handle("POST", "/api/stations/{station_id}/spinup", "SpinUpDeployment", s.SpinUpDeployment)
	h.Handle("GET", "/api/agents", "GetAgents", s.GetAgents)
	h.Handle("POST", "/api/stations/{station_id}/events", "CreateEventApi", s.CreateEventApi)
	h.Handle("GET", "/api/stations/{station_id}/events", "GetStationEventsApi", s.GetStationEventsApi)
	h.Handle("DELETE", "/api/stations/{station_id}/events/{event_id}", "DeleteEventApi", s.DeleteEventApi)
	h.Handle("GET", "/api/stations/{station_id}/deployments", "GetStationDeploymentsApi", s.GetStationDeploymentsApi)
	h.Handle("POST", "/api/users/{user_id}/ban", "BanUser", s.BanUser)
	h.Handle("PATCH", "/api/users/{user_id}/unban", "UnbanUser", s.UnbanUser)
	h.Handle("GET", "/api/stations/{station_id}/bans", "GetStationBans", s.GetStationBans)
	h.Handle("GET", "/api/stations/{station_id}/roles", "GetStationRolesApi", s.GetStationRolesApi)
	h.Handle("POST", "/api/stations/{station_id}/roles", "CreateStationRoleApi", s.CreateStationRoleApi)
	h.Handle("DELETE", "/api/stations/{station_id}/roles/{role_id}", "DeleteStationRoleApi", s.DeleteStationRoleApi)
	h.Handle("PATCH", "/api/stations/{station_id}/roles/{role_id}/permissions", "UpdateRolePermissionsApi", s.UpdateRolePermissionsApi)
	h.Handle("POST", "/api/stations/{station_id}/users/{user_id}/roles/{role_id}", "AssignRoleApi", s.AssignRoleApi)
	h.Handle("DELETE", "/api/stations/{station_id}/users/{user_id}/roles/{role_id}", "RemoveRoleApi", s.RemoveRoleApi)
	h.Handle("GET", "/api/sessions", "GetSessionsApi", s.GetSessionsApi)
	h.Handle("POST", "/api/sessions", "AddSessionApi", s.AddSessionApi)
	h.Handle("DELETE", "/api/sessions/{session_id}", "RemoveSessionApi", s.RemoveSessionApi)
	h.Handle("GET", "/api/telemetry", "GetTelemetry", s.GetTelemetry)
	h.Handle("GET", "/api/ws-clients", "GetWsClients", s.GetWsClients)
	h.Handle("GET", "/api/ws-clients/{session_id}/messages", "GetWsMessages", s.GetWsMessages)
	h.Handle("GET", "/api/stations/{station_id}/members", "GetStationMembers", s.GetStationMembers)
	h.Handle("GET", "/api/drafters/{scope}", "GetDrafters", s.GetDrafters)
	h.Handle("POST", "/api/drafters/{scope}/{user_id}", "GrantDrafter", s.GrantDrafter)
	h.Handle("DELETE", "/api/drafters/{scope}/{user_id}", "RevokeDrafter", s.RevokeDrafter)
	// -- Solara additions, registered after every original route (SEC-SID) --
	h.Handle("GET", "/api/server-tokens", "ListServerTokens", s.ListServerTokens)
	h.Handle("DELETE", "/api/server-tokens/{id}", "RevokeServerToken", s.RevokeServerToken)
	h.Handle("GET", "/api/game_bans", "Solara/GetGameBans", s.GetGameBans)
	h.Handle("POST", "/api/game_bans/{player_id}/unban", "Solara/LiftGameBans", s.LiftGameBans)
	h.Handle("POST", "/api/game_bans", "Solara/AddGameBan", s.AddGameBan)
	h.Handle("GET", "/api/catalog/items", "Solara/GetGrantableItems", s.GetGrantableItems)
	h.Handle("GET", "/api/users/{user_id}/inventory", "Solara/GetUserInventory", s.GetUserInventory)
	h.Handle("POST", "/api/users/{user_id}/inventory", "Solara/GrantUserItem", s.GrantUserItem)
	h.Handle("DELETE", "/api/users/{user_id}/inventory/{item_id}", "Solara/RevokeUserItem", s.RevokeUserItem)
	h.Handle("GET", "/api/stations/{station_id}/gaia", "Solara/GetGaia", s.GetGaia)
	h.Handle("POST", "/api/stations/{station_id}/gaia/play", "Solara/PlayGaia", s.PlayGaia)
	h.Handle("GET", "/api/stations/{station_id}/notify", "Solara/GetNotify", s.GetNotify)
	h.Handle("POST", "/api/stations/{station_id}/notify", "Solara/SendNotify", s.SendNotify)
	h.Handle("GET", "/api/mutes", "Solara/GetMutes", s.GetMutes)
	h.Handle("POST", "/api/mutes", "Solara/AddMute", s.AddMuteApi)
	h.Handle("POST", "/api/mutes/{player_id}/unmute", "Solara/LiftMute", s.LiftMuteApi)
	h.Handle("GET", "/api/titledata", "Solara/GetTitleDataAdmin", s.GetTitleDataAdmin)
	h.Handle("PUT", "/api/titledata/{key}", "Solara/PutTitleDataAdmin", s.PutTitleDataAdmin)
	h.Handle("DELETE", "/api/titledata/{key}", "Solara/DeleteTitleDataAdmin", s.DeleteTitleDataAdmin)
	h.Handle("GET", "/api/scoped-tokens/permissions", "Solara/ListScopedTokenPerms", s.ListScopedTokenPerms)
	h.Handle("GET", "/api/scoped-tokens/self", "Solara/ScopedTokenSelf", s.ScopedTokenSelf)
	h.Handle("GET", "/api/scoped-tokens", "Solara/ListScopedTokens", s.ListScopedTokens)
	h.Handle("POST", "/api/scoped-tokens", "Solara/CreateScopedToken", s.CreateScopedToken)
	h.Handle("DELETE", "/api/scoped-tokens/{id}", "Solara/RevokeScopedToken", s.RevokeScopedToken)
	// Anonymous-mode name generator (master key only; see dashboard_anonymous_names.go).
	h.Handle("GET", "/api/anonymous-names", "Solara/GetAnonymousNamesAdmin", s.GetAnonymousNamesAdmin)
	h.Handle("PUT", "/api/anonymous-names", "Solara/PutAnonymousNamesAdmin", s.PutAnonymousNamesAdmin)
	h.Handle("DELETE", "/api/anonymous-names", "Solara/DeleteAnonymousNamesAdmin", s.DeleteAnonymousNamesAdmin)
	h.Handle("POST", "/api/anonymous-names/preview", "Solara/PreviewAnonymousNamesAdmin", s.PreviewAnonymousNamesAdmin)
	// Sandbox tab: the Axiom S3 sandbox content the game servers have fetched through /sandboxprojects (sandbox_proxy.go).
	h.Handle("GET", "/api/sandbox/objects", "Solara/GetSandboxObjects", s.GetSandboxObjects)
	// Users tab: paged search over every user, and per-user global permissions (see dashboard_users_admin.go).
	h.Handle("GET", "/api/users/search", "Solara/SearchUsers", s.SearchUsers)
	h.Handle("GET", "/api/stations/{station_id}/matches", "Solara/GetStationMatches", s.GetStationMatches)
	h.Handle("GET", "/api/users/{user_id}/global_permissions", "Solara/GetUserGlobalPermissions", s.GetUserGlobalPermissions)
	h.Handle("PUT", "/api/users/{user_id}/global_permissions", "Solara/PutUserGlobalPermissions", s.PutUserGlobalPermissions)
	h.Handle("POST", "/api/audit", "Solara/PostAudit", s.PostAudit) // dashboard audit log (dashboard_audit.go)
	h.Handle("GET", "/api/audit", "Solara/GetAudit", s.GetAudit)
	return s
}

// dashboardApiKeySigningKey is the literal HMAC key GenerateDashboardApiKey
// used. It is not one of the configured legacy secrets (docs/PORTING_GUIDE.md
// section 7 lists them exactly); the same literal is also used by
// A2StationDbServer.GenerateApiKey.
const dashboardApiKeySigningKey = "astra-api-key-secret"

// dashboardGenerateApiKey mirrors the private static GenerateDashboardApiKey.
func dashboardGenerateApiKey(ownerId string, keyType string) string {
	keyId := dotnet.NewGuid()
	now := dotnet.UtcNow()
	// ToString("yyyy-MM-dd HH:mm:ss.ffffff")
	createdAt := now.Format("yyyy-MM-dd HH:mm:ss") + fmt.Sprintf(".%06d", (now.Ticks()%dotnet.TicksPerSecond)/10)
	payload := stj.O("key_id", keyId, "key_type", keyType, "owner_id", ownerId, "created_at", createdAt)
	header := dashboardBase64UrlEncode(stj.MustMarshal(stj.O("alg", "HS256", "typ", "JWT")))
	body := dashboardBase64UrlEncode(stj.MustMarshal(payload))
	signing := header + "." + body
	mac := hmac.New(sha256.New, []byte(dashboardApiKeySigningKey))
	mac.Write([]byte(signing))
	return signing + "." + dashboardBase64UrlEncode(mac.Sum(nil))
}

// dashboardBase64UrlEncode mirrors the private static Base64UrlEncode.
func dashboardBase64UrlEncode(data []byte) string { return dashboardB64Url(data) }

// ── UI ───────────────────────────────────────────────────────────────────

func (s *AresDashboardServer) Index(c *hosting.Context) hosting.Result {
	return hosting.HtmlDocument(200, dashboardHtml)
}

// ── Meta (Oculus) SSO ──────────────────────────────────────────────────────
// browser -> https://auth.oculus.com/sso/?redirect_uri=<dash>/auth/meta&organization_id=<org>.
const (
	dashboardMetaOrgId       = "1458804674917108" // OUR Meta organization
	dashboardOculusSsoUrl    = "https://auth.oculus.com/sso/"
	dashboardDashboardOrigin = "http://localhost:3000" // dev origin for now
)

func (s *AresDashboardServer) AuthLogin(c *hosting.Context) hosting.Result {
	redirectUri := dashboardDashboardOrigin + "/auth/meta"
	sso := dashboardOculusSsoUrl + "?redirect_uri=" + dashEscapeDataString(redirectUri) +
		"&organization_id=" + dashboardMetaOrgId
	// JS + meta-refresh redirect (framework-agnostic) so the browser bounces to Meta sign-in.
	return hosting.HtmlDocument(200,
		"<!doctype html><meta http-equiv='refresh' content='0;url="+sso+"'>"+
			"<script>location.href="+stj.MarshalString(sso)+"</script>Redirecting to Meta sign-in…")
}

// AuthMetaCallback is the CAPTURE PHASE: it logs exactly what Meta returns.
func (s *AresDashboardServer) AuthMetaCallback(c *hosting.Context) hosting.Result {
	req := c.Request
	queries := req.Queries() // throws like the original on a key-less segment
	nv := hosting.ParseNameValues(req.Raw().URL.RawQuery)
	parts := make([]string, 0, len(nv.Keys()))
	for _, k := range nv.Keys() { // Dictionary enumeration = AllKeys order
		parts = append(parts, dashboardSanitizeForLog("", k)+"="+dashboardSanitizeForLog(k, queries[k])) // SEC-DASH: redact/neutralise
	}
	q := strings.Join(parts, " | ")
	referer, _ := req.Header("Referer")
	referer = dashboardSanitizeForLog("", referer)
	logging.Console("[DASH-SSO] /auth/meta callback remote=" + req.Remote() + " host=" + req.Host() + " " +
		"uri=" + req.Uri() + " queries=[" + q + "] referer=" + referer)
	return hosting.HtmlDocument(200,
		"<h2>Meta sign-in received</h2><p>Callback captured — check the server log line tagged "+
			"<code>[DASH-SSO]</code> for the params, then we finish the validator + session.</p>")
}

// ── API ──────────────────────────────────────────────────────────────────

func (s *AresDashboardServer) GetStats(c *hosting.Context) hosting.Result {
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	deployments := docdb.GetCollection(Database, true, models.NewDeploymentDbObject)
	events := docdb.GetCollection(Database, true, models.NewServerEventDbObject)

	allDeployments := []*models.DeploymentDbObject{}
	if deployments != nil {
		allDeployments = deployments.FindAll()
	}
	st := NewDashboardStats()
	if users != nil {
		st.TotalUsers = len(users.FindAll())
	}
	if stations != nil {
		st.TotalStations = len(stations.FindAll())
	}
	st.TotalDeployments = len(allDeployments)
	for _, d := range allDeployments {
		if d.Online {
			st.OnlineDeployments++
		}
	}
	for _, d := range allDeployments {
		if d.Online {
			st.TotalPlayers += d.PlayerCount
		}
	}
	if events != nil {
		st.TotalEvents = len(events.FindAll())
	}
	return hosting.Ok(st)
}

const dashboardGlobalAdminRoleId = "__global_admin__"

// dashboardEnsureGlobalAdminRole mirrors the private static EnsureGlobalAdminRole.
func dashboardEnsureGlobalAdminRole() *models.RoleResponse {
	roles := docdb.GetCollection(Database, true, models.NewRoleResponse)
	var existing *models.RoleResponse
	if roles != nil {
		existing = roles.FindOne(func(r *models.RoleResponse) bool { return dashLiteEq(r.RoleId, dashboardGlobalAdminRoleId) })
	}
	if existing != nil {
		return existing
	}
	role := models.NewRoleResponse()
	role.RoleId = dashStrPtr(dashboardGlobalAdminRoleId)
	role.StationId = dashStrPtr("global")
	role.RoleName = dashStrPtr("Global Administrator")
	role.RoleDescription = dashStrPtr("Full access to all permissions")
	role.Permissions = []*string{dashStrPtr("global:admin")}
	if roles != nil {
		roles.Insert(role)
	}
	return role
}

func (s *AresDashboardServer) GetUsers(c *hosting.Context) hosting.Result {
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	// Skip records with no UserId (half-written login rows are unactionable and
	// used to crash the UI's user_id.slice(0,8)).
	list := []*DashboardUser{}
	if users != nil {
		all := []*models.UserDataResponse{}
		for _, u := range users.FindAll() {
			if !dashIsNullOrWhiteSpaceP(u.UserId) {
				all = append(all, u)
			}
		}
		dashStableSort(all, func(a, b *models.UserDataResponse) bool { return a.LastLogin.After(b.LastLogin) })
		if len(all) > 50 {
			all = all[:50]
		}
		for _, u := range all {
			list = append(list, dashToDashboardUser(u))
		}
	}
	return hosting.Ok(list)
}

// dashToDashboardUser is GetUsers' per-user projection (shared with the Solara paged search).
func dashToDashboardUser(u *models.UserDataResponse) *DashboardUser {
	du := NewDashboardUser()
	du.UserId = dashDeref(u.UserId)
	if dashIsNullOrWhiteSpaceP(u.Username) {
		du.Username = "(unknown)"
	} else {
		du.Username = *u.Username
	}
	du.Platform = u.Platform
	du.LastLogin = u.LastLogin
	du.CreatedAt = u.CreatedAt
	du.RoleCount = len(u.Roles)
	// `b.Expiration == null` is always false for a DateTime (kept as written).
	if u.Bans != nil {
		now := dotnet.UtcNow()
		for _, b := range u.Bans {
			if !b.Revoked && b.Expiration.After(now) {
				du.Banned = true
				break
			}
		}
	}
	du.IsAdmin = u.Roles != nil && dashListContains(u.Roles, dashboardGlobalAdminRoleId)
	return du
}

func (s *AresDashboardServer) GrantAdmin(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	userCol := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	var user *models.UserDataResponse
	if userCol != nil {
		user = userCol.FindOne(func(u *models.UserDataResponse) bool { return dashLiteEq(u.UserId, userId) })
	}
	if user == nil {
		return hosting.Ok(&DashboardActionResult{Success: false, Error: dashStrPtr("user not found")})
	}

	dashboardEnsureGlobalAdminRole()
	if user.Roles == nil {
		user.Roles = []*string{}
	}
	if !dashListContains(user.Roles, dashboardGlobalAdminRoleId) {
		user.Roles = append(user.Roles, dashStrPtr(dashboardGlobalAdminRoleId))
		if userCol != nil {
			userCol.Update(user)
		}
	}
	return hosting.Ok(&DashboardActionResult{Success: true})
}

func (s *AresDashboardServer) RevokeAdmin(c *hosting.Context) hosting.Result {
	userId := c.Param("user_id")
	userCol := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	var user *models.UserDataResponse
	if userCol != nil {
		user = userCol.FindOne(func(u *models.UserDataResponse) bool { return dashLiteEq(u.UserId, userId) })
	}
	if user == nil {
		return hosting.Ok(&DashboardActionResult{Success: false, Error: dashStrPtr("user not found")})
	}

	if user.Roles != nil {
		var removed bool
		user.Roles, removed = dashListRemove(user.Roles, dashboardGlobalAdminRoleId)
		if removed && userCol != nil {
			userCol.Update(user)
		}
	}
	return hosting.Ok(&DashboardActionResult{Success: true})
}

// --- BOARD IMAGE UPLOAD (dashboard side) --------------------------------------
//   POST /api/stations/{station_id}/board?key=<BoardTextureUrl1|SignPlazaFront|...>
//   body: raw image bytes (png/jpg/gif/webp)
// The stored URL must be the EXTERNAL origin (BOARD_PUBLIC_BASE, falling back
// to the known public StationDb hostname).

func (s *AresDashboardServer) UploadStationBoard(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	key := c.Request.GetQueryParameter("key", "")
	if dashIndexOf(BoardConfigKeys, key) < 0 {
		return hosting.BadRequest(stj.O("error", "unknown board key", "allowed", BoardConfigKeys))
	}

	body := c.Request.Body()
	if len(body) == 0 {
		return hosting.BadRequest(stj.O("error", "empty body"))
	}
	if len(body) > 8*1024*1024 {
		return hosting.BadRequest(stj.O("error", "image too large (max 8MB)"))
	}

	ext := SniffImageExtensionPublic(body)
	if ext == nil {
		return hosting.BadRequest(stj.O("error", "not a png/jpg/gif/webp image"))
	}

	stationCollection := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stationCollection != nil {
		station = stationCollection.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.NotFound(stj.O("error", "no such station", "station_id", stationId))
	}

	dir := BoardUploadDirPublic()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	safeStation := SanitiseForFileNamePublic(stationId)
	fileName := fmt.Sprintf("%s_%s_%d.%s", safeStation, key, time.Now().Unix(), *ext)

	// a leftover file is harmless; never fail the upload over cleanup
	func() {
		defer func() { _ = recover() }()
		for _, stale := range dashGetFiles(dir, safeStation+"_"+key+"_") {
			if err := os.Remove(stale); err != nil {
				return
			}
		}
	}()

	if err := os.WriteFile(filepath.Join(dir, fileName), body, 0o644); err != nil {
		panic(err)
	}

	baseUrl := strings.TrimRight(os.Getenv("BOARD_PUBLIC_BASE"), "/")
	if dashIsNullOrWhiteSpace(baseUrl) {
		baseUrl = "http://209.126.2.136:78"
	}
	url := baseUrl + "/board/" + fileName

	if station.Config == nil {
		dotnet.Throw("System.NullReferenceException", "Object reference not set to an instance of an object.")
	}
	station.Config[key] = dashStrPtr(url)
	if stationCollection != nil {
		stationCollection.Update(station)
	}

	return hosting.Ok(stj.O("success", true, "key", key, "url", url, "bytes", len(body)))
}

// dashGetFiles mirrors Directory.GetFiles(dir, prefix + "*"): file names are
// matched case-insensitively on Windows, as the original's file system did.
func dashGetFiles(dir, prefix string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		match := strings.HasPrefix(n, prefix)
		if runtime.GOOS == "windows" {
			match = len(n) >= len(prefix) && strings.EqualFold(n[:len(prefix)], prefix)
		}
		if match {
			out = append(out, filepath.Join(dir, n))
		}
	}
	return out
}

func dashIndexOf(arr []string, v string) int {
	for i, x := range arr {
		if x == v {
			return i
		}
	}
	return -1
}

// BoardKeys: GET /api/board/keys -- the uploadable slots.
func (s *AresDashboardServer) BoardKeys(c *hosting.Context) hosting.Result {
	return hosting.Ok(BoardConfigKeys)
}

// GetStationBoard: current board URLs for this station.
func (s *AresDashboardServer) GetStationBoard(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	stationCollection := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stationCollection != nil {
		station = stationCollection.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	// Dictionary<string, string>: a stored value that reads back as null
	// (LiteDB "" -> null) is served as null.
	result := stj.Obj{}
	for _, k := range BoardConfigKeys {
		v := dashStrPtr("")
		if station != nil {
			if cv, ok := station.Config[k]; ok {
				v = cv
			}
		}
		result.Set(k, v)
	}
	return hosting.Ok(result)
}

// ClearStationBoard: DELETE ?key=... -- clear one board back to blank.
func (s *AresDashboardServer) ClearStationBoard(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	key := c.Request.GetQueryParameter("key", "")
	stationCollection := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stationCollection != nil {
		station = stationCollection.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.NotFound(nil)
	}
	if station.Config == nil {
		dotnet.Throw("System.NullReferenceException", "Object reference not set to an instance of an object.")
	}
	delete(station.Config, key)
	if stationCollection != nil {
		stationCollection.Update(station)
	}
	return hosting.Ok(stj.O("success", true, "key", key))
}

// --- NETVARS -----------------------------------------------------------------------------------

// ListNetvarDumps: which deployments have reported a dump, how big, how fresh.
func (s *AresDashboardServer) ListNetvarDumps(c *hosting.Context) hosting.Result {
	return hosting.Ok(NetvarDumpStoreList())
}

// GetNetvarDump: ?deployment_id=<id>&type=netvars|state -- the raw JSON the game sent.
func (s *AresDashboardServer) GetNetvarDump(c *hosting.Context) hosting.Result {
	dep := c.Request.GetQueryParameter("deployment_id", "")
	typ := c.Request.GetQueryParameter("type", "netvars")
	d := Get(&dep, typ)
	if d == nil {
		return hosting.NotFound(stj.O("error", "no dump received yet", "deployment_id", dep, "type", typ))
	}
	return hosting.Configurable(200, "application/json", []byte(d.Data))
}

// GetStationConfig: the station's own key/values (not the served defaults).
func (s *AresDashboardServer) GetStationConfig(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	col := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if col != nil {
		station = col.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.NotFound(stj.O("error", "no such station", "station_id", stationId))
	}
	if station.Config == nil {
		return hosting.Ok(map[string]string{})
	}
	return hosting.Ok(station.Config)
}

// PatchStationConfig: merge {key: value} pairs into the station config.
func (s *AresDashboardServer) PatchStationConfig(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	keys, data, ok := dashDeserializeStringDict(c.Request.Body())
	if !ok || len(keys) == 0 {
		return hosting.BadRequest(stj.O("error", "body must be a JSON object of string keys to string values"))
	}

	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stations != nil {
		station = stations.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.NotFound(stj.O("error", "no such station", "station_id", stationId))
	}
	if station.Config == nil {
		station.Config = map[string]*string{}
	}
	targets := []*models.StationDbObject{station}
	if dashFleetScope(c) { // Solara: ?fleet=1 applies the same keys to every station (fleet.go)
		targets = []*models.StationDbObject{station}
		for _, st := range stations.FindAll() { // FindAll returns fresh copies: keep `station` as the page's own
			if !dashLiteEq(st.StationId, stationId) {
				targets = append(targets, st)
			}
		}
	}
	for _, st := range targets {
		if st.Config == nil {
			st.Config = map[string]*string{}
		}
		for _, k := range keys {
			if dashIsNullOrWhiteSpace(k) {
				continue
			}
			st.Config[dashTrim(k)] = dashStrPtr(dashDeref(data[k])) // kv.Value ?? ""
		}
		stations.Update(st)
		OnStationConfigChanged(st.StationId) // push to co-located game servers
		Push(st.StationId)                   // push the station browser whitelist
	}
	return hosting.Ok(station.Config)
}

// ── SPEC EDITOR SAVED LEVELS ─────────────────────────────────────────────────────────────

func (s *AresDashboardServer) GetSpecLevels(c *hosting.Context) hosting.Result {
	list := []stj.Obj{}
	for _, l := range SpecLevelsStoreList() {
		list = append(list, stj.O(
			"name", l.Name,
			"autoload", l.Entry.Autoload,
			"loaded", l.Entry.Loaded,
			"updated", l.Entry.Updated,
			"size", l.Entry.Size,
			"serverLoaded", l.Entry.ServerLoaded,
			"serverSeen", l.Entry.ServerSeen,
		))
	}
	return hosting.Ok(list)
}

func (s *AresDashboardServer) GetSpecLevel(c *hosting.Context) hosting.Result {
	name := c.Param("name")
	n := SpecLevelsStoreCleanName(&name)
	if n == nil {
		return hosting.BadRequest(stj.O("error", "bad level name"))
	}
	text := SpecLevelsStoreReadText(*n)
	if text == nil {
		return hosting.NotFound(stj.O("error", "no such level", "name", *n))
	}
	return hosting.Ok(stj.O("name", *n, "text", *text))
}

// PatchSpecLevel: body { autoload?: bool, loaded?: bool }.
func (s *AresDashboardServer) PatchSpecLevel(c *hosting.Context) hosting.Result {
	name := c.Param("name")
	n := SpecLevelsStoreCleanName(&name)
	if n == nil {
		return hosting.BadRequest(stj.O("error", "bad level name"))
	}
	body := dashDeserialize(c.Request.Body(), func() *dashboardSpecLevelPatchBody { return &dashboardSpecLevelPatchBody{} })
	if body == nil || (body.Autoload == nil && body.Loaded == nil) {
		return hosting.BadRequest(stj.O("error", "body must be { autoload?: bool, loaded?: bool }"))
	}
	if !SpecLevelsStoreUpdate(*n, body.Autoload, body.Loaded) {
		return hosting.NotFound(stj.O("error", "no such level", "name", *n))
	}
	return hosting.Ok(stj.O("ok", true))
}

func (s *AresDashboardServer) DeleteSpecLevel(c *hosting.Context) hosting.Result {
	name := c.Param("name")
	n := SpecLevelsStoreCleanName(&name)
	if n == nil {
		return hosting.BadRequest(stj.O("error", "bad level name"))
	}
	if !SpecLevelsStoreDelete(*n) {
		return hosting.NotFound(stj.O("error", "no such level", "name", *n))
	}
	return hosting.Ok(stj.O("ok", true))
}

// ── STATION WHITELIST ────────────────────────────────────────────────────────────────────
// Stored in station Config under "acl.whitelist" as a comma-separated list of
// usernames. An empty list means "visible to everyone".

func (s *AresDashboardServer) GetStationWhitelist(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stations != nil {
		station = stations.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.NotFound(stj.O("error", "no such station", "station_id", stationId))
	}

	var raw *string
	if station.Config != nil {
		if v, ok := station.Config[WhitelistKey]; ok {
			raw = v
		}
	}
	listed := Parse(raw)

	// Every account (not the 50 most recent); same guard as GetUsers.
	type account struct {
		userId    string
		username  string
		platform  *string
		lastLogin dotnet.DateTime
	}
	accts := []account{}
	if ucol := docdb.GetCollection(Database, true, models.NewUserDataResponse); ucol != nil {
		for _, u := range ucol.FindAll() {
			if !dashIsNullOrWhiteSpaceP(u.UserId) && !dashIsNullOrWhiteSpaceP(u.Username) {
				accts = append(accts, account{*u.UserId, *u.Username, u.Platform, u.LastLogin})
			}
		}
		dashStableSort(accts, func(a, b account) bool { return dashCompareOrdinalIgnoreCase(a.username, b.username) < 0 })
	}
	users := make([]stj.Obj, 0, len(accts))
	known := map[string]bool{}
	for _, a := range accts {
		users = append(users, stj.O("user_id", a.userId, "username", a.username, "platform", a.platform, "last_login", a.lastLogin))
		known[string(dashUpperKey(a.username))] = true
	}
	entries := make([]stj.Obj, 0, len(listed))
	for _, n := range listed {
		entries = append(entries, stj.O("username", n, "known", known[string(dashUpperKey(n))]))
	}
	return hosting.Ok(stj.O(
		"station_id", stationId,
		// An empty list is not "nobody" -- it is the default, "visible to everyone".
		"enabled", len(listed) > 0,
		"entries", entries,
		"accounts", users,
	))
}

// dashUpperKey is the OrdinalIgnoreCase hash-set key.
func dashUpperKey(s string) []rune {
	u := dashUpperInvariant(s)
	r := make([]rune, len(u))
	for i, x := range u {
		r[i] = rune(x)
	}
	return r
}

// PutStationWhitelist: body { "usernames": ["a","b"] }.
func (s *AresDashboardServer) PutStationWhitelist(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	body := dashDeserialize(c.Request.Body(), func() *dashboardWhitelistBody { return &dashboardWhitelistBody{} })
	if body == nil {
		return hosting.BadRequest(stj.O("error", `body must be {"usernames": [..]}`))
	}

	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stations != nil {
		station = stations.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.NotFound(stj.O("error", "no such station", "station_id", stationId))
	}

	// Normalise: trim, drop blanks, de-duplicate case-insensitively.
	clean := []string{}
	for _, n := range body.Usernames {
		t := dashTrim(dashDeref(n))
		if len(t) == 0 || strings.Contains(t, ",") {
			continue // a comma would split into two entries
		}
		dup := false
		for _, e := range clean {
			if dashEqualsOrdinalIgnoreCase(e, t) {
				dup = true
				break
			}
		}
		if !dup {
			clean = append(clean, t)
		}
	}

	if station.Config == nil {
		station.Config = map[string]*string{}
	}
	if len(clean) == 0 {
		delete(station.Config, WhitelistKey) // back to visible-to-everyone
	} else {
		station.Config[WhitelistKey] = dashStrPtr(strings.Join(clean, ", "))
	}
	stations.Update(station)

	OnStationConfigChanged(&stationId)
	Push(&stationId) // takes effect immediately; no restart, no redeploy
	logging.Console(fmt.Sprintf("[ACL] %s whitelist set to %d account(s)", stationId, len(clean)))

	return hosting.Ok(stj.O("success", true, "station_id", stationId, "enabled", len(clean) > 0, "usernames", clean))
}

// DeleteStationConfigKey: ?key=<name> -- remove one key (falls back to the default).
func (s *AresDashboardServer) DeleteStationConfigKey(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	key := c.Request.GetQueryParameter("key", "")
	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	var station *models.StationDbObject
	if stations != nil {
		station = stations.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, stationId) })
	}
	if station == nil {
		return hosting.NotFound(stj.O("error", "no such station", "station_id", stationId))
	}
	targets := []*models.StationDbObject{station}
	if dashFleetScope(c) { // Solara: ?fleet=1 removes the key from every station (fleet.go)
		targets = stations.FindAll()
	}
	removed := false
	for _, st := range targets {
		if st.Config == nil {
			continue
		}
		if _, ok := st.Config[key]; ok {
			delete(st.Config, key)
			removed = true
			stations.Update(st)
			OnStationConfigChanged(st.StationId)
			Push(st.StationId)
		}
	}
	return hosting.Ok(stj.O("success", removed, "key", key))
}
