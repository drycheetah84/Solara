// Ports the helper section of AUnrealFeatures.Ares/Servers/A2StationDbServer.cs:
// api-key / id generation, response mappers, permissions, default station
// roles, the station board config (BoardConfigKeys, default board image,
// public base URL), the server master key / build constants and the Meta
// user-nonce verification.
package ares

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/logging"
	"solara/internal/secrets"
	"solara/internal/stj"
)

// ─── Collections ──────────────────────────────────────────────────────────

func a2Users() *docdb.Collection[models.UserDataResponse] {
	return docdb.GetCollection[models.UserDataResponse](Database, true, models.NewUserDataResponse)
}

func a2Keys() *docdb.Collection[models.UserApiKeyDbObject] {
	return docdb.GetCollection[models.UserApiKeyDbObject](Database, true, models.NewUserApiKeyDbObject)
}

func a2Roles() *docdb.Collection[models.RoleResponse] {
	return docdb.GetCollection[models.RoleResponse](Database, true, models.NewRoleResponse)
}

func a2Stations() *docdb.Collection[models.StationDbObject] {
	return docdb.GetCollection[models.StationDbObject](Database, true, models.NewStationDbObject)
}

func a2Deployments() *docdb.Collection[models.DeploymentDbObject] {
	return docdb.GetCollection[models.DeploymentDbObject](Database, true, models.NewDeploymentDbObject)
}

func a2ServerEvents() *docdb.Collection[models.ServerEventDbObject] {
	return docdb.GetCollection[models.ServerEventDbObject](Database, true, models.NewServerEventDbObject)
}

func a2StationEvents() *docdb.Collection[models.StationEventDbObject] {
	return docdb.GetCollection[models.StationEventDbObject](Database, true, models.NewStationEventDbObject)
}

func a2Signups() *docdb.Collection[models.EventSignupDbObject] {
	return docdb.GetCollection[models.EventSignupDbObject](Database, true, models.NewEventSignupDbObject)
}

// ─── Helpers ──────────────────────────────────────────────────────────────

// a2ApiKeyHmacKey is the fixed HMAC key of GenerateApiKey. The resulting
// signature is never verified anywhere (keys are looked up verbatim), so it is
// a format constant rather than a credential.
const a2ApiKeyHmacKey = "astra-api-key-secret"

// a2GenerateApiKey mirrors GenerateApiKey(ownerId, keyType = "user"): a
// JWT-shaped string over { key_id, key_type, owner_id, created_at }.
func a2GenerateApiKey(ownerId *string, keyType string) string {
	keyId := dotnet.NewGuid()
	now := dotnet.UtcNow()
	createdAt := now.Format("yyyy-MM-dd HH:mm:ss") + fmt.Sprintf(".%06d", now.Ticks()%dotnet.TicksPerSecond/10)
	payload := stj.O("key_id", keyId, "key_type", keyType, "owner_id", ownerId, "created_at", createdAt)
	header := a2Base64UrlEncode(stj.MustMarshal(stj.O("alg", "HS256", "typ", "JWT")))
	body := a2Base64UrlEncode(stj.MustMarshal(payload))
	signing := header + "." + body
	mac := hmac.New(sha256.New, []byte(a2ApiKeyHmacKey))
	mac.Write([]byte(signing))
	return signing + "." + a2Base64UrlEncode(mac.Sum(nil))
}

// a2Base64UrlEncode mirrors Base64UrlEncode (base64, +/ -> -_, '=' trimmed).
func a2Base64UrlEncode(data []byte) string {
	s := base64.StdEncoding.EncodeToString(data)
	s = strings.ReplaceAll(strings.ReplaceAll(s, "+", "-"), "/", "_")
	return strings.TrimRight(s, "=")
}

// a2GenerateId mirrors GenerateId(): Guid.NewGuid().ToString("N")[..16].
func a2GenerateId() string { return dotnet.NewGuidN()[:16] }

// a2DeploymentToResponse mirrors DeploymentToResponse.
func a2DeploymentToResponse(d *models.DeploymentDbObject) *models.ServerDeploymentResponse {
	last := dotnet.MinValue
	if d.LastEvent != nil {
		last = *d.LastEvent
	}
	return &models.ServerDeploymentResponse{
		DeploymentId:   d.DeploymentId,
		StationId:      d.StationId,
		DeploymentName: d.DeploymentName,
		Region:         d.Region,
		IpAddress:      d.IpAddress,
		Version:        d.Version,
		CreatedAt:      d.CreatedAt,
		Online:         d.Online,
		LastEventAt:    last,
		PlayerCount:    d.PlayerCount,
		Config:         d.Config,
	}
}

