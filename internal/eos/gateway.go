// Port of AUnrealFeatures.EOSSDK/EosGatewayServer.cs — static state (sessions,
// lobbies, whitelists, persistence, lobby reaper, static constructor), the
// server type/constructor and route registration. Handlers live in
// gateway_handlers.go, gateway_playerdata.go and gateway_presence.go.
package eos

import (
	"embed"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"solara/internal/eos/models"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/stj"
)

const (
	gatewayHostname = "localhost" // EosGatewayServer.HOSTNAME
	gatewayPort     = 50          // EosGatewayServer.PORT

	PRODUCT_ID    = "472bc2b0df5c4a78921dbc8d6d70b7e7"
	SANDBOX_ID    = "2a166d77d93144289a40283f5d1d70ca"
	DEPLOYMENT_ID = "e203ea2f9d70483fb6149107d52c25b5"
	ORG_ID        = "o-p2vxwcxe6x6wv45luacent56lxbgun"
	CLIENT_ID     = "xyza7891wH5ksxQFbH850JkfRrnz4RGe"

	CORRELATION_ID = "EOS-fiWTTmfxAk6W-hQjcUin0A-08vSZf-Sjk6Xbx4FA3ewVg"
	SERVER_NAME    = "eos-gateway"
)

// DeploymentId mirrors the static property EosGatewayServer.DeploymentId.
func DeploymentId() string { return DEPLOYMENT_ID }

//go:embed responses/*.json
var responsesFS embed.FS

var (
	defaultEndpointsPayload stj.Raw // JsonElement re-written by the serializer
	customEndpointsPayload  stj.Raw
)

// ── PUID → display name registry (populated on login) ────────────────────
var (
	userNamesMu sync.Mutex
	userNames   = map[string]string{}
)

func userNamesSet(puid, name string) {
	userNamesMu.Lock()
	userNames[puid] = name
	userNamesMu.Unlock()
}

func userNamesGet(puid string) (string, bool) {
	userNamesMu.Lock()
	defer userNamesMu.Unlock()
	v, ok := userNames[puid]
	return v, ok
}

// ── Dynamic EOS session list — managed via DashboardServer /api/sessions ──

// Sessions mirrors EosGatewayServer.Sessions. Read it through
// SessionsSnapshot (the original's callers used `lock (Sessions) ToList()`).
var Sessions = []*models.EosSessionInfo{}
var sessionLock sync.Mutex

// SessionsSnapshot mirrors `lock (EosGatewayServer.Sessions) { Sessions.ToList(); }`.
func SessionsSnapshot() []*models.EosSessionInfo {
	ensureGatewayStatic()
	sessionLock.Lock()
	defer sessionLock.Unlock()
	return append([]*models.EosSessionInfo{}, Sessions...)
}

// ── Per-station whitelists ───────────────────────────────────────────────
// A station with a non-empty whitelist only appears in the station browser for the accounts on it.
// The values are PUSHED in from the Ares side whenever a station's config changes (see StationAcl).
// Station ids and names are both matched case-insensitively.

type nameSet struct {
	names []string            // first spelling, insertion order
	index map[string]struct{} // upper-cased
}

func (s *nameSet) add(n string) {
	k := strings.ToUpper(n)
	if _, ok := s.index[k]; ok {
		return
	}
	s.index[k] = struct{}{}
	s.names = append(s.names, n)
}

func (s *nameSet) contains(n string) bool { _, ok := s.index[strings.ToUpper(n)]; return ok }

var (
	whitelistMu      sync.Mutex
	stationWhitelist = map[string]*nameSet{} // key: upper-cased station id (OrdinalIgnoreCase)
)

// SetStationWhitelist mirrors EosGatewayServer.SetStationWhitelist(string?, IEnumerable<string>?).
func SetStationWhitelist(stationId string, usernames []string) {
	ensureGatewayStatic()
	if strings.TrimSpace(stationId) == "" {
		return
	}
	set := &nameSet{index: map[string]struct{}{}}
	for _, u := range usernames {
		if strings.TrimSpace(u) != "" {
			set.add(strings.TrimSpace(u))
		}
	}
	whitelistMu.Lock()
	defer whitelistMu.Unlock()
	if len(set.names) == 0 {
		delete(stationWhitelist, strings.ToUpper(stationId))
	} else {
		stationWhitelist[strings.ToUpper(stationId)] = set
	}
}

