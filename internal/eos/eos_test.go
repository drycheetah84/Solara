package eos

import (
	"bufio"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"solara/internal/dotnet"
	"solara/internal/eos/models"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/secrets"
	"solara/internal/stj"
)

var oracle = map[string]string{}

// apos is STJ's escape for an apostrophe (built at runtime; see PORTING_GUIDE tooling hazard).
var apos = "\\" + "u0027"

func TestMain(m *testing.M) {
	logging.DisableFiles = true
	dir, err := os.MkdirTemp("", "eos-test")
	if err != nil {
		panic(err)
	}
	BaseDirectory = dir
	secrets.Set("TURN_SECRET", "test-turn-secret")
	secrets.Set("RTC_API_KEY", "test-rtc-key")
	secrets.Set("RTC_API_SECRET", "test-rtc-secret")

	f, err := os.Open(filepath.Join("testdata", "oracle.txt"))
	if err != nil {
		panic(err)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "\t"); ok {
			oracle[k] = v
		}
	}
	f.Close()
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// esc applies the oracle's CR/LF escaping.
func esc(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r", `\r`), "\n", `\n`)
}

func checkOracle(t *testing.T, name string, got []byte) {
	t.Helper()
	want, ok := oracle[name]
	if !ok {
		t.Fatalf("oracle has no %q", name)
	}
	if esc(string(got)) != want {
		t.Errorf("%s mismatch\n got: %s\nwant: %s", name, esc(string(got)), want)
	}
}

// ── Route table ─────────────────────────────────────────────────────────────

func TestRouteTables(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "original_routes.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		cols := strings.Split(line, "\t")
		if len(cols) == 4 && (cols[0] == "EosGatewayServer" || cols[0] == "EosWsServer") {
			want[cols[0]] = append(want[cols[0]], cols[1]+" /"+strings.Trim(cols[2], "/")+" "+cols[3])
		}
	}
	gw := newEosGatewayServer()
	ws := newEosWsServer()
	for name, srv := range map[string][]string{"EosGatewayServer": routes(gw.HTTP.Endpoints()), "EosWsServer": routes(ws.HTTP.Endpoints())} {
		w := want[name]
		if len(w) == 0 || strings.Join(w, "\n") != strings.Join(srv, "\n") {
			t.Errorf("%s routes differ\n got: %v\nwant: %v", name, srv, w)
		}
	}
}

func routes(eps []*hosting.Endpoint) []string {
	var out []string
	for _, e := range eps {
		out = append(out, e.Method+" "+e.RouteUri+" "+e.Name)
	}
	return out
}

// ── Model JSON shapes (bytes from the original runtime) ─────────────────────

func TestModelJSONDefaults(t *testing.T) {
	cases := map[string]any{
		"EosLobbyFilterRequest.default":      models.NewEosLobbyFilterRequest(),
		"EosLobbyResponse.default":           models.NewEosLobbyResponse(),
		"EosLobbySession.default":            models.NewEosLobbySession(),
		"EosLobbySettings.default":           models.NewEosLobbySettings(),
		"EosLobbyPlatforms.default":          models.NewEosLobbyPlatforms(),
		"EosLobbyMemberEntry.default":        models.NewEosLobbyMemberEntry(),
		"EosLobbyCreateRequest.default":      models.NewEosLobbyCreateRequest(),
		"EosLobbyUpdateRequest.default":      models.NewEosLobbyUpdateRequest(),
		"EosFilterRequest.default":           models.NewEosFilterRequest(),
		"EosMatchmakingResponse.default":     models.NewEosMatchmakingResponse(),
		"EosMatchmakingSession.default":      models.NewEosMatchmakingSession(),
		"EosSessionAttributes.default":       models.NewEosSessionAttributes(),
		"EosSessionSettings.default":         models.NewEosSessionSettings(),
		"EosOAuthTokenResponse.default":      models.NewEosOAuthTokenResponse(),
		"EosTelemetryRequest.default":        models.NewEosTelemetryRequest(),
		"EosTelemetryEvent.default":          models.NewEosTelemetryEvent(),
		"EosTurnCredentialsResponse.default": models.NewEosTurnCredentialsResponse(),
	}
	for name, v := range cases {
		checkOracle(t, name, stj.MustMarshal(v))
	}
	si := models.NewEosSessionInfo()
	if len(si.Id) != 32 {
		t.Errorf("EosSessionInfo.Id initializer: %q", si.Id)
	}
	si.Id = "fixed"
	checkOracle(t, "EosSessionInfo.default", stj.MustMarshal(si))
}

func s(v string) *string { return &v }

func populatedLobby() *models.EosLobbySession {
	l := models.NewEosLobbySession()
	l.Deployment, l.Id, l.Bucket = "dep", "lid", "b<&>"
	l.TotalPlayers, l.OpenPublicPlayers = 2, 8
	l.PublicPlayers = []string{"p1", "p2"}
	l.Started, l.LastUpdated = true, "2025-01-02T03:04:05.678Z"
	l.Attributes.Set("z", s("1"))
	l.Attributes.Set("a", nil)
	l.Attributes.Set("ADDRESS_s", s("1.2.3.4"))
	l.Owner, l.OwnerPlatformId = "p1", 4000
	l.Settings = models.NewEosLobbySettings()
	p := models.NewEosLobbyPlatforms()
	p.AllowedPlatformIds = []int{1, 2}
	l.Settings.Platforms = p
	l.Lock = "lk"
	return l
}

func fullSession() *models.EosSessionInfo {
	return &models.EosSessionInfo{Id: "id1", ServerName: "S" + string(rune(0xE9)), IpAddress: "1.1.1.1", Port: "7777",
		MaxPlayers: 12, Bucket: "B", BuildId: 1, ImguiPort: "9", StationId: s("st"), DeploymentId: s("dp"), PublicPlayers: []string{"a"}}
}