// a2StationToResponse mirrors StationToResponse(s, deps, includeDeployments, includeConfig).
// deps == nil stands for a null IEnumerable.
func a2StationToResponse(s *models.StationDbObject, deps []*models.DeploymentDbObject, includeDeployments, includeConfig bool) *models.StationResponse {
	r := &models.StationResponse{StationId: s.StationId, StationName: s.StationName, CreatedAt: s.CreatedAt}
	if includeDeployments && deps != nil {
		r.Deployments = []*models.ServerDeploymentResponse{}
		for _, d := range deps {
			r.Deployments = append(r.Deployments, a2DeploymentToResponse(d))
		}
	}
	if includeConfig {
		r.Config = s.Config
	}
	return r
}

// a2ExpandRoles mirrors ExpandRoles(roleIds).
func a2ExpandRoles(roleIds []*string) []*models.RoleResponse {
	roleCollection := a2Roles()
	out := []*models.RoleResponse{}
	for _, rid := range roleIds {
		if roleCollection == nil {
			continue
		}
		rid := rid
		if r := roleCollection.FindOne(func(r *models.RoleResponse) bool { return a2EqFoldPtr(r.RoleId, rid) }); r != nil {
			out = append(out, r)
		}
	}
	return out
}

// a2AllPermissions mirrors the private static AllPermissions.
var a2AllPermissions = []string{
	"admin",
	"station:create", "station:delete", "station:write", "station:read",
	"station_config:write", "station_config:read",
	"role:read", "role:write",
	"global:fleet:create", "global:fleet:delete", "fleet:read", "fleet:join", "fleet:write",
	"fleet_config:read", "fleet_config:write", "config_netvars:write", "custom_config:write",
	"record_entry:write",
	"server_login", "server_event:read", "server_event:write",
	"user:create", "user:delete", "global:user_data", "user_data:read", "user_login",
	"user_permissions:read", "user_roles:read", "user_roles:write",
	"user_ban:write", "user_ban_short:write", "user_ban:update", "user_ban:revoke",
	"user_mute", "user_kick", "user_warn",
	"global_voip", "dev_inputs",
	"key_holder", "key_holder_circuitlounge", "key_holder_driftplex", "key_holder_complex", "key_holder_office",
	"color:red", "color:yellow", "color:green",
	"anonymous_mode",
	"fleet_report:read", "fleet_report:write", "self:fleet_report:read", "self:fleet_report:write",
}

// a2GetPermissionsForUser mirrors GetPermissionsForUser(user).
func a2GetPermissionsForUser(user *models.UserDataResponse) []*models.UserPermission {
	roleCollection := a2Roles()
	perms := []*models.UserPermission{}
	for _, roleId := range user.Roles {
		if roleCollection == nil {
			continue
		}
		roleId := roleId
		role := roleCollection.FindOne(func(r *models.RoleResponse) bool { return a2EqFoldPtr(r.RoleId, roleId) })
		if role == nil || role.Permissions == nil {
			continue
		}
		if a2ContainsStr(role.Permissions, a2Str("global:admin")) {
			for _, perm := range a2AllPermissions {
				perms = append(perms, &models.UserPermission{StationId: a2Str("global"), Permission: a2Str(perm)})
			}
			return perms
		}
		for _, perm := range role.Permissions {
			perms = append(perms, &models.UserPermission{StationId: role.StationId, Permission: perm})
		}
	}
	return perms
}

// a2DefaultStationRoles mirrors DefaultStationRoles: the client
// (UA2StationDashboardSubsystem) only recognizes these three role names.
var a2DefaultStationRoles = []struct {
	Name  string
	Perms []string
}{
	// Full explicit permission set (not the "global:admin" sentinel).
	{"Admin", a2AllPermissions},
	{"Event Manager", []string{
		"server_event:read", "server_event:write",
		"station:read", "station:write",
		"station_config:read", "station_config:write",
		"config_netvars:write", "custom_config:write",
		"user_kick", "user_mute", "user_warn", "user_roles:read",
	}},
	{"Helper", []string{"station:read", "user_kick", "user_mute", "user_warn"}},
}

