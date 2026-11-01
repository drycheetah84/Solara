package mothership

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"solara/internal/dotnet"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

const unauthorizedBody = `{"error":"Unauthorized","code":401}`

func tokenFor(sub string, exp int64) string {
	return generateJwt(stj.O("sub", sub, "exp", exp))
}

func TestStaticConfig(t *testing.T) {
	staticInit()
	if SERVER_API_KEY != testServerKey || serverKeySource != "env HALCYON_SERVER_API_KEY" {
		t.Fatalf("server key %q from %q", SERVER_API_KEY, serverKeySource)
	}
	if INSECURE_TESTING || !QUEST_ENFORCE || RIFT_ENFORCE || !QUEST_REQUIRE_STORE || !QUEST_BAN_ON_TAMPER || QUEST_REQUIRE_PATCHED {
		t.Fatal("unexpected default policy")
	}
}

func TestIndexHealthAndUnknownRoute(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, "GET", "/", "")
	expect(t, rec, 200, `{"service":"Mothership API","version":"1.0.0","status":"running","sdk_version":"v2025.11.24.1"}`)
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatal(rec.Header())
	}
	if m := decode(t, do(t, s, "GET", "/health", "")); m["status"] != "healthy" {
		t.Fatal(m)
	}
	expect(t, do(t, s, "GET", "/nope", ""), 404, "")
}

func TestDisabledAuthRoutes(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "POST", "/v1/client/player/auth/Insecure_1", ""), 403, `{"error":"insecure auth disabled"}`)
	expect(t, do(t, s, "POST", "/v1/client/player/auth/GOOGLE", ""), 403, `{"error":"auth method disabled"}`)
}

func TestJwtValidation(t *testing.T) {
	s := newTestServer(t)
	future := time.Now().Add(time.Hour).Unix()
	good := tokenFor("alice", future)
	if m := decode(t, do(t, s, "GET", "/v1/userdata/client?key_name=k", "", "x-mothership-token", good)); m["user_id"] != "alice" {
		t.Fatal(m)
	}
	bad := []string{
		tampered(good),                          // tampered signature
		tokenFor("alice", time.Now().Unix()-10), // expired
		"a.b",                                   // wrong part count
		generateJwt(stj.O("sub", 5)),            // sub not a string -> caught
		generateJwt(stj.O("exp", "x", "sub", "a")), // exp not a number -> caught
	}
	want := `{"statusCode":401,"name":"Unauthorized","message":"Missing or invalid session token","details":"","key_name":"","error":"Missing or invalid session token"}`
	for _, tok := range bad {
		expect(t, do(t, s, "GET", "/v1/userdata/client?key_name=k", "", "x-mothership-token", tok), 401, want)
	}
	// no exp claim: accepted
	if m := decode(t, do(t, s, "GET", "/v1/userdata/client?key_name=k", "", "x-mothership-token", generateJwt(stj.O("sub", "bob")))); m["user_id"] != "bob" {
		t.Fatal(m)
	}
}

func TestUserDataLifecycle(t *testing.T) {
	s := newTestServer(t)
	tok := tokenFor("u1", time.Now().Add(time.Hour).Unix())
	hdr := []string{"x-mothership-token", tok}

	expect(t, do(t, s, "GET", "/v1/userdata/client?key_name=player_inventory", "", hdr...), 200,
		`{"id":"","user_id":"u1","key_name":"player_inventory","value":"","generation":0,"metadata_id":"player_inventory","created_by":"u1","last_written_by":"u1"}`)
	expect(t, do(t, s, "GET", "/v1/userdata/client", "", hdr...), 400,
		`{"statusCode":400,"name":"BadRequest","message":"Missing key_name or token subject","details":"","key_name":"","error":"Missing key_name or token subject"}`)

	m := decode(t, do(t, s, "POST", "/v1/userdata/client", `{"key_name":"player_inventory","value":"abc"}`, hdr...))
	if m["generation"].(float64) != 1 || m["value"] != "abc" || m["id"] == "" {
		t.Fatal(m)
	}
	m = decode(t, do(t, s, "POST", "/v1/userdata/client", `{"key_name":"player_inventory","value":"xyz"}`, hdr...))
	if m["generation"].(float64) != 2 || m["value"] != "xyz" {
		t.Fatal(m)
	}
	// LiteDB collation: the stored key is found case-insensitively.
	if m = decode(t, do(t, s, "GET", "/v1/userdata/client?key_name=PLAYER_INVENTORY", "", hdr...)); m["value"] != "xyz" {
		t.Fatal(m)
	}
	// POST without a token: dropped save.
	expect(t, do(t, s, "POST", "/v1/userdata/client", `{"key_name":"a"}`), 401,
		`{"statusCode":401,"name":"Unauthorized","message":"Missing or invalid session token","details":"","key_name":"","error":"Missing or invalid session token"}`)
	// DELETE uses the query user_id (JWT only gates).
	expect(t, do(t, s, "DELETE", "/v1/userdata?user_id=u1&key_name=nope", "", hdr...), 404, `{"error":"User data not found"}`)
	expect(t, do(t, s, "DELETE", "/v1/userdata?user_id=u1&key_name=player_inventory", "", hdr...), 200, `{"status":"deleted"}`)
	expect(t, do(t, s, "DELETE", "/v1/userdata?user_id=u1&key_name=player_inventory", ""), 401, unauthorizedBody)
}