func TestModelJSONPopulated(t *testing.T) {
	lobby := populatedLobby()
	checkOracle(t, "EosLobbySession.populated", stj.MustMarshal(lobby))
	checkOracle(t, "EosLobbyResponse.populated", stj.MustMarshal(&models.EosLobbyResponse{Count: 1, Sessions: []*models.EosLobbySession{lobby}}))

	me := models.NewEosLobbyMemberEntry()
	me.PlatformId, me.Platform = 1, "steam"
	me.Data.Set("b", true)
	me.Data.Set("n", int64(5))
	me.Data.Set("d", 1.5)
	me.Data.Set("s", "x")
	me.Data.Set("nul", nil)
	tenth := 0.1 // runtime addition (Go folds constant 0.1+0.2 exactly)
	me.Data.Set("big", tenth+0.2)
	checkOracle(t, "EosLobbyMemberEntry.populated", stj.MustMarshal(me))

	crit := models.NewEosFilterCriterion()
	crit.Key, crit.Op, crit.Value = "bucket", "EQUAL", stj.MustParse([]byte(`"A2_4729"`))
	checkOracle(t, "EosFilterCriterion.populated", stj.MustMarshal(crit))

	f := false
	checkOracle(t, "EosOAuthTokenResponse.populated", stj.MustMarshal(&models.EosOAuthTokenResponse{
		AccessToken: "at", TokenType: "bearer", ExpiresAt: "2035-01-01T00:00:00.000Z", ExpiresIn: 315360000, Nonce: s("n"),
		Features: []string{"A", "B"}, OrganizationId: "o", ProductId: "p", SandboxId: "s", DeploymentId: "d",
		OrganizationUserId: s("ou"), ProductUserId: s("pu"), ProductUserIdCreated: &f, IdToken: s("it"), ClientId: s("c"),
	}))
	checkOracle(t, "EosTurnCredentialsResponse.populated", stj.MustMarshal(&models.EosTurnCredentialsResponse{
		Username: "u", Password: "p+/=", Ttl: 3600000, Uris: []string{"stun:x"}}))
	checkOracle(t, "EosSessionInfo.populated", stj.MustMarshal(fullSession()))
}

func TestIndentedPersistenceFormat(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("oracle captured with Windows Environment.NewLine")
	}
	si := models.NewEosSessionInfo()
	si.Id = "fixed"
	checkOracle(t, "Sessions.indented", []byte(serializeIndented([]*models.EosSessionInfo{fullSession(), si})))
	checkOracle(t, "Lobbies.indented", []byte(serializeIndented([]*models.EosLobbySession{populatedLobby(), models.NewEosLobbySession()})))
	checkOracle(t, "EmptyList.indented", []byte(serializeIndented([]*models.EosSessionInfo{})))
}

func TestModelDeserializeDefaults(t *testing.T) {
	var req *models.EosLobbyCreateRequest
	stj.MustUnmarshal([]byte(`{"settings":{"maxPublicPlayers":4,"platforms":{}},"attributes":{"b":"1","a":null}}`), &req)
	checkOracle(t, "EosLobbyCreateRequest.deser", stj.MustMarshal(req))

	var si *models.EosSessionInfo
	stj.MustUnmarshal([]byte(`{"id":"x","public_players":["q"]}`), &si)
	checkOracle(t, "EosSessionInfo.deser", stj.MustMarshal(si))

	var tr *models.EosTelemetryRequest
	stj.MustUnmarshal([]byte("{\"Events\":[{\"EventName\":\"e1\",\"EventData\":{ \"k\" : [1, 2.50, {\"x\" : \"\\"+"u00e9\\/\"}], \"a\":null, \"t\": true }}]}"), &tr)
	checkOracle(t, "EosTelemetryEvent.roundtrip", stj.MustMarshal(tr.Events[0]))
}

func TestObservabilityRecordsJSON(t *testing.T) {
	ts := dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234567)
	checkOracle(t, "TelemetryEvent", stj.MustMarshal(TelemetryEvent{"abc", "ev", `{"x":1}`, ts}))
	checkOracle(t, "WsMessage", stj.MustMarshal(WsMessage{"in", "CONNECT", "a:b", "body", dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc)}))
	cs := &WsClientSession{SessionId: "sid", Path: "/p", ConnectedAt: dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc)}
	cs.AddMessage(WsMessage{"in", "c", "h", "b", dotnet.UtcNow()})
	checkOracle(t, "WsClientSession", stj.MustMarshal(cs))
}

func TestObservabilityStore(t *testing.T) {
	PushTelemetry("a", "1")
	PushTelemetry("b", "2")
	got := GetTelemetry(1)
	if len(got) != 1 || got[0].EventName != "b" || len(got[0].Id) != 12 {
		t.Fatalf("GetTelemetry(1) = %+v", got)
	}
	c := OpenClient("obs-sess", "/x")
	PushWsMessage("obs-sess", "in", "CONNECT", "", "")
	CloseClient("obs-sess")
	if GetClient("obs-sess") != c || c.MessageCount() != 1 || c.Disconnected() == nil {
		t.Fatal("ws client tracking")
	}
	if GetClient("nope") != nil {
		t.Fatal("unknown client should be nil")
	}
}

// ── Helpers vs oracle ───────────────────────────────────────────────────────

func TestDeterministicIds(t *testing.T) {
	check := func(name, got string) {
		t.Helper()
		if oracle[name] != got {
			t.Errorf("%s: got %q want %q", name, got, oracle[name])
		}
	}
	check("ProductUserIdFor.alice", ProductUserIdFor("alice"))
	check("OrgUserIdFor.alice", OrgUserIdFor("alice"))
	check("ProductUserIdFor.unicode", ProductUserIdFor("J"+string(rune(0xF6))+"rg"))
	check("ExtractClientId", ExtractClientId("Basic "+base64.StdEncoding.EncodeToString([]byte("cid:secret")), "fb"))
	check("SteamIdFromTicket.abc", steamIdFromTicket(s("abcdef0123")))
	check("SteamIdFromTicket.null", steamIdFromTicket(nil))
	check("SteamIdFromTicket.zz", steamIdFromTicket(s("zz")))
	b64 := func(x string) string { return base64.RawURLEncoding.EncodeToString([]byte(x)) }
	check("ExtractPuid.ok", ExtractPuidFromBearer("Bearer h."+b64(`{"productUserId":"0002abc"}`)+".s"))
	check("ExtractPuid.missing", ExtractPuidFromBearer("Bearer h."+b64(`{"x":1}`)+".s"))
	check("ExtractPuid.null", ExtractPuidFromBearer("Bearer h."+b64(`{"productUserId":null}`)+".s"))
	check("ExtractPuid.bad", ExtractPuidFromBearer("Bearer h.!!!.s"))
	check("ExtractPuid.basic", ExtractPuidFromBearer("Basic x"))
}

