package mothership

import "solara/internal/mothership/models"

// serverLoginRealPlayer maps the subject of a game-server login back to the player it was made for.
//
// The DLL signs in AS a player through QUEST auth with that player's Mothership PlayerId as the "Meta" UserId,
// so the login lands on a separate record: ExternalService QUEST, ExternalProviderId = the real PlayerId, and a
// PlayerId of its own. The DLL keeps its own per-player data there (player_inventory saves) and must keep doing
// so: it must never be redirected to the client's records. Only reads that need the real player's account (their
// inventory) use this. Anything that is not such a record is returned unchanged.
func serverLoginRealPlayer(sub string) string {
	if sub == "" {
		return sub
	}
	players := v2Players().FindAll()
	var rec *models.MothershipV2PlayerDbObject
	for _, p := range players {
		if eqS(p.PlayerId, sub) && eqS(p.ExternalService, "QUEST") {
			rec = p
			break
		}
	}
	if rec == nil {
		return sub
	}
	real := sv(rec.ExternalProviderId)
	if real == "" || real == sub {
		return sub
	}
	for _, p := range players {
		if p != rec && eqS(p.PlayerId, real) {
			return real
		}
	}
	if col(models.NewMothershipPlayerDbObject).FindOne(func(p *models.MothershipPlayerDbObject) bool {
		return dbEq(p.MothershipPlayerId, real)
	}) != nil {
		return real
	}
	return sub
}
