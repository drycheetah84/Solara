// Port of AUnrealFeatures.EOSSDK/LobbyWsHelper.cs (EOS lobby WebSocket protocol).
package eos

import (
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"

	"solara/internal/dotnet"
	"solara/internal/eos/models"
	"solara/internal/hosting"
	"solara/internal/stj"
)

const dummyPuid = "0002000000000000000000000000000000"

// lobbyId → set of connected WebSockets (for broadcasting push events).
// ConcurrentBag: entries are never removed; closed sockets are skipped.
var (
	lobbyConnsMu sync.Mutex
	lobbyConns   = map[string][]*WebSocket{}
)

func addLobbyConnection(lobbyId string, ws *WebSocket) {
	lobbyConnsMu.Lock()
	lobbyConns[lobbyId] = append(lobbyConns[lobbyId], ws)
	lobbyConnsMu.Unlock()
}

// IsLobbyPath mirrors LobbyWsHelper.IsLobbyPath.
func IsLobbyPath(path string) bool {
	return hasPrefixFold(path, "/lobby/v1/") && hasSuffixFold(path, "/lobbies/connect")
}

// withLobbyLock runs f under LobbyLock (released on panic, like `lock`).
func withLobbyLock(f func()) {
	LobbyLock.Lock()
	defer LobbyLock.Unlock()
	f()
}