// ── JWT ─────────────────────────────────────────────────────────────────────

func verifyJWT(t *testing.T, tok string) stj.Element {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("bad jwt %q", tok)
	}
	hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if string(hdr) != `{"kid":"2022-06-14T06:17:57.047928700Z","typ":"JWT","alg":"RS256"}` {
		t.Fatalf("header %s", hdr)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(jwtPublicKey(), crypto.SHA256, h[:], sig); err != nil {
		t.Fatalf("signature: %v", err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	return stj.MustParse(payload)
}

func TestJwtSignVerify(t *testing.T) {
	p := verifyJWT(t, UserToken("cid", PRODUCT_ID, SANDBOX_ID, DEPLOYMENT_ID, ORG_ID, "pu", "ou", "n", "Name", "1.2.3.4", "oculus_app_id", "0", "other"))
	var keys []string
	for _, pr := range p.EnumerateObject() {
		keys = append(keys, pr.Name)
	}
	if strings.Join(keys, ",") != "clientId,productId,iss,env,nonce,organizationId,features,productUserId,organizationUserId,clientIp,deploymentId,sandboxId,tokenType,exp,iat,account,jti" {
		t.Errorf("user token claims order: %v", keys)
	}
	if p.GetProperty("account").GetRawText() != `{"idp":"oculus_app_id","displayName":"Name","id":"0","plf":"other","oculusOrgScopedId":"0"}` {
		t.Errorf("account claim %s", p.GetProperty("account").GetRawText())
	}
	if p.GetProperty("exp").GetInt64()-p.GetProperty("iat").GetInt64() != 3600*24*365*10 {
		t.Error("user token lifetime")
	}
	c := verifyJWT(t, ClientToken("cid", "p", "s", "d", "o"))
	if c.GetProperty("tokenType").Str() != "clientToken" || c.GetProperty("exp").GetInt64()-c.GetProperty("iat").GetInt64() != 3600 {
		t.Error("client token")
	}
	id := verifyJWT(t, IdToken("cid", "p", "s", "d", "pu", "steam", "765", "steam"))
	if id.GetProperty("act").GetRawText() != `{"pltfm":"steam","eaid":"765","eat":"steam"}` {
		t.Errorf("act %s", id.GetProperty("act").GetRawText())
	}

	// JWKS publishes the same key
	jwks := stj.MustParse(stj.MustMarshal(GetJwks())).GetProperty("keys").EnumerateArray()[0]
	n, _ := base64.RawURLEncoding.DecodeString(jwks.GetProperty("n").Str())
	if new(big.Int).SetBytes(n).Cmp(jwtPublicKey().N) != 0 || jwks.GetProperty("e").Str() != "AQAB" || len(n) != 256 {
		t.Error("jwks key mismatch")
	}
	if !strings.HasPrefix(stj.MarshalString(GetJwks()), `{"keys":[{"kty":"RSA","use":"sig","alg":"RS256","kid":"2022-06-14T06:17:57.047928700Z","n":"`) {
		t.Error("jwks shape")
	}
}

// ── HTTP handlers ───────────────────────────────────────────────────────────

func do(t *testing.T, srv http.Handler, method, target, ctype, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func bearerFor(puid, name string) string {
	payload := `{"productUserId":"` + puid + `","account":{"displayName":"` + name + `"}}`
	return "Bearer x." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func TestDefaultEndpointsBytes(t *testing.T) {
	gw := newEosGatewayServer()
	rec := do(t, gw.HTTP, "GET", "/sdk/v1/default", "", "", nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" ||
		rec.Header().Get("X-Epic-Correlation-Id") != CORRELATION_ID || rec.Header().Get("Server") != SERVER_NAME {
		t.Fatalf("status/headers %d %v", rec.Code, rec.Header())
	}
	checkOracle(t, "default_endpoints", rec.Body.Bytes())
	rec = do(t, gw.HTTP, "GET", "/sdk/v1/product/abc", "", "", nil)
	checkOracle(t, "custom_endpoints", rec.Body.Bytes())
}

func TestSessionsAndMatchmakingFilter(t *testing.T) {
	gw := newEosGatewayServer()
	ClearSessions()
	defer ClearSessions()

	a := models.NewEosSessionInfo()
	a.Id, a.ServerName, a.IpAddress, a.Port, a.Bucket, a.DeploymentId = "sA", "Alpha", "10.0.0.1", "7777", "A2_4729", s("depA")
	a.PublicPlayers = []string{"p1", "p2"}
	b := models.NewEosSessionInfo()
	b.Id, b.ServerName, b.Bucket, b.DeploymentId, b.StationId = "sB", "Beta", "OTHER", s("depB"), s("StationB")
	UpsertSession(a)
	UpsertSession(b)
	UpsertSession(a) // upsert moves to the end
	snap := SessionsSnapshot()
	if len(snap) != 2 || snap[0].Id != "sB" || snap[1].Id != "sA" {
		t.Fatalf("upsert order %v", snap)
	}
	persisted, _ := os.ReadFile(filepath.Join(BaseDirectory, "sessions.json"))
	if !strings.Contains(string(persisted), `"id": "sA"`) {
		t.Fatalf("sessions.json not persisted: %s", persisted)
	}

	rec := do(t, gw.HTTP, "POST", "/matchmaking/v1/x/filter", "application/json",
		`{"criteria":[{"key":"bucket","op":"EQUAL","value":"A2_4729"},{"key":"attributes.NUMPUBLICCONNECTIONS_l","op":"GREATER_THAN","value":5}],"maxResults":10}`, nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("filter status %d", rec.Code)
	}
	body := rec.Body.String()
	el := stj.MustParse(rec.Body.Bytes())
	if el.GetProperty("count").GetInt32() != 1 {
		t.Fatalf("filter count: %s", body)
	}
	sess := el.GetProperty("sessions").EnumerateArray()[0]
	lu := sess.GetProperty("lastUpdated").Str()
	want := `{"count":1,"sessions":[{"deployment":"e203ea2f9d70483fb6149107d52c25b5","id":"sA","bucket":"A2_4729","settings":{"maxPublicPlayers":10,"allowInvites":false,"shouldAdvertise":true,"allowReadById":true,"allowJoinViaPresence":true,"allowJoinInProgress":true,"allowConferenceRoom":false,"checkSanctions":false,"allowMigration":false,"rejoinAfterKick":"","platforms":null},"maxPublicPlayers":10,"openPublicPlayers":8,"maxPrivatePlayers":0,"openPrivatePlayers":0,"publicPlayers":["p1","p2"],"privatePlayers":[],"totalPlayers":2,"allowJoinInProgress":true,"shouldAdvertise":true,"isDedicated":true,"usesStats":false,"allowInvites":false,"usesPresence":false,"allowJoinViaPresence":true,"allowJoinViaPresenceFriendsOnly":false,"buildUniqueId":4729,"started":true,"lastUpdated":"` + lu + `","attributes":{"SERVERNAME_s":"Alpha","BUSESSTATS_b":false,"NUMPRIVATECONNECTIONS_l":0,"USESPRESENCE_b":false,"ADDRESS_s":"10.0.0.1","PRESENCESEARCH_b":true,"NUMPUBLICCONNECTIONS_l":10,"PUBLICPORT_s":"7777","BUILDUNIQUEID_l":4729,"BANTICHEATPROTECTED_b":true,"BISDEDICATED_b":true,"IMGUIPORT_s":""},"owner":"Client_xyza7891d295O6HPItjLVgKsuU06DkR0","ownerPlatformId":null}]}`
	if body != want {
		t.Errorf("filter body\n got: %s\nwant: %s", body, want)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", lu); err != nil {
		t.Errorf("lastUpdated format %q", lu)
	}

	// invalid JSON -> EOS error
	rec = do(t, gw.HTTP, "POST", "/matchmaking/v1/x/filter", "application/json", `{`, nil)
	if rec.Code != 400 || rec.Body.String() != `{"errorCode":"errors.com.epicgames.common.bad_request","errorMessage":"Invalid JSON body","numericErrorCode":1001,"messageVarList":[],"correlationId":"`+CORRELATION_ID+`"}` {
		t.Errorf("bad json: %d %s", rec.Code, rec.Body.String())
	}

	// whitelist hides StationB from strangers but not from listed users
	SetStationWhitelist("stationb", []string{" Carol ", ""})
	defer SetStationWhitelist("stationb", nil)
	rec = do(t, gw.HTTP, "POST", "/matchmaking/v1/x/filter", "application/json", `{}`, nil)
	if stj.MustParse(rec.Body.Bytes()).GetProperty("count").GetInt32() != 1 {
		t.Errorf("whitelist should hide StationB: %s", rec.Body.String())
	}
	rec = do(t, gw.HTTP, "POST", "/matchmaking/v1/x/filter", "application/json", `{}`, map[string]string{"Authorization": bearerFor("", "carol")})
	if stj.MustParse(rec.Body.Bytes()).GetProperty("count").GetInt32() != 2 {
		t.Errorf("whitelisted user should see StationB: %s", rec.Body.String())
	}
	rec = do(t, gw.HTTP, "POST", "/matchmaking/v1/x/filter", "application/json", `{}`, map[string]string{"Authorization": bearerFor(ProductUserIdFor("Carol"), "")})
	if stj.MustParse(rec.Body.Bytes()).GetProperty("count").GetInt32() != 2 {
		t.Errorf("whitelisted puid should see StationB: %s", rec.Body.String())
	}

	// join/leave session + player count reconciliation
	do(t, gw.HTTP, "POST", "/matchmaking/v1/x/sessions/sA/publicplayers/p3", "", "", nil)
	if got := SessionsSnapshot()[1].PublicPlayers; strings.Join(got, ",") != "p1,p2,p3" {
		t.Errorf("join: %v", got)
	}
	rec = do(t, gw.HTTP, "DELETE", "/matchmaking/v1/x/sessions/sA/publicplayers/p1", "", "", nil)
	if rec.Code != 200 || rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("leave response %d %q", rec.Code, rec.Body.String())
	}
	if !SetSessionPlayerCount("depA", 1) || strings.Join(SessionsSnapshot()[1].PublicPlayers, ",") != "p3" {
		t.Errorf("SetSessionPlayerCount: %v", SessionsSnapshot()[1].PublicPlayers)
	}
	if SetSessionPlayerCount("nope", 1) || SetSessionPlayerCount(" ", 1) {
		t.Error("SetSessionPlayerCount unknown deployment")
	}

	// [Solara] moving servers: joining sB without leaving sA takes p3 off sA (one server at a time)
	do(t, gw.HTTP, "POST", "/matchmaking/v1/x/sessions/sB/publicplayers/p3", "", "", nil)
	if a, b := SessionsSnapshot()[1].PublicPlayers, SessionsSnapshot()[0].PublicPlayers; len(a) != 0 || strings.Join(b, ",") != "p3" {
		t.Errorf("join elsewhere: sA=%v sB=%v", a, b)
	}
	do(t, gw.HTTP, "POST", "/matchmaking/v1/x/sessions/sB/publicplayers/p3", "", "", nil) // re-join: still listed once
	if b := SessionsSnapshot()[0].PublicPlayers; strings.Join(b, ",") != "p3" {
		t.Errorf("re-join: sB=%v", b)
	}

	// prune keeps the newest session of each live deployment
	a2 := models.NewEosSessionInfo()
	a2.Id, a2.DeploymentId = "sA2", s("depA")
	UpsertSession(a2)
	if n := PruneSessionsExcept(map[string]bool{"depA": true}); n != 2 {
		t.Errorf("prune removed %d", n)
	}
	if snap := SessionsSnapshot(); len(snap) != 1 || snap[0].Id != "sA2" {
		t.Errorf("prune survivors %v", snap)
	}
	if RemoveSessionsByDeployment("depA") != 1 || RemoveSession("sA2") {
		t.Error("remove by deployment")
	}
}

func TestOAuthToken(t *testing.T) {
	gw := newEosGatewayServer()
	form := "application/x-www-form-urlencoded"

	rec := do(t, gw.HTTP, "POST", "/auth/v1/oauth/token", form, "grant_type=client_credentials", nil)
	if rec.Code != 200 {
		t.Fatalf("client_credentials %d %s", rec.Code, rec.Body.String())
	}
	el := stj.MustParse(rec.Body.Bytes())
	var keys []string
	for _, p := range el.EnumerateObject() {
		keys = append(keys, p.Name)
	}
	if strings.Join(keys, ",") != "access_token,token_type,expires_at,expires_in,features,organization_id,product_id,sandbox_id,deployment_id" {
		t.Errorf("client token keys %v", keys)
	}
	if el.GetProperty("expires_in").GetInt32() != 3599 || el.GetProperty("token_type").Str() != "bearer" {
		t.Error("client token values")
	}
	verifyJWT(t, el.GetProperty("access_token").Str())

	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("myclient:pw"))
	rec = do(t, gw.HTTP, "POST", "/auth/v1/oauth/token", form,
		"grant_type=external_auth&external_auth_type=oculus_userid_nonce&display_name=Alice&nonce=N1", map[string]string{"Authorization": basic})
	el = stj.MustParse(rec.Body.Bytes())
	keys = nil
	for _, p := range el.EnumerateObject() {
		keys = append(keys, p.Name)
	}
	if strings.Join(keys, ",") != "access_token,token_type,expires_at,expires_in,nonce,features,organization_id,product_id,sandbox_id,deployment_id,organization_user_id,product_user_id,product_user_id_created,id_token,client_id" {
		t.Errorf("user token keys %v", keys)
	}
	if el.GetProperty("product_user_id").Str() != ProductUserIdFor("Alice") || el.GetProperty("client_id").Str() != "myclient" ||
		el.GetProperty("nonce").Str() != "N1" || el.GetProperty("expires_in").GetInt32() != 315360000 {
		t.Errorf("oculus token: %s", rec.Body.String())
	}
	claims := verifyJWT(t, el.GetProperty("access_token").Str())
	if claims.GetProperty("account").GetProperty("displayName").Str() != "Alice" {
		t.Error("displayName claim")
	}

	rec = do(t, gw.HTTP, "POST", "/auth/v1/oauth/token", form, "grant_type=external_auth&external_auth_type=steam_encrypted_appticket&external_auth_token=abcdef0123", nil)
	el = stj.MustParse(rec.Body.Bytes())
	if el.GetProperty("product_user_id").Str() != ProductUserIdFor(oracle["SteamIdFromTicket.abc"]) {
		t.Errorf("steam puid: %s", rec.Body.String())
	}

	rec = do(t, gw.HTTP, "POST", "/auth/v1/oauth/token", form, "grant_type=external_auth&external_auth_type=xbox", nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"errorCode":"errors.com.epicgames.common.oauth.invalid_request","errorMessage":"Unsupported external_auth_type: xbox","numericErrorCode":1015`) {
		t.Errorf("unsupported type: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(t, gw.HTTP, "POST", "/auth/v1/oauth/token", form, "grant_type=password", nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), `"numericErrorCode":1016`) {
		t.Errorf("unsupported grant: %d %s", rec.Code, rec.Body.String())
	}

	// device id login registers the display name used by product-user search
	rec = do(t, gw.HTTP, "POST", "/auth/v1/accounts/deviceid", form, "deviceModel=Quest&displayName=Dev&nonce=abc", nil)
	el = stj.MustParse(rec.Body.Bytes())
	if el.GetProperty("token_type").Str() != "bearer" {
		t.Fatalf("deviceid: %s", rec.Body.String())
	}
	puid := ProductUserIdFor("device:abc")
	rec = do(t, gw.HTTP, "POST", "/user/v3/product-users/search", "application/json", `{"productUserIds":["`+puid+`","0002ffffffff","",null,"short"]}`, nil)
	body := rec.Body.String()
	if !strings.HasPrefix(body, `{"productUsers":{"`+puid+`":{"accounts":[{"accountId":"`+puid+`","displayName":"Dev","identityProviderId":"deviceid","lastLogin":"`) ||
		!strings.Contains(body, `"0002ffffffff":{"accounts":[{"accountId":"0002ffffffff","displayName":"0002ffff"`) ||
		strings.Contains(body, `"short"`) {
		t.Errorf("product user search: %s", body)
	}

	// TURN credentials: HMAC-SHA1 of "expiry:puid"
	rec = do(t, gw.HTTP, "POST", "/auth/v1/turn/credentials", "", "", map[string]string{"Authorization": bearerFor("0002abc", "")})
	el = stj.MustParse(rec.Body.Bytes())
	if !strings.HasSuffix(el.GetProperty("username").Str(), ":0002abc") || el.GetProperty("ttl").GetInt32() != 3600000 ||
		len(el.GetProperty("uris").EnumerateArray()) != 3 {
		t.Errorf("turn: %s", rec.Body.String())
	}
}

func TestLobbyHTTP(t *testing.T) {
	gw := newEosGatewayServer()
	auth := map[string]string{"Authorization": bearerFor("0002owner", "")}
	rec := do(t, gw.HTTP, "POST", "/lobby/v1/d/lobbies", "application/json", `{"bucket":"bk","settings":{"maxPublicPlayers":4},"attributes":{"MODE_s":"x"}}`, auth)
	if rec.Code != 200 {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	env := stj.MustParse(rec.Body.Bytes())
	pd := env.GetProperty("publicData")
	id := pd.GetProperty("id").Str()
	if pd.GetProperty("owner").Str() != "0002owner" || pd.GetProperty("openPublicPlayers").GetInt32() != 3 ||
		!strings.HasSuffix(rec.Body.String(), `,"privateData":null,"memberData":null,"room":null,"requestId":null,"name":"lobbyinfo"}`) {
		t.Errorf("create envelope %s", rec.Body.String())
	}

	rec = do(t, gw.HTTP, "GET", "/lobby/v1/d/lobbies/"+id, "", "", nil)
	if rec.Code != 200 {
		t.Fatalf("get %d", rec.Code)
	}
	rec = do(t, gw.HTTP, "GET", "/lobby/v1/d/lobbies/missing", "", "", nil)
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), `"errorMessage":"Lobby missing not found","numericErrorCode":10069`) {
		t.Errorf("get missing %d %s", rec.Code, rec.Body.String())
	}

	rec = do(t, gw.HTTP, "POST", "/lobby/v1/d/lobbies/filter", "application/json", `{"criteria":[{"key":"attributes.MODE_s","op":"EQUAL","value":"x"}],"minCurrentPlayers":1}`, nil)
	if stj.MustParse(rec.Body.Bytes()).GetProperty("count").GetInt32() < 1 {
		t.Errorf("lobby filter %s", rec.Body.String())
	}

	rec = do(t, gw.HTTP, "PUT", "/lobby/v1/d/lobbies/"+id, "application/json", `{"attributes":{"ADDRESS_s":"1.2.3.4"},"started":true}`, nil)
	pd = stj.MustParse(rec.Body.Bytes()).GetProperty("publicData")
	if pd.GetProperty("attributes").GetRawText() != `{"MODE_s":"x","ADDRESS_s":"1.2.3.4"}` || !pd.GetProperty("started").GetBoolean() {
		t.Errorf("update %s", rec.Body.String())
	}
	if data, _ := os.ReadFile(filepath.Join(BaseDirectory, "lobbies.json")); !strings.Contains(string(data), id) {
		t.Error("lobbies.json should hold the ADDRESS_s lobby")
	}

	do(t, gw.HTTP, "POST", "/lobby/v1/d/lobbies/"+id+"/members/0002guest", "", "", nil)
	rec = do(t, gw.HTTP, "GET", "/lobby/v1/d/lobbies/"+id, "", "", nil)
	if !strings.Contains(rec.Body.String(), `"publicPlayers":["0002owner","0002guest"]`) {
		t.Errorf("join %s", rec.Body.String())
	}
	do(t, gw.HTTP, "DELETE", "/lobby/v1/d/lobbies/"+id+"/members/0002owner", "", "", nil)
	if rec = do(t, gw.HTTP, "GET", "/lobby/v1/d/lobbies/"+id, "", "", nil); rec.Code != 404 {
		t.Error("owner leaving destroys the lobby")
	}
}

func TestTelemetryEndpoints(t *testing.T) {
	gw := newEosGatewayServer()
	rec := do(t, gw.HTTP, "POST", "/datarouter/api/v1/public/data/clients", "application/json",
		`{"Events":[{"EventName":"dr.ev","EventData":{ "k" : 1 }}]}`, nil)
	if rec.Code != 200 || rec.Body.String() != "{}" {
		t.Fatalf("datarouter %d %s", rec.Code, rec.Body.String())
	}
	got := GetTelemetry(1)
	if got[0].EventName != "dr.ev" || got[0].Payload != `{"EventName":"dr.ev","EventNamespace":null,"DateOffset":null,"EventData":{"k":1}}` {
		t.Errorf("datarouter telemetry %+v", got[0])
	}
	do(t, gw.HTTP, "POST", "/telemetry/data/datarouter/api/v1/public/data", "", "raw-body", nil)
	if got := GetTelemetry(1); got[0].EventName != "telemetry.data" || got[0].Payload != "raw-body" {
		t.Errorf("telemetry %+v", got[0])
	}
	rec = do(t, gw.HTTP, "POST", "/epic/presence/internal/v1/_/acc/presence/conn", "application/json", `{"status":"online"}`, nil)
	if rec.Code != 200 || rec.Body.String() != "{}" || rec.Header().Get("X-Epic-Correlation-Id") != "" {
		t.Errorf("presence %d %s", rec.Code, rec.Body.String())
	}
}

func TestPlayerDataStorage(t *testing.T) {
	os.RemoveAll(filepath.Join(BaseDirectory, "playerdata"))
	gw := newEosGatewayServer()
	rec := do(t, gw.HTTP, "GET", "/datastorage/v2/access/deployment/d1/private/u1/save.bin", "", "", nil)
	body := rec.Body.String()
	if !strings.HasPrefix(body, `{"accountId":"u1","deploymentId":"d1","writeRequests":[{"id":"`) ||
		!strings.HasSuffix(body, `"uri":"http://localhost:50/datastorage/v2/data/deployment/d1/private/u1/save.bin"}],"readRequests":[]}`) {
		t.Errorf("access (missing) %s", body)
	}
	if rec = do(t, gw.HTTP, "GET", "/datastorage/v2/data/deployment/d1/private/u1/save.bin", "", "", nil); rec.Code != 404 {
		t.Errorf("get missing %d", rec.Code)
	}
	if rec = do(t, gw.HTTP, "PUT", "/datastorage/v2/data/deployment/d1/private/u1/save.bin", "", "hello", nil); rec.Code != 200 {
		t.Errorf("put %d", rec.Code)
	}
	rec = do(t, gw.HTTP, "GET", "/datastorage/v2/data/deployment/d1/private/u1/save.bin", "", "", nil)
	if rec.Body.String() != "hello" || rec.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("get %q", rec.Body.String())
	}
	rec = do(t, gw.HTTP, "GET", "/datastorage/v2/access/deployment/d1/private/u1/save.bin", "", "", nil)
	if !strings.Contains(rec.Body.String(), `"deviceId":null,"uri":`) ||
		!strings.Contains(rec.Body.String(), `"file":{"filename":"save.bin","md5Hash":"5d41402abc4b2a76b9719d911017c592","lastModified":"`) {
		t.Errorf("access (exists) %s", rec.Body.String())
	}
}

func TestEosWsServerRoot(t *testing.T) {
	ws := newEosWsServer()
	if rec := do(t, ws.HTTP, "GET", "/", "", "", nil); rec.Code != 404 || rec.Body.Len() != 0 {
		t.Errorf("root %d", rec.Code)
	}
}

// ── STOMP ───────────────────────────────────────────────────────────────────

func TestStompFrames(t *testing.T) {
	enc := EncodeFrame("CONNECTED", StompHeaders{{"version", "1.2"}, {"session", "abc"}}, "")
	if string(enc) != "CONNECTED\nversion:1.2\nsession:abc\n\n\x00" {
		t.Errorf("encode %q", enc)
	}
	cmd, hdrs, body := ParseFrame(" SUBSCRIBE \r\nid: 1\r\nReceipt:r-1\r\nreceipt:r-2\r\nbad\r\n:x\r\n\r\nline1\nline2\x00")
	if cmd != "SUBSCRIBE" || body != "line1\nline2" {
		t.Errorf("parse cmd=%q body=%q", cmd, body)
	}
	if headerString(hdrs) != "id:1, Receipt:r-2" {
		t.Errorf("headers %q", headerString(hdrs))
	}
	if v, ok := hdrs.TryGetValue("RECEIPT"); !ok || v != "r-2" {
		t.Error("case-insensitive header lookup")
	}
	if _, _, body := ParseFrame("CONNECT\naccept-version:1.2"); body != "" {
		t.Error("no blank line -> empty body")
	}
	if !IsStompPath("/Notifications/v1/x/CONNECT") || IsStompPath("/notifications/v2/connect") {
		t.Error("IsStompPath")
	}
	if !IsLobbyPath("/lobby/v1/d/lobbies/connect") || IsLobbyPath("/lobby/v1/d/lobbies") {
		t.Error("IsLobbyPath")
	}
}

func wsURL(srv *httptest.Server, path string) string {
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = path
	return u.String()
}

func TestStompWebSocket(t *testing.T) {
	gw := newEosGatewayServer()
	srv := httptest.NewServer(gw.HTTP)
	defer srv.Close()

	d := websocket.Dialer{Subprotocols: []string{"v12.stomp", "v11.stomp"}}
	c, resp, err := d.Dial(wsURL(srv, "/notifications/v1/x/connect"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if resp.Header.Get("Sec-WebSocket-Protocol") != "v12.stomp" {
		t.Errorf("subprotocol %q", resp.Header.Get("Sec-WebSocket-Protocol"))
	}
	c.WriteMessage(websocket.TextMessage, []byte("CONNECT\naccept-version:1.2\n\n\x00"))
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	m := string(msg)
	if !strings.HasPrefix(m, "CONNECTED\nversion:1.2\nsession:") || !strings.HasSuffix(m, "\nheart-beat:0,0\nserver:EOS/1.0\n\n\x00") {
		t.Errorf("CONNECTED frame %q", m)
	}
	c.WriteMessage(websocket.TextMessage, []byte("SUBSCRIBE\nid:0\nreceipt:77\n\n\x00"))
	_, msg, _ = c.ReadMessage()
	if string(msg) != "RECEIPT\nreceipt-id:77\n\n\x00" {
		t.Errorf("RECEIPT frame %q", msg)
	}
	c.WriteMessage(websocket.TextMessage, []byte("DISCONNECT\nreceipt:9\n\n\x00"))
	sawReceipt, sawClose := false, false
	for i := 0; i < 4; i++ {
		_, msg, err = c.ReadMessage()
		if err != nil {
			if ce, ok := err.(*websocket.CloseError); ok && ce.Code == 1000 && ce.Text == "bye" {
				sawClose = true
			}
			break
		}
		if string(msg) == "RECEIPT\nreceipt-id:9\n\n\x00" {
			sawReceipt = true
		}
	}
	if !sawReceipt || !sawClose {
		t.Errorf("disconnect receipt=%v close=%v", sawReceipt, sawClose)
	}
}

// ── Lobby WebSocket ─────────────────────────────────────────────────────────

func readJSON(t *testing.T, c *websocket.Conn) stj.Element {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	return stj.MustParse(msg)
}

func TestLobbyWebSocket(t *testing.T) {
	gw := newEosGatewayServer()
	srv := httptest.NewServer(gw.HTTP)
	defer srv.Close()

	// unauthenticated lobby connections are refused with 401
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(srv, "/lobby/v1/d/lobbies/connect"), nil)
	if err == nil || resp == nil || resp.StatusCode != 401 {
		t.Fatalf("expected 401, got %v %v", resp, err)
	}

	owner, _, err := websocket.DefaultDialer.Dial(wsURL(srv, "/lobby/v1/d/lobbies/connect"), http.Header{"Authorization": {bearerFor("0002owner", "")}})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	owner.WriteMessage(websocket.TextMessage, []byte(`{"name":"create","requestId":"r1","payload":{"options":{"maxPlayers":4,"shouldAdvertise":true}}}`))
	info := readJSON(t, owner)
	if info.GetProperty("name").Str() != "lobbyinfo" || info.GetProperty("requestId").Str() != "r1" {
		t.Fatalf("create reply %s", info.GetRawText())
	}
	p := info.GetProperty("payload")
	lobbyId := p.GetProperty("publicData").GetProperty("id").Str()
	var keys []string
	for _, pr := range p.EnumerateObject() {
		keys = append(keys, pr.Name)
	}
	if strings.Join(keys, ",") != "publicData,privateData,memberData,room,requestId,name" ||
		p.GetProperty("memberData").GetRawText() != `{"memberData":{"0002owner":{"data":{},"platformId":4000,"platform":"steam"}}}` ||
		p.GetProperty("publicData").GetProperty("settings").GetRawText() != `{"maxPublicPlayers":4,"allowInvites":false,"shouldAdvertise":true,"allowReadById":false,"allowJoinViaPresence":false,"allowJoinInProgress":false,"allowConferenceRoom":false,"checkSanctions":false,"allowMigration":true,"rejoinAfterKick":"open","platforms":null}` {
		t.Errorf("lobbyinfo payload %s", p.GetRawText())
	}
	pk := ""
	for _, pr := range p.GetProperty("privateData").EnumerateObject() {
		pk += pr.Name + ","
	}
	if pk != "index,lock,invites,historicalPlayers,pendingDelete," {
		t.Errorf("privateData keys %s", pk)
	}

	guest, _, err := websocket.DefaultDialer.Dial(wsURL(srv, "/lobby/v1/d/lobbies/connect"), http.Header{"Authorization": {bearerFor("0002guest", "")}})
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	guest.WriteMessage(websocket.TextMessage, []byte(`{"name":"join","requestId":"r2","payload":{"lobbyId":"`+lobbyId+`"}}`))
	ginfo := readJSON(t, guest)
	room := ginfo.GetProperty("payload").GetProperty("room")
	if room.GetProperty("roomName").Str() != "lobby-"+lobbyId || room.GetProperty("clientBaseUrl").Str() != SERVER_URL {
		t.Errorf("join room %s", room.GetRawText())
	}
	if ginfo.GetProperty("payload").GetProperty("privateData").ValueKind() != stj.Null {
		t.Error("join privateData should be null")
	}
	joined := readJSON(t, owner)
	if joined.GetRawText() != `{"name":"memberjoined","payload":{"lobbyId":"`+lobbyId+`","puid":"0002guest","platformId":4000,"platform":"steam","allowCrossplay":true,"name":"memberjoined"}}` {
		t.Errorf("memberjoined %s", joined.GetRawText())
	}
	readJSON(t, guest) // the joiner receives its own memberjoined broadcast

	guest.WriteMessage(websocket.TextMessage, []byte(`{"name":"memberdata","requestId":"r3","payload":{"lobbyId":"`+lobbyId+`","data":{"READY_b":true,"LVL_l":7,"F_d":1.5,"N_s":"x"}}}`))
	if ok := readJSON(t, guest); ok.GetRawText() != `{"name":"success","payload":{"requestId":"r3","name":"success"},"requestId":"r3"}` {
		t.Errorf("success %s", ok.GetRawText())
	}
	mdc := readJSON(t, owner)
	if mdc.GetRawText() != `{"name":"memberdatachange","payload":{"lobbyId":"`+lobbyId+`","puid":"0002guest","memberData":{"data":{"READY_b":true,"LVL_l":7,"F_d":1.5,"N_s":"x"},"platformId":4000,"platform":"other"},"name":"memberdatachange"}}` {
		t.Errorf("memberdatachange %s", mdc.GetRawText())
	}
	readJSON(t, guest)

	owner.WriteMessage(websocket.TextMessage, []byte(`{"name":"bogus","requestId":"r4"}`))
	if e := readJSON(t, owner); e.GetRawText() != `{"name":"error","payload":{"statusCode":400,"errorCode":"errors.com.epicgames.lobbies.bad_request","errorMessage":"unknown frameType `+apos+`bogus","requestId":"r4","name":"error"},"requestId":"r4"}` {
		t.Errorf("unknown frame %s", e.GetRawText())
	}
	owner.WriteMessage(websocket.TextMessage, []byte(`not json`))
	if e := readJSON(t, owner); e.GetProperty("payload").GetProperty("errorMessage").Str() != "could not parse json" {
		t.Errorf("parse error %s", e.GetRawText())
	}

	owner.WriteMessage(websocket.TextMessage, []byte(`{"name":"delete","requestId":"r5","payload":{"lobbyId":"`+lobbyId+`"}}`))
	del := readJSON(t, owner)
	if del.GetRawText() != `{"name":"lobbydeleted","payload":{"lobbyId":"`+lobbyId+`","name":"lobbydeleted"}}` {
		t.Errorf("lobbydeleted %s", del.GetRawText())
	}
	if ok := readJSON(t, owner); ok.GetProperty("name").Str() != "success" {
		t.Errorf("delete success %s", ok.GetRawText())
	}
	owner.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, _, err := owner.ReadMessage(); err == nil {
		t.Error("server should close after delete")
	} else if ce, ok := err.(*websocket.CloseError); !ok || ce.Code != 1000 || ce.Text != "bye" {
		t.Errorf("close: %v", err)
	}
}

func TestLobbyReaper(t *testing.T) {
	l := models.NewEosLobbySession()
	l.Id = "stale-lobby"
	l.LastUpdated = time.Now().UTC().Add(-2 * time.Minute).Format("2006-01-02T15:04:05.000Z")
	keep := models.NewEosLobbySession()
	keep.Id = "infra-lobby"
	keep.LastUpdated = l.LastUpdated
	keep.Attributes.Set("ADDRESS_s", s("1.1.1.1"))
	withLobbyLock(func() { Lobbies = append(Lobbies, l, keep) })
	reapLobbies(time.Now().UTC())
	withLobbyLock(func() {
		if findLobby("stale-lobby") != nil || findLobby("infra-lobby") == nil {
			t.Error("reaper")
		}
		removeLobby(findLobby("infra-lobby"))
	})
}