func TestServerAuthorizedGate(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=x", ""), 403, `{"error":"forbidden"}`)
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=x", "", "x-server-api-key", "wrong"), 403, `{"error":"forbidden"}`)
	// the automation key is not the server key
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=x", "", "x-server-api-key", testAutomationKey), 403, `{"error":"forbidden"}`)
	key := []string{"x-server-api-key", testServerKey}
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=dash-1", "", key...), 200, `{"authorized":false}`)
	MarkDashboardSession("dash-1")
	MarkDashboardSession("")
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=dash-1", "", key...), 200, `{"authorized":true}`)
	expect(t, do(t, s, "GET", "/v1/server/authorized", "", key...), 200, `{"authorized":false}`)

	// An expired session is evicted.
	stateMu.Lock()
	activeSessions["old"] = time.Now().Unix() - 1
	stateMu.Unlock()
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=old", "", key...), 200, `{"authorized":false}`)
	stateMu.Lock()
	_, still := activeSessions["old"]
	stateMu.Unlock()
	if still {
		t.Fatal("expired session not evicted")
	}

	// A persisted v2 player authorizes (case-sensitive LINQ match).
	p := models.NewMothershipV2PlayerDbObject()
	p.PlayerId, p.ExternalProviderId = strp("pid-9"), strp("ext-9")
	v2Players().Insert(p)
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=ext-9", "", key...), 200, `{"authorized":true}`)
	expect(t, do(t, s, "GET", "/v1/server/authorized?id=EXT-9", "", key...), 200, `{"authorized":false}`)
}

func TestQuestAuthTrustedServer(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{}`), 404, `{"error":"UserId not found"}`)
	expect(t, do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"q1","AttestationToken":"x","MetaNonce":"n"}`), 404, `{"error":"No pending authentication"}`)
	expect(t, do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"q1","MetaNonce":"n"}`), 404, `{"error":"AttestationToken not found"}`)

	m := decode(t, do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"q1"}`))
	if n, _ := m["AttestationNonce"].(string); len(n) != 54 {
		t.Fatalf("nonce %v", m)
	}
	rec := do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"q1234567890","AttestationToken":"x","MetaNonce":"n"}`, "x-server-api-key", testServerKey)
	expect(t, rec, 404, `{"error":"No pending authentication"}`)
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"q1234567890"}`)
	rec = do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"q1234567890","AttestationToken":"x","MetaNonce":"n"}`, "x-server-api-key", testServerKey)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	m = decode(t, rec)
	if m["ExternalProviderUsername"] != "Player_q1234567" || m["Tags"] != nil || m["IsPrimaryId"] != true {
		t.Fatal(m)
	}
	if _, leaked := m["ExternalService"]; leaked {
		t.Fatal("ExternalService must be JSON-ignored")
	}
	tok := m["Token"].(string)
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(tok, ".")[1])
	if !strings.HasPrefix(string(payload), `{"sub":"`+m["PlayerId"].(string)+`","did":"`) ||
		!strings.Contains(string(payload), `"externalService":"QUEST","externalServiceId":"q1234567890","tid":"f3e9fb19","tags":null,"orgScopedExternalServiceId":"q1234567890","nbf":`) {
		t.Fatal(string(payload))
	}
	// trusted-server logins never mark an active session
	if isActiveSession("q1234567890") {
		t.Fatal("trusted server login marked an active session")
	}
	// the issued token is a valid client token
	if m2 := decode(t, do(t, s, "GET", "/v1/userdata/client?key_name=k", "", "x-mothership-token", tok)); m2["user_id"] != m["PlayerId"] {
		t.Fatal(m2)
	}
	// stable player id on re-login
	do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"q1234567890"}`)
	m3 := decode(t, do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"q1234567890","AttestationToken":"x","MetaNonce":"n"}`, "x-server-api-key", testServerKey))
	if m3["PlayerId"] != m["PlayerId"] {
		t.Fatal("player id not stable")
	}
}