// a2EnsureStationRoles mirrors EnsureStationRoles(stationId): idempotently
// ensures Admin / Event Manager / Helper exist for a station.
func a2EnsureStationRoles(stationId string) {
	roleCollection := a2Roles()
	if roleCollection == nil {
		return
	}
	for _, def := range a2DefaultStationRoles {
		name := def.Name
		if roleCollection.FindOne(func(r *models.RoleResponse) bool {
			return a2EqFoldS(r.StationId, stationId) && a2EqFoldS(r.RoleName, name)
		}) != nil {
			continue
		}
		role := models.NewRoleResponse()
		role.RoleId = a2Str(a2GenerateId())
		role.StationId = a2Str(stationId)
		role.RoleName = a2Str(name)
		role.RoleDescription = a2Str("")
		role.Permissions = a2Strs(def.Perms...)
		roleCollection.Insert(role)
	}
}

// ─── STATION BOARD CONFIG (district info-boards / signs) ──────────────────
// The game's dedicated server fetches GET /v1/deployments/{id}?include_station_config=true
// and applies the config key/value pairs as NetVars; that is how the
// BP_Promoboard_* boards get their texture URLs. Images are uploaded to and
// served by this backend (POST /v1/board/upload, GET /board/{file}); the
// station's Config wins over the defaults (station.Config is TryAdd-ed first).

// BoardConfigKeys mirrors A2StationDbServer.BoardConfigKeys.
var BoardConfigKeys = []string{
	"BoardTextureUrl1", "BoardTextureUrl2", "BoardTextureUrl3",
	"BoardTextureUrl4", "BoardTextureUrl5", "BoardTextureUrl6",
	"SignPlazaFront", "SignPlazaWest", "SignPlazaEast",
	"SignComplexWest", "SignComplexEast", "SignComplexBasement",
	"SignArenaPrime",
	"statusBoardTKBMainUrl", "statusBoardTKBSidesUrl",
	"statusBoardTKBBack1Url", "statusBoardTKBBack2Url",
	"kingsCourtTeamLogosUrl",
}

// NetvarOverridePrefix mirrors A2StationDbServer.NetvarOverridePrefix:
// dashboard-set netvar overrides live in the station Config under "nv.".
const NetvarOverridePrefix = "nv."

// a2MaxBoardBytes mirrors MaxBoardBytes (8 MB).
const a2MaxBoardBytes = 8 * 1024 * 1024

// a2BoardUploadDir mirrors the static readonly BoardUploadDir
// (Path.Combine(Environment.CurrentDirectory, "boards")).
func a2BoardUploadDir() string { return filepath.Join(a2CurrentDir(), "boards") }

// a2DefaultStationConfig mirrors DefaultStationConfig (insertion-ordered).
var a2DefaultStationConfig = []struct{ Key, Value string }{
	{"StationAnnouncement", "Welcome to Halcyon Ring"},
}

// DefaultBoardFile mirrors A2StationDbServer.DefaultBoardFile.
const DefaultBoardFile = "_default.png"

// a2DefaultBoardPngBase64 is a 64x64 dark-grey PNG written on first use.
const a2DefaultBoardPngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAEAAAABACAYAAACqaXHeAAAAWklEQVR42u3QMQEAAAjDMOZf9DDB" +
	"RSChSc/aRAEFFFBAAQUUUEABBRRQQAEFFFBAAQUUUEABBRRQQAEFFFBAAQUUUEABBRRQQAEFFFBA" +
	"AQUUUEABBd4WuAAB0wABtAJ4wgAAAABJRU5ErkJggg=="

// EnsureDefaultBoardImage mirrors A2StationDbServer.EnsureDefaultBoardImage.
func EnsureDefaultBoardImage() {
	err := func() error {
		if err := os.MkdirAll(a2BoardUploadDir(), 0o755); err != nil {
			return err
		}
		f := filepath.Join(a2BoardUploadDir(), DefaultBoardFile)
		if st, err := os.Stat(f); err == nil && !st.IsDir() {
			return nil
		}
		png, _ := base64.StdEncoding.DecodeString(a2DefaultBoardPngBase64)
		return os.WriteFile(f, png, 0o644)
	}()
	if err != nil {
		logging.Console("[BOARD] could not write default board image: " + a2IOMessage(err))
	}
}

