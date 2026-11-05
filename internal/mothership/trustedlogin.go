package mothership

import (
	"fmt"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

// trustedServerPlayerToken answers a trusted game server's QUEST login whose UserId is already a Mothership
// player id (the DLL knows players by that id, not by their Meta id). It returns a token for that existing
// player in the normal CompleteQuestAuth reply shape and writes nothing. ok=false: not a known player id, so
// the caller continues with the ordinary QUEST flow.
//
// Only the record whose PlayerId is userId counts -- never one whose ExternalProviderId is userId, because
// before this fix every such server login created a junk QUEST player keyed by the Mothership id.
func trustedServerPlayerToken(cl *docdb.Collection[models.MothershipV2PlayerDbObject], userId string) (hosting.Result, bool) {
	if userId == "" {
		return nil, false
	}
	var p *models.MothershipV2PlayerDbObject
	for _, cand := range cl.FindAll() {
		if eqS(cand.PlayerId, userId) && cand.IsPrimaryId {
			p = cand
			break
		}
	}
	if p == nil {
		for _, cand := range cl.FindAll() {
			if eqS(cand.PlayerId, userId) {
				p = cand
				break
			}
		}
	}
	if p == nil {
		return nil, false
	}
	service, extId := sv(p.ExternalService), sv(p.ExternalProviderId)
	if service == "" {
		service = "QUEST"
	}
	now := time.Now()
	token := generateJwt(stjObjForPlayerToken(userId, service, extId, now))
	reply := *p // copy: the stored record keeps its own token
	reply.Token = strp(token)
	reply.ExpirationTime = now.Add(time.Hour).UnixMilli()
	authLog(fmt.Sprintf("[QUEST-ATT] trusted server signed in as existing player %s (%s)", userId, service))
	return jsonT(&reply, 201), true
}

// stjObjForPlayerToken is CompleteQuestAuth's JWT payload for an existing player (same fields and order).
func stjObjForPlayerToken(playerId, service, extId string, now time.Time) stj.Obj {
	return stj.O(
		"sub", playerId,
		"did", newId(),
		"env", ENVIRONMENT_ID,
		"externalService", service,
		"externalServiceId", extId,
		"tid", TENANT_ID,
		"tags", nil,
		"orgScopedExternalServiceId", extId,
		"nbf", now.Unix(),
		"exp", now.Add(time.Hour).Unix(),
		"iat", now.Unix(),
	)
}
