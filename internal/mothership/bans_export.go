package mothership

import (
	"sort"

	"solara/internal/dotnet"
	"solara/internal/mothership/models"
)

// ActiveBan is one still-running Mothership ban (in-game moderation panel / server bans) for the dashboard.
type ActiveBan struct {
	BanId     string          `json:"ban_id"`
	PlayerId  string          `json:"player_id"`
	MetaId    string          `json:"meta_id"`
	Username  string          `json:"username"`
	Category  int             `json:"category"`
	Reason    string          `json:"reason"`
	Minutes   int             `json:"duration_minutes"`
	Source    string          `json:"source"`
	CreatedAt dotnet.DateTime `json:"created_at"`
	ExpiresAt dotnet.DateTime `json:"expires_at"`
}

// ActiveBans lists every Mothership ban that has not expired yet, newest first, with the player's platform
// account (Meta id + username) when a v2 player links it.
func ActiveBans() []ActiveBan {
	now := dotnet.UtcNow()
	out := []ActiveBan{}
	for _, b := range bans().Find(func(b *models.MothershipBanDbObject) bool { return b.ExpiresAt.Time().After(now.Time()) }) {
		ab := ActiveBan{BanId: sv(b.BanId), PlayerId: sv(b.PlayerId), Category: b.Category, Reason: sv(b.Reason),
			Minutes: b.DurationMinutes, Source: sv(b.Metadata), CreatedAt: b.CreatedAt, ExpiresAt: b.ExpiresAt}
		if p := v2Players().FindOne(func(p *models.MothershipV2PlayerDbObject) bool { return dbEq(p.PlayerId, ab.PlayerId) }); p != nil {
			ab.MetaId, ab.Username = sv(p.ExternalProviderId), sv(p.ExternalProviderUsername)
		}
		out = append(out, ab)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// LiftBans ends every still-active ban of the Mothership player now (ExpiresAt = now; records are kept) and
// returns how many it lifted.
func LiftBans(playerId string) int {
	now := dotnet.UtcNow()
	cl, n := bans(), 0
	ids := append([]string{playerId}, resolveBanTargets(playerId)...) // Solara: a Meta id lifts its linked players
	for _, b := range cl.Find(func(b *models.MothershipBanDbObject) bool { return containsStr(ids, sv(b.PlayerId)) }) {
		if b.ExpiresAt.Time().After(now.Time()) { // by instant: DB dates come back Local-kind
			b.ExpiresAt = now
			cl.Update(b)
			n++
		}
	}
	InvalidateBanCache()
	return n
}