func findLobby(id string) *models.EosLobbySession {
	for _, l := range Lobbies {
		if l.Id == id {
			return l
		}
	}
	return nil
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// LobbyWsHelperRunAsync mirrors LobbyWsHelper.RunAsync (renamed: StompHelper
// also has a static RunAsync).
func LobbyWsHelperRunAsync(ws *WebSocket, path string, ownerPuid string, log func(string)) {
	buf := make([]byte, 64*1024)
	var joinedLobbyId *string

	defer func() {
		// finally: ConcurrentBag doesn't support removal; the connection is
		// detected as closed on the next broadcast attempt.
		_ = joinedLobbyId
	}()
	defer func() {
		// catch (Exception ex) when (ex is not OperationCanceledException)
		if p := recover(); p != nil {
			log("LOBBY-WS error | " + hosting.ExceptionMessage(p))
		}
	}()

	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}

	for ws.State() == WsOpen {
		n, mt, err := ws.Receive(buf)
		must(err)
		if mt == WsClose {
			break
		}

		raw := strings.TrimSpace(utf8GetString(buf[:n]))
		if raw == "" {
			continue
		}

		log("LOBBY-WS IN | " + utf16Prefix(raw, min(utf16Len(raw), 200)))

		name, requestId := "", ""
		var payload stj.Element
		if !tryRun(func() {
			root := stj.MustParse([]byte(raw))
			if n, ok := root.TryGetProperty("name"); ok {
				name = strOrEmpty(n.GetString())
			} else {
				name = ""
			}
			if r, ok := root.TryGetProperty("requestId"); ok {
				requestId = strOrEmpty(r.GetString())
			} else {
				requestId = ""
			}
			if rawPayload, ok := root.TryGetProperty("payload"); ok {
				payload = rawPayload
			}
		}) {
			must(sendError(ws, "bad_request", "could not parse json", requestId, 400))
			continue
		}

		isObj := payload.ValueKind() == stj.Object

		switch strings.ToLower(name) {
		case "create":
			// Extract settings from payload.options
			var opts stj.Element
			if isObj {
				if o, ok := payload.TryGetProperty("options"); ok {
					opts = o
				}
			}
			optObj := opts.ValueKind() == stj.Object
			optBool := func(key string) bool {
				if !optObj {
					return false
				}
				v, ok := opts.TryGetProperty(key)
				return ok && v.GetBoolean()
			}

			settings := models.NewEosLobbySettings()
			settings.MaxPublicPlayers = 10
			if optObj {
				if mp, ok := opts.TryGetProperty("maxPlayers"); ok {
					settings.MaxPublicPlayers = int(mp.GetInt32())
				}
			}
			settings.ShouldAdvertise = optBool("shouldAdvertise")
			settings.AllowJoinViaPresence = optBool("allowJoinViaPresence")
			settings.AllowReadById = optBool("allowReadById")
			settings.AllowInvites = optBool("allowInvites")

			lobby := models.NewEosLobbySession()
			lobby.Deployment = DeploymentId()
			lobby.Id = dotnet.NewGuidN()
			lobby.Bucket = ""
			lobby.Settings = settings
			lobby.TotalPlayers = 1
			lobby.OpenPublicPlayers = settings.MaxPublicPlayers - 1
			lobby.PublicPlayers = []string{ownerPuid}
			lobby.Started = false
			lobby.LastUpdated = utcNowString()
			lobby.Attributes = models.NewDict[*string]()
			lobby.Owner = ownerPuid
			lobby.OwnerPlatformId = 4000
			lobby.MemberData = models.NewDict[*models.EosLobbyMemberEntry]()
			me := models.NewEosLobbyMemberEntry()
			me.PlatformId, me.Platform = 4000, "steam"
			lobby.MemberData.Set(ownerPuid, me)

			var publicData, memberDataMap stj.Raw
			withLobbyLock(func() {
				Lobbies = append(Lobbies, lobby)
			})

			id := lobby.Id
			joinedLobbyId = &id
			addLobbyConnection(id, ws)

			// Snapshots are taken under LobbyLock in Go (concurrent map access
			// is fatal in Go); the bytes equal the original's.
			withLobbyLock(func() {
				lobby.Lock = dotnet.NewGuidN()
				publicData = stj.Raw(stj.MustMarshal(lobby))
				memberDataMap = stj.Raw(stj.MustMarshal(buildMemberDataMap(lobby)))
			})
			privateData := stj.O(
				"index", rand.IntN(math.MaxInt32),
				"lock", lobby.Lock,
				"invites", []any{},
				"historicalPlayers", []string{ownerPuid},
				"pendingDelete", nil,
			)

			roomName0 := RoomName(lobby.Id)
			must(sendJson(ws, stj.O(
				"name", "lobbyinfo",
				"payload", stj.O(
					"publicData", publicData,
					"privateData", privateData,
					"memberData", stj.O("memberData", memberDataMap),
					"room", nil, // Pavlov uses P2P voice (no SFU/conference room) — matches real Epic
					"requestId", requestId,
					"name", "lobbyinfo",
				),
				"requestId", requestId,
			)))
			log("LOBBY-WS created | lobbyId=" + lobby.Id + " owner=" + ownerPuid + " rtcRoom=" + roomName0)

		case "join":
			var lobbyId *string
			if isObj {
				if li, ok := payload.TryGetProperty("lobbyId"); ok {
					lobbyId = li.GetString()
				}
			}

			if lobbyId == nil {
				must(sendError(ws, "bad_request", "missing lobbyId", requestId, 400))
				break
			}

			var publicData, memberDataMap stj.Raw
			var roomName1 *string

			withLobbyLock(func() {
				lobby := findLobby(*lobbyId)
				if lobby != nil {
					// Add joining player to shared state
					if !containsString(lobby.PublicPlayers, ownerPuid) {
						lobby.PublicPlayers = append(lobby.PublicPlayers, ownerPuid)
						lobby.TotalPlayers = len(lobby.PublicPlayers)
						lobby.OpenPublicPlayers = max(0, lobby.Settings.MaxPublicPlayers-lobby.TotalPlayers)
						lobby.LastUpdated = utcNowString()
					}

					if !lobby.MemberData.ContainsKey(ownerPuid) {
						entry := models.NewEosLobbyMemberEntry()
						entry.PlatformId, entry.Platform = 4000, "other"
						if knownName, ok := userNamesGet(ownerPuid); ok {
							entry.Data.Set("DISPLAYNAME_s", knownName)
						}
						lobby.MemberData.Set(ownerPuid, entry)
					}

					// First real player to join a community lobby becomes the owner
					if lobby.Owner == dummyPuid && ownerPuid != dummyPuid {
						lobby.Owner = ownerPuid
						lobby.LastUpdated = utcNowString()
						log("LOBBY-WS ownership transferred | lobbyId=" + *lobbyId + " newOwner=" + ownerPuid)
					}

					log("LOBBY-WS join | lobbyId=" + *lobbyId + " joiner=" + ownerPuid + " players=[" +
						strings.Join(lobby.PublicPlayers, ",") + "] total=" + strconv.Itoa(lobby.TotalPlayers))

					publicData = stj.Raw(stj.MustMarshal(lobby))
					memberDataMap = stj.Raw(stj.MustMarshal(buildMemberDataMap(lobby)))
					rn := RoomName(lobby.Id)
					roomName1 = &rn
				}
			})

			if memberDataMap == nil {
				must(sendError(ws, "session_not_found", "", requestId, 404))
				break
			}

			joinedLobbyId = lobbyId
			addLobbyConnection(*lobbyId, ws)

			// Send lobbyinfo to the joining client
			must(sendJson(ws, stj.O(
				"name", "lobbyinfo",
				"payload", stj.O(
					"publicData", publicData,
					"privateData", nil,
					"memberData", stj.O("memberData", memberDataMap),
					"room", stj.O("roomName", *roomName1, "clientBaseUrl", SERVER_URL, "token", MakeToken(*roomName1, ownerPuid)),
					"requestId", requestId,
					"name", "lobbyinfo",
				),
				"requestId", requestId,
			)))

			// Broadcast memberjoined so the owner (and others) see the new player
			BroadcastAsync(*lobbyId, stj.O(
				"name", "memberjoined",
				"payload", stj.O("lobbyId", *lobbyId, "puid", ownerPuid, "platformId", 4000, "platform", "steam", "allowCrossplay", true, "name", "memberjoined"),
			))

			// If ownership was just transferred, broadcast it
			{
				var isNewOwner bool
				withLobbyLock(func() {
					l := findLobby(*lobbyId)
					isNewOwner = l != nil && l.Owner == ownerPuid && ownerPuid != dummyPuid
				})
				if isNewOwner {
					BroadcastAsync(*lobbyId, stj.O(
						"name", "lobbyownerchange",
						"payload", stj.O("lobbyId", *lobbyId, "newOwner", ownerPuid, "name", "lobbyownerchange"),
					))
				}
			}

			log("LOBBY-WS joined | lobbyId=" + *lobbyId + " joiner=" + ownerPuid + " rtcRoom=" + *roomName1)

		case "lobbydata":
			if !isObj {
				break
			}
			var lobbyId *string
			if li, ok := payload.TryGetProperty("lobbyId"); ok {
				lobbyId = li.GetString()
			}
			var lobbyData stj.Element
			if ld, ok := payload.TryGetProperty("lobbyData"); ok {
				lobbyData = ld
			}

			if lobbyId == nil || lobbyData.ValueKind() != stj.Object {
				must(sendError(ws, "bad_request", "missing lobbyId or lobbyData", requestId, 400))
				break
			}

			var updated *models.Dict[*string]
			var updatedRaw stj.Raw
			withLobbyLock(func() {
				lobby := findLobby(*lobbyId)
				if lobby != nil {
					for _, prop := range lobbyData.EnumerateObject() {
						lobby.Attributes.Set(prop.Name, prop.Value.GetString())
					}
					lobby.LastUpdated = utcNowString()
					c := lobby.Attributes.Clone()
					updated = &c
					updatedRaw = stj.Raw(stj.MustMarshal(c))
				}
			})

			if updated == nil {
				must(sendError(ws, "session_not_found", "", requestId, 404))
				break
			}

			must(sendSuccess(ws, requestId))

			// Push lobbydatachange to all connected clients for this lobby
			BroadcastAsync(*lobbyId, stj.O(
				"name", "lobbydatachange",
				"payload", stj.O("lobbyId", *lobbyId, "attributes", updatedRaw, "name", "lobbydatachange"),
			))
			log("LOBBY-WS lobbydata | lobbyId=" + *lobbyId + " attrs=" + strconv.Itoa(updated.Count()))

		case "memberdata":
			if !isObj {
				break
			}
			var lobbyId *string
			if li, ok := payload.TryGetProperty("lobbyId"); ok {
				lobbyId = li.GetString()
			}
			var dataEl stj.Element
			if de, ok := payload.TryGetProperty("data"); ok {
				dataEl = de
			}

			if lobbyId == nil {
				must(sendError(ws, "bad_request", "missing lobbyId", requestId, 400))
				break
			}

			var entry *models.EosLobbyMemberEntry
			var memberDataRaw stj.Raw
			withLobbyLock(func() {
				lobby := findLobby(*lobbyId)
				if lobby != nil {
					var ok bool
					if entry, ok = lobby.MemberData.TryGetValue(ownerPuid); !ok {
						entry = models.NewEosLobbyMemberEntry()
						lobby.MemberData.Set(ownerPuid, entry)
					}

					if dataEl.ValueKind() == stj.Object {
						for _, prop := range dataEl.EnumerateObject() {
							var v any
							switch prop.Value.ValueKind() {
							case stj.True:
								v = true
							case stj.False:
								v = false
							case stj.Number:
								if i, ok := prop.Value.TryGetInt64(); ok {
									v = i
								} else {
									v = prop.Value.GetDouble()
								}
							default:
								if s := prop.Value.GetString(); s != nil {
									v = *s
								} else {
									v = nil
								}
							}
							entry.Data.Set(prop.Name, v)
						}
					}
					memberDataRaw = stj.Raw(stj.MustMarshal(stj.O("data", entry.Data, "platformId", entry.PlatformId, "platform", entry.Platform)))
				}
			})

			if entry == nil {
				must(sendError(ws, "session_not_found", "", requestId, 404))
				break
			}

			must(sendSuccess(ws, requestId))

			BroadcastAsync(*lobbyId, stj.O(
				"name", "memberdatachange",
				"payload", stj.O(
					"lobbyId", *lobbyId,
					"puid", ownerPuid,
					"memberData", memberDataRaw,
					"name", "memberdatachange",
				),
			))
			log("LOBBY-WS memberdata | lobbyId=" + *lobbyId + " puid=" + ownerPuid)

		case "heartbeat":
			var lobbyId *string
			if isObj {
				if li, ok := payload.TryGetProperty("lobbyId"); ok {
					lobbyId = li.GetString()
				}
			}
			if lobbyId != nil {
				withLobbyLock(func() {
					if lobby := findLobby(*lobbyId); lobby != nil {
						lobby.LastUpdated = utcNowString()
					}
				})
			}
			must(sendSuccess(ws, requestId))

		case "kick":
			if !isObj {
				break
			}
			var lobbyId, targetPuid *string
			if li, ok := payload.TryGetProperty("lobbyId"); ok {
				lobbyId = li.GetString()
			}
			if tp, ok := payload.TryGetProperty("puid"); ok {
				targetPuid = tp.GetString()
			}

			if lobbyId == nil || targetPuid == nil {
				must(sendError(ws, "bad_request", "missing lobbyId or puid", requestId, 400))
				break
			}

			withLobbyLock(func() {
				lobby := findLobby(*lobbyId)
				if lobby != nil && lobby.Owner == ownerPuid {
					lobby.PublicPlayers = removeFirst(lobby.PublicPlayers, *targetPuid)
					lobby.MemberData.Remove(*targetPuid)
					lobby.TotalPlayers = len(lobby.PublicPlayers)
					lobby.OpenPublicPlayers = max(0, lobby.Settings.MaxPublicPlayers-lobby.TotalPlayers)
					lobby.LastUpdated = utcNowString()
				}
			})

			must(sendSuccess(ws, requestId))

			BroadcastAsync(*lobbyId, stj.O(
				"name", "memberkicked",
				"payload", stj.O("lobbyId", *lobbyId, "puid", *targetPuid, "name", "memberkicked"),
			))

			log("LOBBY-WS kick | lobbyId=" + *lobbyId + " target=" + *targetPuid + " by=" + ownerPuid)

		case "delete":
			var lobbyId *string
			if isObj {
				if li, ok := payload.TryGetProperty("lobbyId"); ok {
					lobbyId = li.GetString()
				}
			}

			if lobbyId != nil {
				withLobbyLock(func() {
					if lobby := findLobby(*lobbyId); lobby != nil {
						removeLobby(lobby)
					}
				})
				BroadcastAsync(*lobbyId, stj.O(
					"name", "lobbydeleted",
					"payload", stj.O("lobbyId", *lobbyId, "name", "lobbydeleted"),
				))
			}

			must(sendSuccess(ws, requestId))
			log("LOBBY-WS delete | lobbyId=" + strOrEmpty(lobbyId))
			return

		case "leave":
			must(sendSuccess(ws, requestId))
			return

		default:
			must(sendError(ws, "bad_request", "unknown frameType '"+name, requestId, 400))
		}
	}
}