// fakeGraph is a stand-in for graph.oculus.com used only by tests.
type fakeGraph struct {
	mu       sync.Mutex
	claims   string
	valid    bool
	verifyQ  url.Values
	nonceF   url.Values
	nonceCT  string
	banQ     chan url.Values
	requests int
}

func newFakeGraph(t *testing.T) *fakeGraph {
	g := &fakeGraph{banQ: make(chan url.Values, 4), valid: true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.requests++
		switch r.URL.Path {
		case "/platform_integrity/verify":
			g.verifyQ = r.URL.Query()
			io.WriteString(w, `{"data":[{"message":"success","claims":"`+base64.RawURLEncoding.EncodeToString([]byte(g.claims))+`"}]}`)
		case "/user_nonce_validate":
			b, _ := io.ReadAll(r.Body)
			g.nonceF, _ = url.ParseQuery(string(b))
			g.nonceCT = r.Header.Get("Content-Type")
			if g.valid {
				io.WriteString(w, `{"is_valid":true}`)
			} else {
				io.WriteString(w, `{"is_valid":false}`)
			}
		case "/platform_integrity/device_ban":
			g.banQ <- r.URL.Query()
			io.WriteString(w, `{"success":true}`)
		}
	}))
	saved := graphBaseURL
	graphBaseURL = srv.URL
	t.Cleanup(func() { graphBaseURL = saved; srv.Close() })
	return g
}

func questClaims(nonce, pkg string) string {
	return `{"request_details":{"nonce":"` + nonce + `","exp":` + itoa(time.Now().Add(time.Hour).Unix()) + `,"timestamp":1},` +
		`"app_state":{"package_id":"` + pkg + `","app_integrity_state":"StoreRecognized","package_cert_sha256_digest":["aa","` + QUEST_EXPECTED_CERT_SHA + `"]},` +
		`"device_state":{"device_integrity_state":"Basic","unique_id":"dev-` + nonce[:4] + `","security_update_pending_days":2}}`
}

func itoa(v int64) string { return stj.MarshalString(v) }

func beginQuest(t *testing.T, s *MothershipServer, user string) string {
	m := decode(t, do(t, s, "POST", "/v2/player/client/auth/begin/QUEST", `{"UserId":"`+user+`"}`))
	return m["AttestationNonce"].(string)
}

func TestQuestAuthEnforced(t *testing.T) {
	s := newTestServer(t)
	g := newFakeGraph(t)
	staticInit()

	// Success path.
	nonce := beginQuest(t, s, "real-1")
	g.claims = questClaims(nonce, QUEST_EXPECTED_PACKAGE)
	rec := do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"real-1","AttestationToken":"tok en+/","MetaNonce":"mn-1"}`)
	if rec.Code != 201 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if g.verifyQ.Get("token") != "tok en+/" || g.verifyQ.Get("access_token") != "OC|"+META_APP_ID+"|test-meta-secret" {
		t.Fatal(g.verifyQ)
	}
	if g.nonceF.Get("nonce") != "mn-1" || g.nonceF.Get("user_id") != "real-1" || g.nonceCT != "application/x-www-form-urlencoded" {
		t.Fatal(g.nonceF, g.nonceCT)
	}
	if !isActiveSession("real-1") {
		t.Fatal("genuine login must mark an active session")
	}

	// Replay of the same MetaNonce is denied (the userproof check is cached now).
	nonce = beginQuest(t, s, "real-1")
	g.claims = questClaims(nonce, QUEST_EXPECTED_PACKAGE)
	expect(t, do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"real-1","AttestationToken":"t","MetaNonce":"mn-1"}`),
		401, `{"error":"userproof nonce replayed"}`)

	// Invalid user proof.
	g.valid = false
	nonce = beginQuest(t, s, "real-2")
	g.claims = questClaims(nonce, QUEST_EXPECTED_PACKAGE)
	expect(t, do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"real-2","AttestationToken":"t","MetaNonce":"mn-2"}`),
		401, `{"error":"userproof nonce invalid"}`)
	g.valid = true

	// Nonce mismatch.
	beginQuest(t, s, "real-3")
	g.claims = questClaims("someothernonce", QUEST_EXPECTED_PACKAGE)
	expect(t, do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"real-3","AttestationToken":"t","MetaNonce":"mn-3"}`),
		401, `{"error":"attestation nonce mismatch"}`)

	// Tamper (wrong package): denied and a device ban is fired once.
	nonce = beginQuest(t, s, "real-4")
	g.claims = questClaims(nonce, "com.evil")
	expect(t, do(t, s, "POST", "/v2/player/client/auth/complete/QUEST", `{"UserId":"real-4","AttestationToken":"t","MetaNonce":"mn-4"}`),
		401, `{"error":"attestation policy failed: package_id=com.evil"}`)
	select {
	case q := <-g.banQ:
		if q.Get("method") != "POST" || q.Get("unique_id") != "dev-"+nonce[:4] || q.Get("is_banned") != "true" ||
			q.Get("remaining_time_in_minute") != "10080" || q.Get("access_token") != "OC|"+META_APP_ID+"|test-meta-secret" {
			t.Fatal(q)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("device ban not sent")
	}
}