// WhitelistedStationCount mirrors EosGatewayServer.WhitelistedStationCount.
func WhitelistedStationCount() int {
	ensureGatewayStatic()
	whitelistMu.Lock()
	defer whitelistMu.Unlock()
	return len(stationWhitelist)
}

// base64UrlDecode mirrors EosGatewayServer.Base64UrlDecode.
func base64UrlDecode(s string) []byte {
	t := strings.NewReplacer("-", "+", "_", "/").Replace(s)
	switch len(t) % 4 {
	case 2:
		t += "=="
	case 3:
		t += "="
	}
	b, err := fromBase64String(t)
	if err != nil {
		panic(err)
	}
	return b
}

// IdentifyCaller decodes the caller out of the EOS user token. We minted and
// signed it ourselves (EosJwtFactory), so the payload is trusted here; it is
// only ever used to decide what to SHOW, never to grant anything.
func IdentifyCaller(request *hosting.Request) (Name string, Puid string) {
	name, puid := "", ""
	ok := tryRun(func() {
		header := request.GetHeaderValue("Authorization", "")
		if !hasPrefixFold(header, "Bearer ") {
			return
		}
		parts := strings.Split(strings.TrimSpace(header[7:]), ".")
		if len(parts) < 2 {
			return
		}
		root := stj.MustParse([]byte(utf8GetString(base64UrlDecode(parts[1]))))
		p := ""
		if v, ok := root.TryGetProperty("productUserId"); ok {
			p = strOrEmpty(v.GetString())
		}
		n := ""
		if acc, ok := root.TryGetProperty("account"); ok && acc.ValueKind() == stj.Object {
			if dn, ok := acc.TryGetProperty("displayName"); ok {
				n = strOrEmpty(dn.GetString())
			}
		}
		if n == "" && p != "" {
			if known, ok := userNamesGet(p); ok {
				n = known
			}
		}
		name, puid = n, p
	})
	if !ok {
		return "", ""
	}
	return name, puid
}

// visibleTo: an allowlist fails CLOSED — a station that has one stays hidden
// from a caller we cannot identify. Stations without a whitelist are untouched.
func visibleTo(s *models.EosSessionInfo, name, puid string) bool {
	if s.StationId == nil || strings.TrimSpace(*s.StationId) == "" {
		return true
	}
	whitelistMu.Lock()
	allowed, ok := stationWhitelist[strings.ToUpper(*s.StationId)]
	whitelistMu.Unlock()
	if !ok || len(allowed.names) == 0 {
		return true
	}
	if name != "" && allowed.contains(name) {
		return true
	}
	// The Oculus login derives the puid straight from the display name, so a whitelisted name still
	// matches when the token carries no readable displayName.
	if puid != "" {
		for _, n := range allowed.names {
			if strings.EqualFold(ProductUserIdFor(n), puid) {
				return true
			}
		}
	}
	return false
}

// Runtime-registered sessions are persisted to disk next to the binary and
// reloaded on boot — same pattern as lobbies.json.
func sessionsPath() string { return pathCombine(BaseDirectory, "sessions.json") }

// PersistSessions mirrors EosGatewayServer.PersistSessions (failures are non-fatal).
func PersistSessions() {
	tryRun(func() {
		sessionLock.Lock()
		text := serializeIndented(Sessions)
		sessionLock.Unlock()
		if err := os.WriteFile(sessionsPath(), []byte(text), 0o644); err != nil {
			panic(err)
		}
	})
}

func loadPersistedSessions() {
	tryRun(func() {
		if !fileExists(sessionsPath()) {
			return
		}
		text, err := readAllText(sessionsPath())
		if err != nil {
			panic(err)
		}
		var list []*models.EosSessionInfo
		stj.MustUnmarshal([]byte(text), &list)
		if list == nil {
			return
		}
		func() {
			sessionLock.Lock()
			defer sessionLock.Unlock()
			for _, s := range list {
				id := s.Id // NullReferenceException on a null entry aborts the load
				// upsert — persisted state overrides the baked seed
				Sessions = removeSessions(Sessions, func(x *models.EosSessionInfo) bool { return x.Id == id })
				Sessions = append(Sessions, s)
			}
		}()
		logging.Console(fmt.Sprintf("[EosGatewayServer] Loaded %d persisted sessions from disk", len(list)))
	})
}

