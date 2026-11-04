// Ports the player-auth endpoints of AUnrealFeatures.AAMothership/MothershipServer.cs:
// UpsertV1Player, ClientAuthInsecure1/2, ClientAuthGoogle/Apple/Quest (disabled),
// ClientAuthRift, ServerAuthorized, AnalyticsBatch, Begin/CompleteQuestAuth,
// Begin/CompleteSteamAuth and VerifyToken.
package mothership

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func v2Players() *docdb.Collection[models.MothershipV2PlayerDbObject] {
	return col(models.NewMothershipV2PlayerDbObject)
}

// upsertV1Player mirrors UpsertV1Player (unreferenced in the original too):
// look up a v1 player by external account id (or create one), refresh its
// session token and persist.
func upsertV1Player(externalId string, service *string, nickname string) *models.MothershipPlayerDbObject {
	c := col(models.NewMothershipPlayerDbObject)
	var existing *models.MothershipPlayerDbObject
	if externalId != "" {
		for _, p := range c.FindAll() {
			if eqS(p.ExternalAccountId, externalId) {
				existing = p
				break
			}
		}
	}
	playerId := newId()
	if existing != nil && existing.MothershipPlayerId != nil {
		playerId = *existing.MothershipPlayerId
	}
	var extId *string
	if externalId != "" {
		extId = &externalId
	}
	token := generateSessionJwt(playerId, service, extId)

	player := existing
	if player == nil {
		player = models.NewMothershipPlayerDbObject()
	}
	player.MothershipPlayerId = strp(playerId)
	player.ExternalAccountId = strp(externalId)
	if nickname != "" || existing == nil {
		player.ExternalAccountNickname = strp(nickname)
	}
	player.ExpirationTime = time.Now().UTC().Add(24 * time.Hour).Unix()
	if existing != nil && existing.Tags != nil {
		player.Tags = existing.Tags
	} else {
		player.Tags = []*string{}
	}
	player.Token = strp(token)

	if existing == nil {
		c.Insert(player)
	} else {
		c.Update(player)
	}
	s := models.NewMothershipSessionDbObject()
	s.Token, s.PlayerId = strp(token), strp(playerId)
	col(models.NewMothershipSessionDbObject).Insert(s)
	return player
}

// DISABLED (2026-08-20): Insecure_1/2 minted a session for any accountId (auth bypass) -> 403.
func (s *MothershipServer) ClientAuthInsecure1(c *hosting.Context) hosting.Result {
	return errorResult("insecure auth disabled", 403)
}

func (s *MothershipServer) ClientAuthInsecure2(c *hosting.Context) hosting.Result {
	return errorResult("insecure auth disabled", 403)
}

// DISABLED (2026-08-20, prod): v1 social routes minted a session for ANY id -> 403.
func (s *MothershipServer) ClientAuthGoogle(c *hosting.Context) hosting.Result {
	return errorResult("auth method disabled", 403)
}

func (s *MothershipServer) ClientAuthApple(c *hosting.Context) hosting.Result {
	return errorResult("auth method disabled", 403)
}

func (s *MothershipServer) ClientAuthQuest(c *hosting.Context) hosting.Result {
	return errorResult("auth method disabled", 403)
}