// a2BoardPublicBaseEnv mirrors (Environment.GetEnvironmentVariable("BOARD_PUBLIC_BASE") ?? "").TrimEnd('/').
func a2BoardPublicBaseEnv() string {
	return strings.TrimRight(os.Getenv("BOARD_PUBLIC_BASE"), "/")
}

// AddBoardDefaults mirrors A2StationDbServer.AddBoardDefaults(cfg, host):
// fill in any board key the station has not overridden.
func AddBoardDefaults(cfg map[string]*string, host string) {
	EnsureDefaultBoardImage()
	baseUrl := a2BoardPublicBaseEnv()
	if a2IsBlank(baseUrl) {
		baseUrl = PublicBoardBase(host)
	}
	u := baseUrl + "/board/" + DefaultBoardFile
	for _, k := range BoardConfigKeys {
		a2TryAdd(cfg, k, a2Str(u))
	}
}

// PublicBoardBase mirrors A2StationDbServer.PublicBoardBase(host): never hand
// back a loopback or private address as a board URL.
func PublicBoardBase(host string) string {
	const fallback = "http://209.126.2.136:78"
	if a2IsBlank(host) {
		return fallback
	}
	h := strings.Split(host, ":")[0]
	if strings.EqualFold(h, "localhost") ||
		strings.HasPrefix(h, "127.") || strings.HasPrefix(h, "10.") || strings.HasPrefix(h, "192.168.") ||
		h == "::1" || strings.HasPrefix(h, "172.16.") || strings.HasPrefix(h, "169.254.") {
		return fallback
	}
	return "https://" + host
}

// BoardUploadDirPublic mirrors A2StationDbServer.BoardUploadDirPublic.
func BoardUploadDirPublic() string { return a2BoardUploadDir() }

// SniffImageExtensionPublic mirrors A2StationDbServer.SniffImageExtensionPublic.
func SniffImageExtensionPublic(b []byte) *string { return a2SniffImageExtension(b) }

// SanitiseForFileNamePublic mirrors A2StationDbServer.SanitiseForFileNamePublic.
func SanitiseForFileNamePublic(s string) string { return a2SanitiseForFileName(s) }

// a2SniffImageExtension mirrors SniffImageExtension: the image type from its
// magic bytes, or null.
func a2SniffImageExtension(b []byte) *string {
	if len(b) >= 8 && b[0] == 0x89 && b[1] == 0x50 && b[2] == 0x4E && b[3] == 0x47 {
		return a2Str("png")
	}
	if len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return a2Str("jpg")
	}
	if len(b) >= 6 && b[0] == 0x47 && b[1] == 0x49 && b[2] == 0x46 {
		return a2Str("gif")
	}
	if len(b) >= 12 && b[0] == 0x52 && b[1] == 0x49 && b[2] == 0x46 && b[3] == 0x46 &&
		b[8] == 0x57 && b[9] == 0x45 && b[10] == 0x42 && b[11] == 0x50 {
		return a2Str("webp")
	}
	return nil
}