func TestBansAndModeration(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "POST", "/v1/moderation/server/ban", `{"player_id":"p1"}`), 401, unauthorizedBody)
	expect(t, do(t, s, "POST", "/v1/moderation/server/ban", `{"player_id":"p1"}`, "x-server-api-key", "x"), 401, unauthorizedBody)
	adm := []string{"x-automation-key", testAutomationKey}

	m := decode(t, do(t, s, "POST", "/v1/moderation/server/ban", `{"player_id":"P1","duration_minutes":60,"category":2,"reason":"r","org_wide":true}`, adm...))
	ban := m["Results"].([]any)[0].(map[string]any)
	if ban["player_id"] != "P1" || ban["duration_minutes"].(float64) != 60 || ban["org_wide"] != true {
		t.Fatal(ban)
	}
	created, _ := time.Parse(time.RFC3339Nano, ban["created_at"].(string))
	expires, _ := time.Parse(time.RFC3339Nano, ban["expires_at"].(string))
	if d := expires.Sub(created); d < 59*time.Minute || d > 61*time.Minute {
		t.Fatal(d)
	}
	// Query by player id: LiteDB collation (case-insensitive).
	if l := decode(t, do(t, s, "GET", "/v1/moderation?player_id=p1", "", adm...))["Results"].([]any); len(l) != 1 {
		t.Fatal(l)
	}
	// Ban by id.
	if b := decode(t, do(t, s, "GET", "/v1/moderation?ban_id="+ban["ban_id"].(string), "", adm...)); b["ban_id"] != ban["ban_id"] {
		t.Fatal(b)
	}
	// Unknown ban id falls through to Results.NotFound(): empty text/plain.
	rec := do(t, s, "GET", "/v1/moderation?ban_id=nope", "", adm...)
	expect(t, rec, 404, "")
	if rec.Header().Get("Content-Type") != "text/plain" {
		t.Fatal(rec.Header())
	}
	// Bulk bans; a non-Int32 duration is an uncaught FormatException -> 500 page.
	m = decode(t, do(t, s, "POST", "/v1/moderation/server/bans/bulk", `{"players":[{"player_id":"a","duration_minutes":5},{"player_id":"b"}]}`, adm...))
	if len(m["Results"].([]any)) != 2 {
		t.Fatal(m)
	}
	rec = do(t, s, "POST", "/v1/moderation/server/bans/bulk", `{"players":[{"duration_minutes":1.5}]}`, adm...)
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "Error while processing ServerBulkBans") {
		t.Fatal(rec.Code, rec.Body.String())
	}
	// Mutes.
	expect(t, do(t, s, "POST", "/v1/moderation/automation", `{"action":"x"}`, adm...), 400, `{"error":"Invalid action"}`)
	mute := decode(t, do(t, s, "POST", "/v1/moderation/automation", `{"action":"mute","player_id":"p","duration_minutes":3}`, adm...))
	expect(t, do(t, s, "DELETE", "/v1/moderation/automation", "", adm...), 404, `{"error":"Mute not found"}`)
	expect(t, do(t, s, "DELETE", "/v1/moderation/automation?mute_id="+mute["mute_id"].(string), "", adm...), 200, `{"status":"deleted"}`)
	all := decode(t, do(t, s, "GET", "/v1/moderation/automation", "", adm...))
	if len(all["bans"].([]any)) != 3 || len(all["mutes"].([]any)) != 0 || len(all["reports"].([]any)) != 0 {
		t.Fatal(all)
	}
	// Client report requires a JWT, not an admin key.
	expect(t, do(t, s, "POST", "/v1/moderation/client/report", `{}`, adm...), 401, unauthorizedBody)
	tok := tokenFor("u", time.Now().Add(time.Hour).Unix())
	r := decode(t, do(t, s, "POST", "/v1/moderation/client/report", `{"reported_user_id":"x","category":4,"modded_client":true}`, "x-mothership-token", tok))
	if r["reported_user_id"] != "x" || r["category"].(float64) != 4 || r["modded_client"] != true || r["reporter_user_id"] != "" {
		t.Fatal(r)
	}
}