// ClientAuthRift authenticates a player via Oculus Rift with a fixed external ID.
func (s *MothershipServer) ClientAuthRift(c *hosting.Context) hosting.Result {
	const RIFT_EXTERNAL_ID = "4649330831842445"
	const RIFT_ORG_SCOPED_ID = "4322323827863641"
	request := c.Request

	if RIFT_ENFORCE && !isTrustedServer(request) { // no graph call on RIFT logins until enabled
		body := parseBody(request)
		uid := str(body, "UserId")
		for _, k := range []string{"user_id", "AccountId", "accountId", "platform_id"} {
			if uid == "" {
				uid = str(body, k)
			}
		}
		nonce := str(body, "Nonce")
		for _, k := range []string{"nonce", "MetaNonce", "user_proof_nonce"} {
			if nonce == "" {
				nonce = str(body, k)
			}
		}
		ok, raw := verifyMetaUserNonce(uid, nonce)
		logConsole(fmt.Sprintf("[RIFT-AUTH] uid='%s' nonceLen=%d proofOk=%s enforce=%s raw=%s bodyKeys=[%s]",
			uid, utf16Len(nonce), boolStr(ok), boolStr(RIFT_ENFORCE), raw, strings.Join(body.Keys(), ",")))
		if RIFT_ENFORCE && !ok {
			return jsonAnon(stj.O("error", "rift nonce verification failed", "code", 401), 401)
		}
	}

	// Solara (2026-11-03): every PC client used to be folded into ONE player (the fixed RIFT_EXTERNAL_ID), so all PC
	// players shared an inventory, quests, a name and one Vivox identity -- a second spectator signing in kicked
	// the others out of voice. The client sends its own Meta UserId (body {UserId, Nonce}); key the player on it.
	// A body without a usable UserId keeps the old shared account.
	extId, orgId := RIFT_EXTERNAL_ID, RIFT_ORG_SCOPED_ID
	{
		b := parseBody(request)
		uid := str(b, "UserId")
		for _, k := range []string{"user_id", "AccountId", "accountId", "platform_id"} {
			if uid == "" {
				uid = str(b, k)
			}
		}
		if riftUserIdOk(uid) {
			extId, orgId = uid, uid
		}
		logConsole(fmt.Sprintf("[RIFT-AUTH] uid='%s' -> externalId=%s", uid, extId))
	}

	cl := v2Players()
	var existing *models.MothershipV2PlayerDbObject
	for _, p := range cl.FindAll() {
		if eqS(p.ExternalProviderId, extId) && eqS(p.ExternalService, "RIFT") {
			existing = p
			break
		}
	}
	playerId := newId()
	if existing != nil && existing.PlayerId != nil {
		playerId = *existing.PlayerId
	}

	now := time.Now()
	expSec := now.Add(time.Hour).Unix()
	expMs := now.Add(time.Hour).UnixMilli()

	token := generateJwt(stj.O(
		"sub", playerId,
		"did", newId(),
		"env", ENVIRONMENT_ID,
		"externalService", "RIFT",
		"externalServiceId", extId,
		"tid", TENANT_ID,
		"tags", nil,
		"orgScopedExternalServiceId", orgId,
		"nbf", now.Unix(),
		"exp", expSec,
		"iat", now.Unix(),
	))

	player := existing
	if player == nil {
		player = models.NewMothershipV2PlayerDbObject()
	}
	player.ExternalProviderId = strp(extId)
	var prevName *string
	if existing != nil {
		prevName = existing.ExternalProviderUsername
	}
	player.ExternalProviderUsername = strp(resolvePlayerName(extId, prevName, "")) // Solara: station-login name; see playername.go
	player.IsPrimaryId = true
	player.PlayerId = strp(playerId)
	player.Tags = nil
	player.Token = strp(token)
	player.ExpirationTime = expMs
	player.ExternalService = strp("RIFT")
	if existing == nil {
		cl.Insert(player)
	} else {
		cl.Update(player)
	}

	// Tier 1 join gate: mark the RIFT ids active so RIFT players aren't kicked.
	markActiveSession(12, extId, orgId, playerId)
	return jsonT(player, 201)
}

// ServerAuthorized is the game-server join gate (Tier 1); server key only.
func (s *MothershipServer) ServerAuthorized(c *hosting.Context) hosting.Result {
	if !isTrustedServer(c.Request) {
		return errorResult("forbidden", 403)
	}
	id := c.Request.GetQueryParameter("id", "")
	// Authorized if a LIVE session (in-memory) OR a PERSISTED player record exists.
	ok := isActiveSession(id) || playerPersisted(id)
	return jsonAnon(stj.O("authorized", ok), 200)
}

// AnalyticsBatch accepts a client analytics event batch (no-op).
func (s *MothershipServer) AnalyticsBatch(c *hosting.Context) hosting.Result {
	return jsonAnon(stj.O("event_id", "not found", "results", []any{}), 200)
}

