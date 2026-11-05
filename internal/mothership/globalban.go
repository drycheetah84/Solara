package mothership

// Solara addition: global bans.
//
// A still-active MothershipBanDbObject (in-game moderation panel, a game server, the dashboard) is a GLOBAL ban:
//   - every client login refuses to mint a token (Quest complete, Rift, Steam, the v1 session logins), answering
//     with the ban in the Mothership error shape the client parses (banDenied);
//   - every existing token stops validating (getUserIdFromToken), so a session dies the moment the ban lands.
// Bans are managed on the dashboard (Users page, "In-game bans": ban + unban).
// Calls from our own game servers (isTrustedServer) are exempt: the DLL logs in as a player to read their data.
//
// Bans are keyed by Mothership player id; a dashboard ban may name a Meta (platform) id instead, which is resolved to
// every Mothership player linked to it. Active = ExpiresAt in the future, compared by instant (dates read back from
// the DB come back Local-kind). Permanent bans are stored 100 years out so lifting (ExpiresAt = now) still works.

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

// BanPermanentMinutes is how a permanent ban is stored (100 years).
const BanPermanentMinutes = 100 * 365 * 24 * 60

var (
	banCacheMu sync.Mutex
	banCacheAt time.Time
	banCache   map[string]*models.MothershipBanDbObject // Mothership player id -> the active ban ending last
)

// InvalidateBanCache makes the next lookup re-read the bans collection (call after adding / lifting a ban).
func InvalidateBanCache() {
	banCacheMu.Lock()
	banCacheAt = time.Time{}
	banCacheMu.Unlock()
}

// activeBan returns the player's active ban (nil = none). Re-reads the collection at most every 3 s.
func activeBan(playerId string) *models.MothershipBanDbObject {
	if playerId == "" {
		return nil
	}
	banCacheMu.Lock()
	defer banCacheMu.Unlock()
	if banCache == nil || time.Since(banCacheAt) > 3*time.Second {
		now := time.Now()
		m := map[string]*models.MothershipBanDbObject{}
		for _, b := range bans().FindAll() {
			id := sv(b.PlayerId)
			if id == "" || !b.ExpiresAt.Time().After(now) {
				continue
			}
			if cur := m[id]; cur == nil || b.ExpiresAt.Time().After(cur.ExpiresAt.Time()) {
				m[id] = b
			}
		}
		banCache, banCacheAt = m, time.Now()
	}
	return banCache[playerId]
}

// banDenied is the login refusal. The client's Mothership error parser wants statusCode plus a message object
// holding ClientMessage / TraceId / MothershipErrorCode ("The response was malformed, no ClientMessage, TraceId or
// MothershipErrorCode found"); "error" is kept for our own tools and logs.
func banDenied(b *models.MothershipBanDbObject) hosting.Result {
	text := BanMessage(b)
	authLog(fmt.Sprintf("[BAN] login refused for %s: %s", sv(b.PlayerId), text))
	return jsonAnon(stj.O(
		"statusCode", 403,
		"name", "PlayerBanned",
		"message", stj.O("ClientMessage", text, "TraceId", sv(b.BanId), "MothershipErrorCode", "PLAYER_BANNED"),
		"error", text,
	), 403)
}

// BanMessage is what the banned player is shown: the reason and when it ends.
func BanMessage(b *models.MothershipBanDbObject) string {
	reason := strings.TrimSpace(sv(b.Reason))
	if reason == "" {
		reason = "breaking the community rules"
	}
	left := time.Until(b.ExpiresAt.Time())
	switch {
	case b.DurationMinutes >= BanPermanentMinutes || left > 50*365*24*time.Hour:
		return "You are banned from Halcyon permanently. Reason: " + reason
	case left > 48*time.Hour:
		return fmt.Sprintf("You are banned from Halcyon for %d more days. Reason: %s", int(left.Hours()/24)+1, reason)
	case left > 90*time.Minute:
		return fmt.Sprintf("You are banned from Halcyon for %d more hours. Reason: %s", int(left.Hours())+1, reason)
	default:
		return fmt.Sprintf("You are banned from Halcyon for %d more minutes. Reason: %s", int(left.Minutes())+1, reason)
	}
}

// loginBanCheck is called by every client login once it knows the Mothership player id (and the platform id).
// ok=false: return the result instead of minting a token.
func loginBanCheck(req *hosting.Request, ids ...string) (hosting.Result, bool) {
	if isTrustedServer(req) {
		return nil, true
	}
	for _, id := range ids {
		if b := activeBan(id); b != nil {
			return banDenied(b), false
		}
	}
	return nil, true
}

// resolveBanTargets maps a Mothership player id or a Meta / platform id to the Mothership player ids to ban.
func resolveBanTargets(id string) []string {
	out := []string{}
	add := func(s string) {
		if s != "" && !containsStr(out, s) {
			out = append(out, s)
		}
	}
	for _, p := range v2Players().Find(func(p *models.MothershipV2PlayerDbObject) bool {
		return dbEq(p.PlayerId, id) || dbEq(p.ExternalProviderId, id)
	}) {
		add(sv(p.PlayerId))
	}
	for _, p := range col(models.NewMothershipPlayerDbObject).Find(func(p *models.MothershipPlayerDbObject) bool {
		return dbEq(p.MothershipPlayerId, id) || dbEq(p.ExternalAccountId, id)
	}) {
		add(sv(p.MothershipPlayerId))
	}
	add(id) // always the id as given too (a Mothership id that only shows up as a token subject, or a player not seen yet)
	return out
}

// AddBan bans a player globally (minutes <= 0 = permanent) and returns the Mothership ids banned.
func AddBan(id string, minutes int, reason, source string) ([]string, string) {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 || strings.ContainsAny(id, "\t\r\n") {
		return nil, "player_id required"
	}
	if minutes <= 0 {
		minutes = BanPermanentMinutes
	}
	if minutes > BanPermanentMinutes {
		return nil, "duration is too long"
	}
	reason = strings.Join(strings.Fields(reason), " ")
	if len([]rune(reason)) > 200 {
		return nil, "reason is too long (200 characters max)"
	}
	targets := resolveBanTargets(id)
	for _, pid := range targets {
		b := models.NewMothershipBanDbObject()
		b.BanId = strp(newId())
		b.PlayerId = strp(pid)
		b.Reason = strp(reason)
		b.DurationMinutes = minutes
		b.OrgWide = true
		b.Metadata = strp(source)
		b.CreatedAt = dotnet.UtcNow()
		b.ExpiresAt = dotnet.UtcNow().AddMinutes(float64(minutes))
		bans().Insert(b)
	}
	InvalidateBanCache()
	authLog(fmt.Sprintf("[BAN] %s banned %v for %d min: %s", source, targets, minutes, reason))
	return targets, ""
}