func TestSharedGroupDataOrder(t *testing.T) {
	s := newTestServer(t)
	q := jesc("0022")
	expect := func(t *testing.T, rec *httptest.ResponseRecorder, code int, body string) {
		t.Helper()
		expect(t, rec, code, strings.ReplaceAll(body, "QxQ", q+"x"+q))
	}
	adm := []string{"x-server-api-key", testServerKey}
	expect(t, do(t, s, "POST", "/v1/server/shared-group", `{"action":"create","sharedGroupId":"G1"}`, adm...), 200, `{"SharedGroupId":"G1"}`)
	expect(t, do(t, s, "POST", "/v1/server/shared-group", `{"action":"delete"}`, adm...), 400, `{"error":"Invalid action"}`)
	expect(t, do(t, s, "GET", "/v1/shared-group/g1", ""), 401, unauthorizedBody)
	expect(t, do(t, s, "POST", "/v1/shared-group/g1", `{"data":{"z":"1","a":2,"m":{"x":1}}}`, adm...), 200,
		`{"SharedGroupId":"G1","data":{"z":"1","a":"2","m":"{QxQ:1}"},"members":[]}`)
	expect(t, do(t, s, "POST", "/v1/shared-group/G1", `{"data":{"b":"3"},"keysToRemove":["a"]}`, adm...), 200,
		`{"SharedGroupId":"G1","data":{"z":"1","m":"{QxQ:1}","b":"3"},"members":[]}`)
	expect(t, do(t, s, "POST", "/v1/shared-group/G1/members", `{"members":["x","y","x"]}`, adm...), 200,
		`{"SharedGroupId":"G1","data":{"z":"1","m":"{QxQ:1}","b":"3"},"members":["x","y","x"]}`)
	expect(t, do(t, s, "DELETE", "/v1/shared-group/G1/members", `{"members":["x"]}`, adm...), 200,
		`{"SharedGroupId":"G1","data":{"z":"1","m":"{QxQ:1}","b":"3"},"members":["y"]}`)
	expect(t, do(t, s, "GET", "/v1/shared-group/G1/data", "", adm...), 200,
		`{"SharedGroupId":"G1","data":{"z":"1","m":"{QxQ:1}","b":"3"},"members":["y"]}`)
	expect(t, do(t, s, "DELETE", "/v1/shared-group/G1", "", adm...), 200, `{"status":"deleted"}`)
	expect(t, do(t, s, "GET", "/v1/shared-group/G1", "", adm...), 404, `{"error":"Shared group not found"}`)
}

func TestStorageAndMisc(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "GET", "/storage/nope", ""), 400, `{"error":"Invalid storage key"}`)
	expect(t, do(t, s, "GET", "/storage/players", ""), 200, `[]`)
	rec := do(t, s, "POST", "/storage/x", `{ "b" : [1, 2.50], "a": "<", "b": 3 }`)
	if rec.Code != 201 || !strings.HasSuffix(rec.Body.String(), `","data":{"b":3,"a":"`+jesc("003C")+`"}}`) {
		t.Fatal(rec.Code, rec.Body.String())
	}
	// malformed body -> empty dictionary
	if rec = do(t, s, "POST", "/storage/x", `{`); !strings.HasSuffix(rec.Body.String(), `"data":{}}`) {
		t.Fatal(rec.Body.String())
	}
	if m := decode(t, do(t, s, "GET", "/debug/storage", "")); len(m) != 22 || m["players"].(float64) != 0 {
		t.Fatal(m)
	}
	m := decode(t, do(t, s, "POST", "/v1/quest/analytics/event/batch", `{"events":[{"a":1},"s"]}`))
	if m["events_processed"].(float64) != 2 || m["status"] != "success" {
		t.Fatal(m)
	}
	ev := do(t, s, "GET", "/storage/analytics_events", "").Body.String()
	if !strings.Contains(ev, `"platform":"quest"`) || !strings.Contains(ev, `"data":"{`+jesc("0022")+`a`+jesc("0022")+`:1}"`) || !strings.Contains(ev, `"data":"s"`) {
		t.Fatal(ev)
	}
}

// jesc builds the JSON escape sequence backslash-u-XXXX (kept out of source
// text on purpose, see docs/PORTING_GUIDE.md section 0).
func jesc(hex string) string { return "\\" + "u" + hex }