// BeginQuestAuth begins the Quest v2 auth flow, returns an attestation nonce.
func (s *MothershipServer) BeginQuestAuth(c *hosting.Context) hosting.Result {
	body := parseBody(c.Request)
	userId := str(body, "UserId")
	if userId == "" {
		return errorResult("UserId not found", 404)
	}
	nonce := base64Url(randomBytes(40))
	stateMu.Lock()
	pendingQuestAuth[userId] = nonce
	stateMu.Unlock()
	return jsonAnon(stj.O("AttestationNonce", nonce), 200)
}

// CompleteQuestAuth completes the Quest v2 auth flow and issues a JWT.
func (s *MothershipServer) CompleteQuestAuth(c *hosting.Context) hosting.Result {
	request := c.Request
	body := parseBody(request)
	userId := str(body, "UserId")
	attestationToken := str(body, "AttestationToken")
	metaNonce := str(body, "MetaNonce")

	if userId == "" {
		return errorResult("UserId not found", 404)
	}
	if attestationToken == "" {
		return errorResult("AttestationToken not found", 404)
	}
	if metaNonce == "" {
		return errorResult("MetaNonce not found", 404)
	}
	stateMu.Lock()
	issuedNonce, pending := pendingQuestAuth[userId]
	stateMu.Unlock()
	if !pending {
		authLog(fmt.Sprintf("[QUEST-ATT] DENY userId=%s reason='no pending authentication (complete without a matching begin)'", userId))
		return errorResult("No pending authentication", 404)
	}

	// Trusted-server calls (our DLL) carry the real SERVER_API_KEY and are exempt.
	trustedServer := isTrustedServer(request)

	// Only touch graph.oculus.com when actually enforcing a real client.
	att := attestationResult{Ok: false, Message: "skipped"}
	proofOk, proofRaw := false, "skipped"
	if QUEST_ENFORCE && !trustedServer {
		att = verifyQuestAttestation(attestationToken)
		proofOk, proofRaw = verifyMetaUserNonce(userId, metaNonce)
	}

	authLog(fmt.Sprintf("[QUEST-ATT] userId=%s attOk=%s attMsg=%s ", userId, boolStr(att.Ok), att.Message) +
		fmt.Sprintf("claimNonce=%s metaNonce=%s issuedNonce=%s ", att.Nonce, metaNonce, issuedNonce) +
		fmt.Sprintf("exp=%d ts=%d pkg=%s app=%s dev=%s ", att.Exp, att.Timestamp, att.PackageId, att.AppIntegrity, att.DeviceIntegrity) +
		fmt.Sprintf("patchDays=%d banned=%s uid=%s cert=%s ", att.SecurityPendingDays, boolStr(att.DeviceBanned), att.UniqueId, att.CertDigestsCsv) +
		fmt.Sprintf("proofOk=%s proofRaw=%s enforce=%s trustedServer=%s", boolStr(proofOk), proofRaw, boolStr(QUEST_ENFORCE), boolStr(trustedServer)))

	if QUEST_ENFORCE && !trustedServer {
		deny := func(reason string) hosting.Result {
			authLog(fmt.Sprintf("[QUEST-ATT] DENY userId=%s reason='%s' uid=%s", userId, reason, att.UniqueId))
			return errorResult(reason, 401)
		}

		if !proofOk {
			return deny("userproof nonce invalid")
		}
		// MetaNonce is single-use too.
		if metaNonce == "" || !tryAdd(usedQuestNonces, "mp:"+metaNonce) {
			return deny("userproof nonce replayed")
		}
		if !att.Ok {
			return deny("attestation verify failed: " + att.Message)
		}
		bound := (issuedNonce != "" && att.Nonce == issuedNonce) || (issuedNonce == "" && att.Nonce != "")
		if !bound {
			return deny("attestation nonce mismatch")
		}
		if att.Nonce == "" || !tryAdd(usedQuestNonces, "att:"+att.Nonce) {
			return deny("attestation nonce replayed")
		}
		if att.Exp != 0 && att.Exp < time.Now().Unix() {
			return deny("attestation expired")
		}
		if att.DeviceBanned {
			return deny("device banned (device_ban.is_banned)")
		}
		if att.SecurityPendingDays >= 1 {
			authLog(fmt.Sprintf("[QUEST-ATT] note userId=%s security_update_pending_days=%d (allowed)", userId, att.SecurityPendingDays))
		}

		// TAMPER checks: any failure denies AND fires a 7-day device ban.
		var tamper *string
		if QUEST_EXPECTED_PACKAGE != "" && att.PackageId != QUEST_EXPECTED_PACKAGE {
			tamper = strp("package_id=" + att.PackageId)
		} else if QUEST_REQUIRE_STORE && att.AppIntegrity != "StoreRecognized" {
			tamper = strp("app_integrity=" + att.AppIntegrity)
		} else if QUEST_EXPECTED_CERT_SHA != "" && !containsStr(strings.Split(att.CertDigestsCsv, ","), QUEST_EXPECTED_CERT_SHA) {
			tamper = strp("cert_digest_mismatch")
		} else if QUEST_REQUIRE_DEVICE_BASIC && att.DeviceIntegrity != "Advanced" && att.DeviceIntegrity != "Basic" {
			tamper = strp("device_integrity=" + att.DeviceIntegrity)
		} else if QUEST_REQUIRE_PATCHED && att.SecurityPendingDays >= 1 {
			tamper = strp(fmt.Sprintf("security_update_pending_days=%d", att.SecurityPendingDays))
		}

		if tamper != nil {
			if QUEST_BAN_ON_TAMPER && att.UniqueId != "" {
				banDevice(att.UniqueId, QUEST_BAN_MINUTES, *tamper) // fire-and-forget
			}
			plus := ""
			if QUEST_BAN_ON_TAMPER {
				plus = "+BAN"
			}
			authLog(fmt.Sprintf("[QUEST-ATT] DENY%s userId=%s reason='%s' uid=%s", plus, userId, *tamper, att.UniqueId))
			return errorResult("attestation policy failed: "+*tamper, 401)
		}
	}

	cl := v2Players()
	var existing *models.MothershipV2PlayerDbObject
	for _, p := range cl.FindAll() {
		if eqS(p.ExternalProviderId, userId) && eqS(p.ExternalService, "QUEST") {
			existing = p
			break
		}
	}
	playerId := newId()
	if existing != nil && existing.PlayerId != nil {
		playerId = *existing.PlayerId
	}

	now := time.Now()
	expSec := now.Add(time.Hour).Unix()
	expMs := now.Add(time.Hour).UnixMilli()

	jwtPayload := stj.O(
		"sub", playerId,
		"did", newId(),
		"env", ENVIRONMENT_ID,
		"externalService", "QUEST",
		"externalServiceId", userId,
		"tid", TENANT_ID,
		"tags", nil,
		"orgScopedExternalServiceId", userId,
		"nbf", now.Unix(),
		"exp", expSec,
		"iat", now.Unix(),
	)

	player := existing
	if player == nil {
		player = models.NewMothershipV2PlayerDbObject()
	}
	player.ExternalProviderId = strp(userId)
	var prevQuestName *string
	if existing != nil {
		prevQuestName = existing.ExternalProviderUsername
	}
	// Solara: the real name (station login) instead of rewriting "Player_<id>" on every sign-in; see playername.go.
	player.ExternalProviderUsername = strp(resolvePlayerName(userId, prevQuestName, "Player_"+leftUTF16(userId, 8)))
	player.IsPrimaryId = true
	player.PlayerId = strp(playerId)
	player.Tags = nil
	player.Token = strp(generateJwt(jwtPayload))
	player.ExpirationTime = expMs
	player.ExternalService = strp("QUEST")

	if existing == nil {
		cl.Insert(player)
	} else {
		cl.Update(player)
	}

	// Tier 1 join gate: a genuine client login is now an active session.
	if !trustedServer {
		markActiveSession(12, userId, playerId)
	}

	stateMu.Lock()
	delete(pendingQuestAuth, userId)
	stateMu.Unlock()
	return jsonT(player, 201)
}