// removeSessions mirrors List.RemoveAll (order of survivors kept).
func removeSessions(list []*models.EosSessionInfo, pred func(*models.EosSessionInfo) bool) []*models.EosSessionInfo {
	out := list[:0:0]
	for _, s := range list {
		if !pred(s) {
			out = append(out, s)
		}
	}
	return out
}

// ── Dynamic EOS lobby list — player-hosted rooms ──────────────────────────

// Lobbies mirrors EosGatewayServer.Lobbies; guard every access with LobbyLock.
var Lobbies = []*models.EosLobbySession{}

// LobbyLock mirrors EosGatewayServer.LobbyLock.
var LobbyLock sync.Mutex

func lobbiesPath() string { return pathCombine(BaseDirectory, "lobbies.json") }

// removeLobby mirrors Lobbies.Remove(lobby) (caller holds LobbyLock).
func removeLobby(l *models.EosLobbySession) {
	for i, x := range Lobbies {
		if x == l {
			Lobbies = append(Lobbies[:i:i], Lobbies[i+1:]...)
			return
		}
	}
}

// PersistLobbies mirrors EosGatewayServer.PersistLobbies (takes LobbyLock).
func PersistLobbies() {
	LobbyLock.Lock()
	defer LobbyLock.Unlock()
	persistLobbiesLocked()
}

// persistLobbiesLocked is PersistLobbies for callers already inside
// `lock (LobbyLock)` (C# Monitor is re-entrant; sync.Mutex is not).
func persistLobbiesLocked() {
	tryRun(func() {
		toSave := []*models.EosLobbySession{}
		for _, l := range Lobbies {
			if l.Attributes.ContainsKey("ADDRESS_s") {
				toSave = append(toSave, l)
			}
		}
		if err := os.WriteFile(lobbiesPath(), []byte(serializeIndented(toSave)), 0o644); err != nil {
			panic(err)
		}
	})
}

func loadPersistedLobbies() {
	tryRun(func() {
		if !fileExists(lobbiesPath()) {
			return
		}
		text, err := readAllText(lobbiesPath())
		if err != nil {
			panic(err)
		}
		var list []*models.EosLobbySession
		stj.MustUnmarshal([]byte(text), &list)
		if list == nil {
			return
		}
		withLobbyLock(func() { Lobbies = append(Lobbies, list...) })
		logging.Console(fmt.Sprintf("[EosGatewayServer] Loaded %d persisted lobbies from disk", len(list)))
	})
}

// ── Stale-lobby reaper ────────────────────────────────────────────────────
// Player-hosted lobbies send a "heartbeat" WS frame on an interval (which bumps
// LastUpdated). A lobby that goes silent past LOBBY_TTL is considered dead and
// removed. Infra-managed dedicated-server lobbies (those carrying ADDRESS_s) are
// never reaped.
const (
	lobbyTTL   = 90 * time.Second
	lobbySweep = 30 * time.Second
)

func startLobbyReaper() {
	go func() {
		t := time.NewTicker(lobbySweep)
		defer t.Stop()
		for range t.C {
			reapLobbies(time.Now().UTC())
		}
	}()
}

func reapLobbies(now time.Time) {
	tryRun(func() {
		var expired []*models.EosLobbySession
		withLobbyLock(func() {
			for _, l := range Lobbies {
				if l.Attributes.ContainsKey("ADDRESS_s") {
					continue // infra-managed, never reap
				}
				last, ok := parseUniversal(l.LastUpdated)
				if !ok {
					continue // no parseable timestamp → leave it alone
				}
				if now.Sub(last) > lobbyTTL {
					expired = append(expired, l)
				}
			}
			for _, l := range expired {
				removeLobby(l)
			}
		})

		for _, l := range expired {
			logging.Console(fmt.Sprintf("[EosGatewayServer] Reaped stale lobby %s (silent > %.0fs)", l.Id, lobbyTTL.Seconds()))
			BroadcastAsync(l.Id, stj.O(
				"name", "lobbydeleted",
				"payload", stj.O("lobbyId", l.Id, "name", "lobbydeleted"),
			))
		}

		if len(expired) > 0 {
			PersistLobbies()
		}
	})
}