func TestFriends(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "POST", "/api/GetFriendsV2", `{}`), 400, `{"StatusCode":400,"Error":"PlayFabId is required"}`)
	expect(t, do(t, s, "POST", "/api/RequestFriend", `{"PlayFabId":"me"}`), 400, `{"error":"Missing required fields"}`)
	expect(t, do(t, s, "POST", "/api/RequestFriend", `{"PlayFabId":"me","MyFriendLinkId":"l","FriendFriendLinkId":"abcdefghij"}`), 200, `{"success":true}`)
	expect(t, do(t, s, "POST", "/api/RequestFriend", `{"PlayFabId":"ME","MyFriendLinkId":"l","FriendFriendLinkId":"ABCDEFGHIJ"}`), 409, `{"error":"Already friends"}`)
	expect(t, do(t, s, "POST", "/api/SetPrivacyState", `{"PlayFabId":"me","PrivacyState":"HIDDEN"}`), 200, `{"StatusCode":200,"Error":null}`)
	rec := do(t, s, "POST", "/api/GetFriendsV2", `{"PlayFabId":"me"}`)
	body := rec.Body.String()
	if !strings.HasPrefix(body, `{"Result":{"Friends":[{"Presence":{"FriendLinkId":"abcdefghij","UserName":"Player_abcdefgh","RoomId":null,"Zone":null,"Region":null,"IsPublic":true},"Created":"`) ||
		!strings.HasSuffix(body, `"}],"MyPrivacyState":2},"StatusCode":200,"Error":null}`) {
		t.Fatal(body)
	}
	expect(t, do(t, s, "POST", "/api/RemoveFriend", `{"PlayFabId":"me","FriendFriendLinkId":"abcdefghij"}`), 200, `{"success":true}`)
}

func TestInventoryClientGrantsCatalogue(t *testing.T) {
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "someone-else, inv-user")
	s := newTestServer(t)
	catalogueMu.Lock()
	catalogueAt = dotnet.MinValue // force a rebuild
	catalogueMu.Unlock()
	tok := tokenFor("inv-user", time.Now().Add(time.Hour).Unix())
	inv := encodeQuestBlob(`{"equippedItems":["Hat","None","Coat"]}`)
	do(t, s, "POST", "/v1/userdata/client", `{"key_name":"player_inventory","value":"`+inv+`"}`, "x-mothership-token", tok)
	catalogueMu.Lock()
	catalogueAt = dotnet.MinValue // force a rebuild
	catalogueMu.Unlock()
	rec := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok)
	// worn items first, then the embedded 50451 catalogue; every entitlement carries an int quantity
	// (the 50451 parser 0x4DDBA90 rejects the whole load without one)
	wantPrefix := `{"Results":{"QUEST":{"platform":"QUEST","isPrimary":true,"entitlements":[` +
		`{"entitlement_id":"` + stableId("inv-user:Hat") + `","in_game_id":"Hat","name":"Hat","quantity":1},` +
		`{"entitlement_id":"` + stableId("inv-user:Coat") + `","in_game_id":"Coat","name":"Coat","quantity":1},`
	body := rec.Body.String()
	if rec.Code != 200 || !strings.HasPrefix(body, wantPrefix) {
		t.Fatalf("got %d %.400s", rec.Code, body)
	}
	ids, cur := a2CatalogueItems()
	if len(ids) < 600 || len(cur) != 3 || strings.Count(body, `"quantity":`) != len(ids)+2 {
		t.Fatalf("catalogue: %d ids, %d currencies, %d quantities", len(ids), len(cur), strings.Count(body, `"quantity":`))
	}
	if !strings.Contains(body, `"in_game_id":"HardCurrencyJoules","name":"HardCurrencyJoules","quantity":1000000}`) ||
		strings.Contains(body, "BadgeDeveloper") {
		t.Fatal("currency balance / developer items")
	}

	// A stored row that is not an entitlement (no in_game_id) is skipped; the catalogue still loads.
	adm := []string{"x-server-api-key", testServerKey}
	do(t, s, "POST", "/v1/inventory", `{"player_id":"inv-user","item_id":"i1", "x" : [ 1 ]}`, adm...)
	body2 := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok).Body.String()
	if strings.Contains(body2, `"item_id"`) || strings.Count(body2, `"quantity":`) != len(ids)+2 {
		t.Fatalf("%.300s", body2)
	}
	expect(t, do(t, s, "GET", "/v1/inventory/client", ""), 401,
		`{"statusCode":401,"name":"Unauthorized","message":"Missing or invalid session token","details":"","key_name":"","error":"Missing or invalid session token"}`)
}