// BeginSteamAuth begins the Steam v2 auth flow, returns a nonce.
func (s *MothershipServer) BeginSteamAuth(c *hosting.Context) hosting.Result {
	return jsonAnon(stj.O("Nonce", base64Url(randomBytes(16))), 200)
}

// CompleteSteamAuth completes the Steam v2 auth flow and issues a JWT.
func (s *MothershipServer) CompleteSteamAuth(c *hosting.Context) hosting.Result {
	request := c.Request
	body := parseBody(request)
	nonce := str(body, "Nonce")
	steamTicket := str(body, "SteamTicket")

	if nonce == "" {
		return errorResult("Nonce not found", 404)
	}
	if steamTicket == "" {
		return errorResult("SteamTicket not found", 404)
	}

	steamId := "76561199232028535"
	cl := v2Players()
	var existing *models.MothershipV2PlayerDbObject
	for _, p := range cl.FindAll() {
		if eqS(p.ExternalProviderId, steamId) && eqS(p.ExternalService, "STEAM") {
			existing = p
			break
		}
	}
	playerId := newId()
	if existing != nil && existing.PlayerId != nil {
		playerId = *existing.PlayerId
	}
	deployId, ok := request.Header("x-mothership-deployment-id")
	if !ok {
		deployId = newId()
	}
	envId, ok := request.Header("x-mothership-env-id")
	if !ok {
		envId = ENVIRONMENT_ID
	}
	titleId, ok := request.Header("x-mothership-title-id")
	if !ok {
		titleId = TENANT_ID
	}

	now := time.Now()
	expSec := now.Add(time.Hour).Unix()
	expMs := now.Add(time.Hour).UnixMilli()

	jwtPayload := stj.O(
		"sub", playerId,
		"did", deployId,
		"env", envId,
		"externalService", "STEAM",
		"externalServiceId", steamId,
		"tid", titleId,
		"tags", nil,
		"orgScopedExternalServiceId", steamId,
		"nbf", now.Unix(),
		"exp", expSec,
		"iat", now.Unix(),
	)

	player := existing
	if player == nil {
		player = models.NewMothershipV2PlayerDbObject()
	}
	player.ExternalProviderId = strp(steamId)
	var prevName *string
	if existing != nil {
		prevName = existing.ExternalProviderUsername
	}
	player.ExternalProviderUsername = strp(orStr(prevName, "drycheetah84"))
	player.IsPrimaryId = true
	player.PlayerId = strp(playerId)
	player.Tags = nil
	player.Token = strp(generateJwt(jwtPayload))
	player.ExpirationTime = expMs
	player.ExternalService = strp("STEAM")
	if existing == nil {
		cl.Insert(player)
	} else {
		cl.Update(player)
	}
	return jsonT(player, 201)
}