// BroadcastAsync mirrors LobbyWsHelper.BroadcastAsync: the frame is sent to
// every socket registered for the lobby that is still open; send failures are
// ignored. (Runs synchronously: the original's fire-and-forget task completed
// its sends inline.)
func BroadcastAsync(lobbyId string, frame any) {
	lobbyConnsMu.Lock()
	bag, ok := lobbyConns[lobbyId]
	bag = append([]*WebSocket(nil), bag...)
	lobbyConnsMu.Unlock()
	if !ok {
		return
	}
	bytes := stj.MustMarshal(frame)
	// ConcurrentBag enumerates most-recently-added first
	for i := len(bag) - 1; i >= 0; i-- {
		ws := bag[i]
		tryRun(func() {
			if ws.State() == WsOpen {
				_ = ws.SendText(bytes)
			}
		})
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────

func buildMemberDataMap(lobby *models.EosLobbySession) stj.Obj {
	m := stj.Obj{}
	for _, puid := range lobby.PublicPlayers {
		entry, _ := lobby.MemberData.TryGetValue(puid)
		var data any = models.NewDict[any]()
		platformId, platform := 4000, "other"
		if entry != nil {
			data, platformId, platform = entry.Data, entry.PlatformId, entry.Platform
		}
		m.Set(puid, stj.O("data", data, "platformId", platformId, "platform", platform))
	}
	return m
}

func sendSuccess(ws *WebSocket, requestId string) error {
	return sendJson(ws, stj.O("name", "success", "payload", stj.O("requestId", requestId, "name", "success"), "requestId", requestId))
}

func sendError(ws *WebSocket, code, message, requestId string, statusCode int) error {
	return sendJson(ws, stj.O(
		"name", "error",
		"payload", stj.O("statusCode", statusCode, "errorCode", "errors.com.epicgames.lobbies."+code, "errorMessage", message, "requestId", requestId, "name", "error"),
		"requestId", requestId,
	))
}

func sendJson(ws *WebSocket, obj any) error {
	bytes := stj.MustMarshal(obj)
	if ws.State() == WsOpen {
		return ws.SendText(bytes)
	}
	return nil
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// removeFirst mirrors List<string>.Remove (first occurrence).
func removeFirst(list []string, s string) []string {
	for i, x := range list {
		if x == s {
			return append(list[:i:i], list[i+1:]...)
		}
	}
	return list
}