func TestAutomationCrud(t *testing.T) {
	s := newTestServer(t)
	adm := []string{"x-automation-key", testAutomationKey}
	expect(t, do(t, s, "POST", "/v1/title-data", `{"key":"K","value":"v1"}`), 401, unauthorizedBody)
	m := decode(t, do(t, s, "POST", "/v1/title-data", `{"key":"K","value":"v1"}`, adm...))
	m2 := decode(t, do(t, s, "POST", "/v1/title-data/automation", `{"key":"k","value":"v2"}`, adm...)) // collation: updates K
	if m["key"] != "K" || m2["key"] != "K" || m2["value"] != "v2" {
		t.Fatal(m, m2)
	}
	if l := decode(t, do(t, s, "GET", "/v1/title-data/client", ""))["Results"].([]any); len(l) != 1 {
		t.Fatal(l)
	}
	expect(t, do(t, s, "DELETE", "/v1/title-data?key=zz", "", adm...), 404, `{"error":"Title data not found"}`)

	ent := decode(t, do(t, s, "POST", "/v1/entitlement-catalog/automation", `{"name":"n"}`, adm...))
	if ent["type"] != "DURABLE" {
		t.Fatal(ent)
	}
	upd := decode(t, do(t, s, "PUT", "/v1/entitlement-catalog/automation", `{"entitlement_id":"`+ent["entitlement_id"].(string)+`","type":"CONSUMABLE"}`, adm...))
	if upd["result"].(map[string]any)["type"] != "CONSUMABLE" {
		t.Fatal(upd)
	}
	off := decode(t, do(t, s, "POST", "/v1/offers/automation", `{"name":"o","bundle_pricing":{"a" : 1},"discount_percent":5}`, adm...))
	if off["bundle_pricing"] != `{"a" : 1}` || off["discount_percent"].(float64) != 5 {
		t.Fatal(off) // JsonElement.ToString() of an object = raw text
	}
	expect(t, do(t, s, "POST", "/v1/purchase/client", `{"OfferId":"none"}`, adm...), 404, `{"error":"Offer not found"}`)
	tr := decode(t, do(t, s, "POST", "/v1/progression/automation", `{"action":"create_level"}`, adm...))
	if _, ok := tr["level_id"]; !ok || len(tr) != 1 {
		t.Fatal(tr)
	}
	env := decode(t, do(t, s, "POST", "/v1/env/automation", `{"titleId":"t","requiredTags":["a",1]}`, adm...))
	if e := env["env"].(map[string]any); e["envId"] == "" || len(e["requiredTags"].([]any)) != 1 {
		t.Fatal(env)
	}
	expect(t, do(t, s, "GET", "/v1/transaction-catalog/automation/sunset", "", adm...), 200, `{"Results":[]}`)
	expect(t, do(t, s, "GET", "/v1/permissions/client", "", adm...), 200, `{"permissions":["read","write","execute"],"roles":["player"]}`)

	// Player tags + bulk lookup.
	p := models.NewMothershipPlayerDbObject()
	p.MothershipPlayerId = strp("MP1")
	col(models.NewMothershipPlayerDbObject).Insert(p)
	expect(t, do(t, s, "POST", "/v1/automation/player/auth/players/tags", `{"player_id":"mp1","tags":["t1"]}`, adm...), 200, `{"playersToTags":{"mp1":["t1"]}}`)
	bl := decode(t, do(t, s, "POST", "/v1/server/player/auth/bulk_player_lookup", `{"Lookups":[{"PlayerId":"MP1"},{"PlayerId":"zz"},{}]}`, adm...))
	if l := bl["Results"].([]any); len(l) != 1 || l[0].(map[string]any)["Tags"].([]any)[0] != "t1" {
		t.Fatal(bl)
	}
	// verify_token: session lookup.
	sess := models.NewMothershipSessionDbObject()
	sess.Token, sess.PlayerId = strp("tok-1"), strp("MP1")
	col(models.NewMothershipSessionDbObject).Insert(sess)
	expect(t, do(t, s, "POST", "/v1/server/player/auth/verify_token", `{"token":"nope"}`, adm...), 401, `{"valid":false}`)
	if v := decode(t, do(t, s, "POST", "/v1/server/player/auth/verify_token", `{"token":"tok-1"}`, adm...)); v["valid"] != true {
		t.Fatal(v)
	}
}