// a2SanitiseForFileName mirrors SanitiseForFileName: every char that is not
// char.IsLetterOrDigit becomes '-' (per UTF-16 unit; a surrogate pair -> "--").
func a2SanitiseForFileName(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch {
		case c >= 0x10000:
			b.WriteString("--") // surrogates are not letters/digits
		case a2IsLetterOrDigit(c):
			b.WriteRune(c)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// ─── Server identity / build constants ───────────────────────────────────

// a2ServerMasterKey mirrors the private const ServerMasterKey: the fixed key
// the dedicated server authenticates with (launch arg -DashboardApiKey=).
func a2ServerMasterKey() string { return secrets.Get("ServerMasterKey") }

// a2HeaderIsMasterKey mirrors `request.GetHeaderValue("x-api-key") != ServerMasterKey`
// negated (a missing header is null and never equal). The original key was a
// non-empty literal, so an unconfigured (empty) secret never matches.
func a2HeaderIsMasterKey(h *string) bool {
	k := a2ServerMasterKey()
	return h != nil && k != "" && *h == k
}

// BuildVersion / BuildIdNum: the A2 client build this backend serves; stamped
// onto every registered server's DB row and EOS session. The installers
// sed-patch these constants; the original never read them from the
// environment, so neither does the port.
const (
	a2BuildVersion = "22284"
	a2BuildIdNum   = 22284
)

// ─── Meta user-nonce verification (UserProof on /users/log_in) ───────────
// NonceVerify=false -> graph is never called (login path identical to before).
// NonceEnforce=true -> deny when the verified nonce is invalid.
const (
	a2NonceVerify  = false
	a2NonceEnforce = false
	a2QuestAppId   = "2236834297114138"
	// RIFT_APP_ID / RIFT_APP_SECRET are aliases of the Quest values.
	a2RiftAppId      = a2QuestAppId
	a2NonceCacheTtl  = int64(12 * 3600)
	a2NonceGraphUrl  = "https://graph.oculus.com/user_nonce_validate"
	a2NonceHttpLimit = 4 * time.Second
)

func a2QuestAppSecret() string { return secrets.Get("QUEST_APP_SECRET") }

// a2RiftAppSecret mirrors RIFT_APP_SECRET = QUEST_APP_SECRET.
func a2RiftAppSecret() string { return a2QuestAppSecret() }

// a2NonceHttp mirrors NonceHttp (4 s timeout: the accept loop is serial).
var a2NonceHttp = &http.Client{Timeout: a2NonceHttpLimit}

// a2NonceOkCache mirrors _nonceOkCache: userId -> unix-seconds expiry.
var (
	a2NonceMu      sync.Mutex
	a2NonceOkCache = map[string]int64{}
)

// a2VerifyNonceWith mirrors VerifyNonceWith: 1 = valid, 0 = definitively
// invalid, -1 = graph error/timeout.
func a2VerifyNonceWith(accessToken string, userId, nonce *string) (result int) {
	defer func() {
		if recover() != nil {
			result = -1
		}
	}()
	if userId == nil || nonce == nil {
		return -1 // FormUrlEncodedContent with a null value throws
	}
	// FormUrlEncodedContent keeps insertion order: access_token, nonce, user_id.
	body := "access_token=" + a2FormEscape(accessToken) + "&nonce=" + a2FormEscape(*nonce) + "&user_id=" + a2FormEscape(*userId)
	resp, err := a2NonceHttp.Post(a2NonceGraphUrl, "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		return -1
	}
	defer resp.Body.Close()
	text, err := io.ReadAll(resp.Body)
	if err != nil {
		return -1
	}
	doc, err := stj.Parse(text)
	if err != nil {
		return -1
	}
	if doc.ValueKind() != stj.Object {
		return -1 // TryGetProperty on a non-object throws
	}
	v, ok := doc.TryGetProperty("is_valid")
	valid := ok && (v.ValueKind() == stj.True ||
		(v.ValueKind() == stj.String && strings.ToLower(v.Str()) == "true"))
	if valid {
		return 1
	}
	return 0
}

// a2FormEscape mirrors FormUrlEncodedContent's encoding
// (Uri.EscapeDataString with %20 -> '+').
func a2FormEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else if c == ' ' {
			b.WriteByte('+')
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// a2VerifyNonceAny mirrors VerifyNonceAny: cache-first validation that fails
// open on Graph errors and rejects a definitively invalid nonce.
func a2VerifyNonceAny(userId, nonce *string) bool {
	if a2IsNullOrEmpty(userId) || a2IsNullOrEmpty(nonce) {
		return false
	}
	now := time.Now().Unix()
	a2NonceMu.Lock()
	exp, ok := a2NonceOkCache[*userId]
	if ok && exp > now {
		a2NonceMu.Unlock()
		return true // cache hit
	}
	if exp != 0 && exp <= now {
		delete(a2NonceOkCache, *userId) // evict stale
	}
	a2NonceMu.Unlock()
	q := a2VerifyNonceWith("OC|"+a2QuestAppId+"|"+a2QuestAppSecret(), userId, nonce)
	if q == 1 {
		a2NonceMu.Lock()
		a2NonceOkCache[*userId] = now + a2NonceCacheTtl
		a2NonceMu.Unlock()
		return true
	}
	if q == -1 {
		logging.Console("[A2DB][LOGIN] graph unreachable (quest) '" + *userId + "' -> FAIL-OPEN allow")
		return true
	}
	return false // the configured app rejected the nonce
}