// parseUniversal mirrors DateTime.TryParse(s, null, AdjustToUniversal |
// AssumeUniversal) for the ISO-8601 shapes the gateway writes.
func parseUniversal(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// ── Public session statics ────────────────────────────────────────────────

// UpsertSession mirrors EosGatewayServer.UpsertSession.
func UpsertSession(session *models.EosSessionInfo) {
	ensureGatewayStatic()
	sessionLock.Lock()
	Sessions = removeSessions(Sessions, func(s *models.EosSessionInfo) bool { return s.Id == session.Id })
	Sessions = append(Sessions, session)
	sessionLock.Unlock()
	PersistSessions()
}

// RemoveSession mirrors EosGatewayServer.RemoveSession.
func RemoveSession(id string) bool {
	ensureGatewayStatic()
	sessionLock.Lock()
	before := len(Sessions)
	Sessions = removeSessions(Sessions, func(s *models.EosSessionInfo) bool { return s.Id == id })
	removed := len(Sessions) < before
	sessionLock.Unlock()
	if removed {
		PersistSessions()
	}
	return removed
}

// RemoveSessionsByDeployment drops every matchmaking session belonging to one
// deployment (watchdog: a dead server never deletes its own session).
// Returns the number removed.
func RemoveSessionsByDeployment(deploymentId string) int {
	ensureGatewayStatic()
	if strings.TrimSpace(deploymentId) == "" {
		return 0
	}
	sessionLock.Lock()
	before := len(Sessions)
	Sessions = removeSessions(Sessions, func(s *models.EosSessionInfo) bool {
		return s.DeploymentId != nil && *s.DeploymentId == deploymentId
	})
	n := before - len(Sessions)
	sessionLock.Unlock()
	if n > 0 {
		PersistSessions()
	}
	return n
}

// PruneSessionsExcept keeps exactly ONE session per live deployment — the LAST
// (newest) one — and drops everything else (duplicates and dead deployments).
// keepDeploymentIds mirrors HashSet<string> (membership = key present).
// Returns the number removed.
func PruneSessionsExcept(keepDeploymentIds map[string]bool) int {
	ensureGatewayStatic()
	sessionLock.Lock()
	before := len(Sessions)
	lastByDep := map[string]*models.EosSessionInfo{}
	for _, s := range Sessions {
		if s.DeploymentId != nil && *s.DeploymentId != "" {
			if _, keep := keepDeploymentIds[*s.DeploymentId]; keep {
				lastByDep[*s.DeploymentId] = s // later entries overwrite -> keeps the newest
			}
		}
	}
	survivors := map[*models.EosSessionInfo]bool{}
	for _, s := range lastByDep {
		survivors[s] = true
	}
	Sessions = removeSessions(Sessions, func(s *models.EosSessionInfo) bool { return !survivors[s] })
	removed := before - len(Sessions)
	sessionLock.Unlock()
	if removed > 0 {
		PersistSessions()
	}
	return removed
}

// ClearSessions wipes every matchmaking session (admin station purge).
// Returns the number removed.
func ClearSessions() int {
	ensureGatewayStatic()
	sessionLock.Lock()
	n := len(Sessions)
	Sessions = []*models.EosSessionInfo{}
	sessionLock.Unlock()
	PersistSessions()
	return n
}

// SetSessionPlayerCount reconciles a session's player count to the value the
// gameserver reports by trimming PublicPlayers (oldest first). Returns true if
// a matching session was found; never fabricates ids.
func SetSessionPlayerCount(deploymentId string, count int) bool {
	ensureGatewayStatic()
	if strings.TrimSpace(deploymentId) == "" {
		return false
	}
	if count < 0 {
		count = 0
	}
	sessionLock.Lock()
	var s *models.EosSessionInfo
	for _, x := range Sessions {
		if x.DeploymentId != nil && *x.DeploymentId == deploymentId {
			s = x
			break
		}
	}
	if s == nil {
		sessionLock.Unlock()
		return false
	}
	for len(s.PublicPlayers) > count {
		s.PublicPlayers = append(s.PublicPlayers[:0:0], s.PublicPlayers[1:]...)
	}
	sessionLock.Unlock()
	PersistSessions()
	return true
}

// ── Static constructor ────────────────────────────────────────────────────

var gatewayStaticOnce sync.Once

// ensureGatewayStatic runs the static constructor once, on first use of any
// EosGatewayServer static (as the CLR does).
func ensureGatewayStatic() { gatewayStaticOnce.Do(gatewayStaticInit) }

func gatewayStaticInit() {
	defaultEndpointsPayload = mustCompact(readResource("default_endpoints.json"))
	customEndpointsPayload = mustCompact(readResource("custom_endpoints.json"))

	// Seed Sessions from matchmaking.json
	tryRun(func() {
		root := stj.MustParse([]byte(readResource("matchmaking.json")))
		arr, ok := root.TryGetProperty("sessions")
		if !ok {
			return
		}
		for _, s := range arr.EnumerateArray() {
			id := ""
			if v, ok := s.TryGetProperty("id"); ok {
				id = strOrEmpty(v.GetString())
			}
			deploymentId := ""
			if dv, ok := s.TryGetProperty("deployment"); ok {
				deploymentId = strOrEmpty(dv.GetString())
			}
			bucket := "A2_4729"
			if b, ok := s.TryGetProperty("bucket"); ok {
				if p := b.GetString(); p != nil {
					bucket = *p
				}
			}
			maxP := 10
			if settings, ok := s.TryGetProperty("settings"); ok {
				if mp, ok := settings.TryGetProperty("maxPublicPlayers"); ok {
					maxP = int(mp.GetInt32())
				}
			}
			svrName, ip, port, imguiPort := "", "", "", ""
			buildId := 4729
			if attrs, ok := s.TryGetProperty("attributes"); ok {
				if sn, ok := attrs.TryGetProperty("SERVERNAME_s"); ok {
					svrName = strOrEmpty(sn.GetString())
				}
				if ad, ok := attrs.TryGetProperty("ADDRESS_s"); ok {
					ip = strOrEmpty(ad.GetString())
				}
				if pp, ok := attrs.TryGetProperty("PUBLICPORT_s"); ok {
					port = strOrEmpty(pp.GetString())
				}
				if bi, ok := attrs.TryGetProperty("BUILDUNIQUEID_l"); ok {
					buildId = int(bi.GetInt32())
				}
				if ig, ok := attrs.TryGetProperty("IMGUIPORT_s"); ok {
					imguiPort = strOrEmpty(ig.GetString())
				}
			}
			publicPlayers := []string{}
			if ppArr, ok := s.TryGetProperty("publicPlayers"); ok {
				for _, p := range ppArr.EnumerateArray() {
					if pid := p.GetString(); pid != nil {
						publicPlayers = append(publicPlayers, *pid)
					}
				}
			}
			info := models.NewEosSessionInfo()
			info.Id = id
			if deploymentId != "" {
				d := deploymentId
				info.DeploymentId = &d
			} else {
				info.DeploymentId = nil
			}
			info.ServerName, info.IpAddress, info.Port = svrName, ip, port
			info.MaxPlayers, info.Bucket, info.BuildId = maxP, bucket, buildId
			info.ImguiPort, info.PublicPlayers = imguiPort, publicPlayers
			sessionLock.Lock()
			Sessions = append(Sessions, info)
			sessionLock.Unlock()
		}
	})

	// Reload runtime-registered sessions saved before the last restart (overrides the seed by id).
	loadPersistedSessions()

	loadPersistedLobbies()
	startLobbyReaper()
}

// readResource mirrors ReadResource ("{}" when the resource is missing).
func readResource(filename string) string {
	b, err := responsesFS.ReadFile("responses/" + filename)
	if err != nil {
		return "{}"
	}
	return utf8GetString(b)
}

func mustCompact(json string) stj.Raw {
	b, err := models.CompactElement(stj.MustParse([]byte(json)))
	if err != nil {
		panic(err)
	}
	return stj.Raw(b)
}

// ── Server ─────────────────────────────────────────────────────────────────

// EosGatewayServer mirrors the C# server class (HTTP + WebSocket on :50).
type EosGatewayServer struct {
	HTTP *hosting.Server
}

// NewEosGatewayServer builds the server, registers its endpoints in the
// original reflection order, installs the WebSocket hook and starts listening
// (the original constructor started the listener).
func NewEosGatewayServer() (*EosGatewayServer, error) {
	s := newEosGatewayServer()
	if err := s.HTTP.Start(); err != nil {
		return nil, err
	}
	return s, nil
}

func newEosGatewayServer() *EosGatewayServer {
	ensureGatewayStatic()
	s := &EosGatewayServer{HTTP: hosting.NewServer("EosGatewayServer", gatewayHostname, gatewayPort)}
	h := s.HTTP
	h.Handle("POST", "/auth/v1/accounts/deviceid", "DeviceIdLogin", s.DeviceIdLogin)
	h.Handle("POST", "/auth/v1/oauth/token", "OAuthToken", s.OAuthToken)
	h.Handle("GET", "/auth/v1/oauth/.well-known/jwks.json", "Jwks", s.Jwks)
	h.Handle("POST", "/auth/v1/turn/credentials", "TurnCredentials", s.TurnCredentials)
	h.Handle("GET", "/rtc/v1/{deployment}/room/{roomName}", "RtcRoomToken", s.RtcRoomToken)
	h.Handle("GET", "/sdk/v1/default", "DefaultEndpoints", s.DefaultEndpoints)
	h.Handle("GET", "/sdk/v1/product/{product_id}", "CustomEndpoints", s.CustomEndpoints)
	h.Handle("POST", "/matchmaking/v1/{deployment_id}/filter", "MatchmakingFilter", s.MatchmakingFilter)
	h.Handle("POST", "/lobby/v1/{deployment_id}/lobbies/filter", "LobbyFilter", s.LobbyFilter)
	h.Handle("POST", "/lobby/v1/{deployment_id}/lobbies", "LobbyCreate", s.LobbyCreate)
	h.Handle("GET", "/lobby/v1/{deployment_id}/lobbies/{lobby_id}", "LobbyGet", s.LobbyGet)
	h.Handle("PUT", "/lobby/v1/{deployment_id}/lobbies/{lobby_id}", "LobbyUpdate", s.LobbyUpdate)
	h.Handle("POST", "/lobby/v1/{deployment_id}/lobbies/{lobby_id}/members/{product_user_id}", "LobbyJoin", s.LobbyJoin)
	h.Handle("DELETE", "/lobby/v1/{deployment_id}/lobbies/{lobby_id}/members/{product_user_id}", "LobbyLeave", s.LobbyLeave)
	h.Handle("POST", "/matchmaking/v1/{deployment_id}/sessions/{session_id}/publicplayers/{product_user_id}", "JoinSession", s.JoinSession)
	h.Handle("DELETE", "/matchmaking/v1/{deployment_id}/sessions/{session_id}/publicplayers/{product_user_id}", "LeaveSession", s.LeaveSession)
	h.Handle("POST", "/user/v3/product-users/search", "ProductUserSearch", s.ProductUserSearch)
	h.Handle("POST", "/telemetry/data/datarouter/api/v1/public/data", "Telemetry", s.Telemetry)
	h.Handle("POST", "/datarouter/api/v1/public/data/clients", "DataRouterEvent", s.DataRouterEvent)
	h.Handle("GET", "/datastorage/v2/access/deployment/{deployment_id}/private/{product_user_id}/{filename}", "PlayerDataStorageAccess", s.PlayerDataStorageAccess)
	h.Handle("GET", "/datastorage/v2/data/deployment/{deployment_id}/private/{product_user_id}/{filename}", "PlayerDataStorageGet", s.PlayerDataStorageGet)
	h.Handle("PUT", "/datastorage/v2/data/deployment/{deployment_id}/private/{product_user_id}/{filename}", "PlayerDataStoragePut", s.PlayerDataStoragePut)
	h.Handle("POST", "/epic/presence/internal/v1/_/{account_id}/presence/{connection_id}", "UpdatePresence", s.UpdatePresence)
	h.WebSocket = s.HandleWebSocketAsync
	return s
}