func TestSteamAndRift(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "POST", "/v2/player/client/auth/complete/STEAM", `{"Nonce":"n"}`), 404, `{"error":"SteamTicket not found"}`)
	m := decode(t, do(t, s, "POST", "/v2/player/client/auth/complete/STEAM", `{"Nonce":"n","SteamTicket":"t"}`, "x-mothership-env-id", "E", "x-mothership-title-id", ""))
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(m["Token"].(string), ".")[1])
	if m["ExternalProviderUsername"] != "drycheetah84" || !strings.Contains(string(payload), `"env":"E","externalService":"STEAM","externalServiceId":"76561199232028535","tid":"",`) {
		t.Fatal(m, string(payload))
	}
	r := decode(t, do(t, s, "POST", "/v1/client/player/auth/RIFT", `{}`))
	if r["ExternalProviderId"] != "4649330831842445" || !isActiveSession("4322323827863641") || !isActiveSession(r["PlayerId"].(string)) {
		t.Fatal(r)
	}
	if n := decode(t, do(t, s, "GET", "/v2/player/client/auth/begin/STEAM", "")); len(n["Nonce"].(string)) != 22 {
		t.Fatal(n)
	}
}

// tampered changes the last signature character (always to a different one).
func tampered(tok string) string {
	if strings.HasSuffix(tok, "A") {
		return tok[:len(tok)-1] + "E"
	}
	return tok[:len(tok)-1] + "A"
}

// Solara extension used by the game-server DLL to enforce bans at station login.
func TestServerActiveBan(t *testing.T) {
	s := newTestServer(t)
	expect(t, do(t, s, "GET", "/v1/moderation/server/active_ban?player_id=P1", ""), 401, unauthorizedBody)
	adm := []string{"x-automation-key", testAutomationKey}
	if m := decode(t, do(t, s, "GET", "/v1/moderation/server/active_ban?player_id=P1", "", adm...)); m["banned"] != false {
		t.Fatalf("no bans: %v", m)
	}
	do(t, s, "POST", "/v1/moderation/server/ban", `{"player_id":"P1","duration_minutes":0,"reason":"expired"}`, adm...)
	if m := decode(t, do(t, s, "GET", "/v1/moderation/server/active_ban?player_id=P1", "", adm...)); m["banned"] != false {
		t.Fatalf("a 0-minute ban is already expired: %v", m)
	}
	do(t, s, "POST", "/v1/moderation/server/ban", `{"player_id":"P1","duration_minutes":30,"reason":"short"}`, adm...)
	do(t, s, "POST", "/v1/moderation/server/ban", `{"player_id":"P1","duration_minutes":600,"reason":"long"}`, adm...)
	m := decode(t, do(t, s, "GET", "/v1/moderation/server/active_ban?player_id=P1", "", adm...))
	if m["banned"] != true || m["reason"] != "long" {
		t.Fatalf("active ban (longest wins): %v", m)
	}
	if m := decode(t, do(t, s, "GET", "/v1/moderation/server/active_ban?player_id=OTHER", "", adm...)); m["banned"] != false {
		t.Fatalf("other player: %v", m)
	}
}

// A player off the allowlist gets only what they own (plus currencies at 0), never the catalogue or worn items.
func TestInventoryClientNotAllowlisted(t *testing.T) {
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "inv-user")
	s := newTestServer(t)
	tok := tokenFor("plain-user", time.Now().Add(time.Hour).Unix())
	inv := encodeQuestBlob(`{"equippedItems":["Hat"]}`)
	do(t, s, "POST", "/v1/userdata/client", `{"key_name":"player_inventory","value":"`+inv+`"}`, "x-mothership-token", tok)
	adm := []string{"x-server-api-key", testServerKey}
	do(t, s, "POST", "/v1/inventory", `{"player_id":"plain-user","item_id":"i1","entitlement_id":"e1","in_game_id":"OwnedThing","name":"OwnedThing"}`, adm...)
	body := do(t, s, "GET", "/v1/inventory/client", "", "x-mothership-token", tok).Body.String()
	_, cur := a2CatalogueItems()
	if strings.Contains(body, `"in_game_id":"Hat"`) || !strings.Contains(body, `"in_game_id":"OwnedThing"`) ||
		strings.Count(body, `"quantity":`) != len(cur)+1 ||
		!strings.Contains(body, `"in_game_id":"HardCurrencyJoules","name":"HardCurrencyJoules","quantity":0}`) {
		t.Fatalf("%.600s", body)
	}

	// The allowlist also matches the Meta id of a linked v2 player.
	t.Setenv("SOLARA_FULL_INVENTORY_IDS", "9990001")
	pl := models.NewMothershipV2PlayerDbObject()
	pl.PlayerId, pl.ExternalProviderId = strp("plain-user"), strp("9990001")
	v2Players().Insert(pl)
	if !fullCatalogueUser("plain-user") || fullCatalogueUser("other") {
		t.Fatal("v2 link")
	}
}