// VerifyToken verifies a player session token and returns the associated player.
func (s *MothershipServer) VerifyToken(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	token := str(body, "token")
	session := col(models.NewMothershipSessionDbObject).FindOne(func(x *models.MothershipSessionDbObject) bool {
		return dbEq(x.Token, token)
	})
	if session == nil {
		return jsonAnon(stj.O("valid", false), 401)
	}
	player := col(models.NewMothershipPlayerDbObject).FindOne(func(p *models.MothershipPlayerDbObject) bool {
		return dbEqP(p.MothershipPlayerId, session.PlayerId)
	})
	if player == nil {
		return jsonAnon(stj.O("valid", false), 401)
	}
	return jsonAnon(stj.O("valid", true, "player", player), 200)
}

// boolStr renders a bool as C# string interpolation does.
func boolStr(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// logConsole mirrors Console.WriteLine(line) (verbatim, no formatting).
func logConsole(line string) { consoleWriteLine(line) }

func containsStr(list []string, x string) bool {
	for _, v := range list {
		if v == x {
			return true
		}
	}
	return false
}

// riftUserIdOk: a Meta user id as the PC client sends it (decimal digits, 5..20 long).
func riftUserIdOk(uid string) bool {
	if len(uid) < 5 || len(uid) > 20 {
		return false
	}
	for _, c := range uid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
