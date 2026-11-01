// Tests for the AresDashboardServer port (DashboardServer.cs,
// DashboardAuthPreprocessor.cs, the dashboard half of DrafterPermissions.cs).
package ares

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/logging"
	"solara/internal/stj"
)

const (
	testDashAdminKey = "test-admin-key-0123456789"
	testDashSecret   = "test-session-secret-abcdef"
)

func init() { logging.DisableFiles = true }

// resetDashAuth re-runs the preprocessor's type initialisation with the given env.
func resetDashAuth(t *testing.T, enforce *string) {
	t.Helper()
	t.Setenv("DASHBOARD_ADMIN_KEY", testDashAdminKey)
	t.Setenv("DASHBOARD_SESSION_SECRET", testDashSecret)
	if enforce == nil {
		t.Setenv("DASHBOARD_AUTH_ENFORCE", "")
		os.Unsetenv("DASHBOARD_AUTH_ENFORCE")
	} else {
		t.Setenv("DASHBOARD_AUTH_ENFORCE", *enforce)
	}
	dashboardAuthOnce = sync.Once{}
	dashboardAuthTypeInit()
}

// withTempDB points ares.Database at a fresh store and the working directory
// at a temp dir (stores such as NetvarOverridesFile write relative to it).
func withTempDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 1, Name: "t", Path: filepath.Join(dir, "t"+docdb.FileSuffix)})
	if err != nil {
		t.Fatal(err)
	}
	old := Database
	Database = db
	wd, _ := os.Getwd()
	_ = os.Chdir(dir)
	t.Cleanup(func() {
		Database = old
		_ = db.Close()
		_ = os.Chdir(wd)
	})
}

func do(t *testing.T, s *AresDashboardServer, method, target, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.HTTP.ServeHTTP(w, r)
	return w
}

var adminHdr = map[string]string{"x-dashboard-key": testDashAdminKey}

func TestDashboardRouteTable(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "docs", "original_routes.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var want []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cols := strings.Split(sc.Text(), "\t")
		if len(cols) == 4 && cols[0] == "AresDashboardServer" {
			want = append(want, cols[1]+" "+cols[2]+" "+cols[3])
		}
	}
	s := newAresDashboardServer()
	var got []string
	for _, e := range s.HTTP.Endpoints() {
		got = append(got, e.Method+" "+e.RouteUri+" "+e.Name)
	}
	if len(want) != 53 {
		t.Fatalf("expected 53 original routes, found %d", len(want))
	}
	// The 53 original routes come first and in order; Solara's additions
	// (SEC-SID server-token API) are registered after them.
	extra := []string{"GET /api/server-tokens ListServerTokens", "DELETE /api/server-tokens/{id} RevokeServerToken",
		"GET /api/game_bans Solara/GetGameBans", "POST /api/game_bans/{player_id}/unban Solara/LiftGameBans",
		"GET /api/catalog/items Solara/GetGrantableItems", "GET /api/users/{user_id}/inventory Solara/GetUserInventory",
		"POST /api/users/{user_id}/inventory Solara/GrantUserItem", "DELETE /api/users/{user_id}/inventory/{item_id} Solara/RevokeUserItem"}
	if len(got) != len(want)+len(extra) || strings.Join(got[:len(want)], "\n") != strings.Join(want, "\n") ||
		strings.Join(got[len(want):], "\n") != strings.Join(extra, "\n") {
		t.Fatalf("route table mismatch:\n got %v\nwant %v + %v", got, want, extra)
	}
}

func TestDashboardIndexHtmlHardened(t *testing.T) {
	// SEC-DASH-6: the embedded page deliberately differs from the original
	// (output encoding, sessionStorage). Assert the hardening, not identity.
	page := dashboardHtml
	for _, must := range []string{"function esc(", "function jsArg(", "function safeUrl(", "sessionStorage.getItem(DASH_KEY_STORAGE)", "localStorage.removeItem(DASH_KEY_STORAGE)"} {
		if !strings.Contains(page, must) {
			t.Errorf("index.html missing %q", must)
		}
	}
	if strings.Contains(page, "localStorage.setItem") {
		t.Error("admin key must not be persisted in localStorage")
	}
	// every value interpolated into the HTML tables goes through an encoder
	for _, raw := range []string{"${u.user_id}", "${u.username}", "${s.station_name}", "${s.station_id}", "${e.event_name}", "${d.deployment_name", "${nm}", "${url}", "${k}"} {
		if strings.Contains(page, raw) {
			t.Errorf("raw interpolation %s still present", raw)
		}
	}
	resetDashAuth(t, nil)
	s := newAresDashboardServer()
	w := do(t, s, "GET", "/", "", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "text/html" || w.Body.String() != page {
		t.Fatalf("GET / = %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("security headers missing: %v", w.Header())
	}
}

func mintSession(secret string, payload string) string {
	h := dashboardB64Url([]byte(`{"alg":"HS256","typ":"JWT"}`))
	p := dashboardB64Url([]byte(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h + "." + p))
	return h + "." + p + "." + dashboardB64Url(mac.Sum(nil))
}

func TestDashboardAuthPreprocessor(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	if !DashboardAuthPreprocessorEnforced() {
		t.Fatal("Enforce must default to ON")
	}
	s := newAresDashboardServer()

	// Public paths pass without credentials.
	for _, p := range []string{"/", "/auth/login", "/auth/meta?code=1", "/apix", "/api"} {
		if w := do(t, s, "GET", p, "", nil); w.Code == 401 {
			t.Errorf("%s must be public, got 401", p)
		}
	}
	// Protected: 401 JSON, no CORS headers (written before EnableCors).
	w := do(t, s, "GET", "/api/stats", "", nil)
	if w.Code != 401 || w.Body.String() != `{"error":"unauthorized"}` || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("blocked = %d %q %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("blocked response must not carry CORS headers")
	}
	// Case-insensitive guard prefix; unknown /api route still guarded.
	if w := do(t, s, "GET", "/API/stats", "", nil); w.Code != 401 {
		t.Errorf("/API/stats = %d", w.Code)
	}
	// CORS preflight always allowed.
	if w := do(t, s, "OPTIONS", "/api/stats", "", nil); w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("OPTIONS = %d", w.Code)
	}
	// Admin key header (as the Next.js proxy sends it) and the x-api-key fallback.
	if w := do(t, s, "GET", "/api/stats", "", adminHdr); w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("x-dashboard-key = %d", w.Code)
	}
	if w := do(t, s, "GET", "/api/stats", "", map[string]string{"x-api-key": testDashAdminKey}); w.Code != 200 {
		t.Errorf("x-api-key = %d", w.Code)
	}
	if w := do(t, s, "GET", "/api/stats", "", map[string]string{"x-dashboard-key": "wrong", "x-api-key": testDashAdminKey}); w.Code != 401 {
		t.Errorf("present x-dashboard-key must win over x-api-key, got %d", w.Code)
	}
	// Session cookie: valid, expired, no exp, tampered, wrong secret, bad shape.
	future := time.Now().Unix() + 3600
	cases := []struct {
		cookie string
		ok     bool
	}{
		{"a2dash_session=" + mintSession(testDashSecret, `{"sub":"x","exp":`+itoa(future)+`}`), true},
		{"foo=bar; a2dash_session=" + mintSession(testDashSecret, `{"sub":"x"}`), true},
		{"a2dash_session=" + mintSession(testDashSecret, `{"exp":1}`), false},
		{"a2dash_session=" + mintSession(testDashSecret, `{"exp":"soon"}`), false},
		{"a2dash_session=" + mintSession("other", `{"sub":"x"}`), false},
		{"a2dash_session=" + mintSession(testDashSecret, `[1]`), false},
		{"a2dash_session=a.b", false},
		{"A2DASH_SESSION=" + mintSession(testDashSecret, `{"sub":"x"}`), false},
	}
	for i, tc := range cases {
		w := do(t, s, "GET", "/api/stats", "", map[string]string{"Cookie": tc.cookie})
		if (w.Code == 200) != tc.ok {
			t.Errorf("cookie case %d: got %d want ok=%v", i, w.Code, tc.ok)
		}
	}

	// Enforce off: log-only, request proceeds.
	off := "0"
	resetDashAuth(t, &off)
	if w := do(t, s, "GET", "/api/stats", "", nil); w.Code != 200 {
		t.Errorf("enforce=0: got %d", w.Code)
	}
	// Any value other than "0" enforces.
	other := "false"
	resetDashAuth(t, &other)
	if w := do(t, s, "GET", "/api/stats", "", nil); w.Code != 401 {
		t.Errorf("enforce=false must still enforce, got %d", w.Code)
	}
}

func itoa(n int64) string { return stj.MarshalString(n) }

func TestDashboardResolveSecretGenerates(t *testing.T) {
	old := DashboardSecretsDir
	DashboardSecretsDir = filepath.Join(t.TempDir(), "config")
	defer func() { DashboardSecretsDir = old }()
	t.Setenv("DASHBOARD_ADMIN_KEY", "CHANGE-ME-please")
	k := dashboardResolveSecret("DASHBOARD_ADMIN_KEY", "admin key")
	if len(k) != 64 || k == "CHANGE-ME-please" {
		t.Fatalf("generated key = %q", k)
	}
	t.Setenv("DASHBOARD_ADMIN_KEY", "  ")
	if k2 := dashboardResolveSecret("DASHBOARD_ADMIN_KEY", "admin key"); len(k2) != 64 || k2 == k {
		t.Fatalf("whitespace must generate a fresh key, got %q", k2)
	}
	t.Setenv("DASHBOARD_ADMIN_KEY", "real")
	if dashboardResolveSecret("DASHBOARD_ADMIN_KEY", "admin key") != "real" {
		t.Fatal("env value must be used")
	}
}

func TestDashboardAuthLoginHtml(t *testing.T) {
	resetDashAuth(t, nil)
	s := newAresDashboardServer()
	w := do(t, s, "GET", "/auth/login", "", nil)
	sso := "https://auth.oculus.com/sso/?redirect_uri=http%3A%2F%2Flocalhost%3A3000%2Fauth%2Fmeta&organization_id=1458804674917108"
	want := "<!doctype html><meta http-equiv='refresh' content='0;url=" + sso + "'><script>location.href=" +
		stj.MarshalString(sso) + "</script>Redirecting to Meta sign-in…"
	if w.Code != 200 || w.Body.String() != want {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	if !strings.Contains(stj.MarshalString(sso), `\`+"u0026") {
		t.Fatal("STJ must escape '&' in the script literal")
	}
}

func TestDashboardStationsFlow(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()

	w := do(t, s, "POST", "/api/stations", `{"station_name":"Halcyon_1"}`, adminHdr)
	res := stj.MustParse(w.Body.Bytes())
	if w.Code != 200 || !res.GetProperty("success").GetBoolean() || res.GetProperty("error").ValueKind() != stj.Null {
		t.Fatalf("create = %d %s", w.Code, w.Body.String())
	}
	id := res.GetProperty("id").Str()
	if len(id) != 32 {
		t.Fatalf("station id %q", id)
	}
	if w := do(t, s, "POST", "/api/stations", `{"station_name":"  "}`, adminHdr); w.Body.String() != `{"success":false,"error":"station_name required","id":null}` {
		t.Fatalf("blank name = %s", w.Body.String())
	}
	if w := do(t, s, "POST", "/api/stations", `not json`, adminHdr); w.Body.String() != `{"success":false,"error":"station_name required","id":null}` {
		t.Fatalf("bad body = %s", w.Body.String())
	}

	// Deployment create (case-insensitive station lookup, as LiteDB did).
	w = do(t, s, "POST", "/api/stations/"+strings.ToUpper(id)+"/deployments", `{"deployment_name":"d1","ip":"1.2.3.4"}`, adminHdr)
	dep := stj.MustParse(w.Body.Bytes())
	if !dep.GetProperty("success").GetBoolean() || strings.Count(dep.GetProperty("api_key").Str(), ".") != 2 {
		t.Fatalf("deployment = %s", w.Body.String())
	}

	w = do(t, s, "GET", "/api/stations", "", adminHdr)
	arr := stj.MustParse(w.Body.Bytes()).EnumerateArray()
	if len(arr) != 1 || arr[0].GetProperty("deployments").GetInt32() != 1 || arr[0].GetProperty("station_name").Str() != "Halcyon_1" {
		t.Fatalf("stations = %s", w.Body.String())
	}
	keys := []string{}
	for _, p := range arr[0].EnumerateObject() {
		keys = append(keys, p.Name)
	}
	if strings.Join(keys, ",") != "station_id,station_name,created_at,online,last_online,deployments,online_deps,player_count" {
		t.Fatalf("DashboardStation order = %v", keys)
	}

	w = do(t, s, "GET", "/api/stations/"+id+"/deployments", "", adminHdr)
	if d := stj.MustParse(w.Body.Bytes()).EnumerateArray(); len(d) != 1 || d[0].GetProperty("ip_address").Str() != "1.2.3.4" {
		t.Fatalf("station deployments = %s", w.Body.String())
	}

	// Stats.
	w = do(t, s, "GET", "/api/stats", "", adminHdr)
	if w.Body.String() != `{"total_users":0,"total_stations":1,"total_deployments":1,"online_deployments":0,"total_players":0,"total_events":0}` {
		t.Fatalf("stats = %s", w.Body.String())
	}

	// Board slots: insertion-ordered dictionary of every key.
	w = do(t, s, "GET", "/api/stations/"+id+"/board", "", adminHdr)
	if !strings.HasPrefix(w.Body.String(), `{"BoardTextureUrl1":"","BoardTextureUrl2":""`) {
		t.Fatalf("board = %s", w.Body.String())
	}
	if w := do(t, s, "POST", "/api/stations/"+id+"/board?key=nope", "x", adminHdr); w.Code != 400 ||
		!strings.HasPrefix(w.Body.String(), `{"error":"unknown board key","allowed":["BoardTextureUrl1"`) {
		t.Fatalf("bad board key = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "DELETE", "/api/stations/missing/board?key=x", "", adminHdr); w.Code != 404 || w.Body.Len() != 0 {
		t.Fatalf("clear board on missing station = %d %q", w.Code, w.Body.String())
	}

	// Config: missing station and bad body.
	if w := do(t, s, "GET", "/api/stations/missing/config", "", adminHdr); w.Code != 404 || w.Body.String() != `{"error":"no such station","station_id":"missing"}` {
		t.Fatalf("config missing = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "PATCH", "/api/stations/"+id+"/config", `{}`, adminHdr); w.Code != 400 {
		t.Fatalf("empty patch = %d", w.Code)
	}
	if w := do(t, s, "PATCH", "/api/stations/"+id+"/config", `{"a":1}`, adminHdr); w.Code != 400 {
		t.Fatalf("non-string patch = %d", w.Code)
	}
	w = do(t, s, "DELETE", "/api/stations/"+id+"/config?key=nope", "", adminHdr)
	if w.Body.String() != `{"success":false,"key":"nope"}` {
		t.Fatalf("delete missing key = %s", w.Body.String())
	}

	// Delete fleet with players online needs force.
	col := docdb.GetCollection(Database, true, models.NewDeploymentDbObject)
	d := col.FindAll()[0]
	d.Online, d.PlayerCount = true, 3
	col.Update(d)
	w = do(t, s, "DELETE", "/api/stations/"+id, "", adminHdr)
	if w.Body.String() != `{"success":false,"error":"fleet has 3 player(s) online `+"\\"+"u2014"+` confirm again to delete anyway","id":null}` {
		t.Fatalf("guarded delete = %s", w.Body.String())
	}
}

func TestDashboardUsersAndModeration(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	users := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	u := models.NewUserDataResponse()
	u.UserId = dashStrPtr("U1")
	u.Username = dashStrPtr("Alice")
	u.LastLogin = dotnet.UtcNow()
	users.Insert(u)
	half := models.NewUserDataResponse() // half-written login row: skipped
	users.Insert(half)

	w := do(t, s, "GET", "/api/users", "", adminHdr)
	arr := stj.MustParse(w.Body.Bytes()).EnumerateArray()
	if len(arr) != 1 || arr[0].GetProperty("user_id").Str() != "U1" || arr[0].GetProperty("is_admin").GetBoolean() {
		t.Fatalf("users = %s", w.Body.String())
	}

	// grant_admin matches the user id case-insensitively (LiteDB collation).
	if w := do(t, s, "POST", "/api/users/u1/grant_admin", "", adminHdr); w.Body.String() != `{"success":true,"error":null,"id":null}` {
		t.Fatalf("grant = %s", w.Body.String())
	}
	w = do(t, s, "GET", "/api/users", "", adminHdr)
	if !stj.MustParse(w.Body.Bytes()).EnumerateArray()[0].GetProperty("is_admin").GetBoolean() {
		t.Fatalf("is_admin not set: %s", w.Body.String())
	}
	if n := len(docdb.GetCollection(Database, true, models.NewRoleResponse).FindAll()); n != 1 {
		t.Fatalf("global admin role rows = %d", n)
	}
	if w := do(t, s, "POST", "/api/users/nobody/grant_admin", "", adminHdr); w.Body.String() != `{"success":false,"error":"user not found","id":null}` {
		t.Fatalf("grant missing = %s", w.Body.String())
	}

	// Ban / list / unban.
	if w := do(t, s, "POST", "/api/users/U1/ban", `{"station_id":"S1","reason":"r","duration":0}`, adminHdr); !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("ban = %s", w.Body.String())
	}
	w = do(t, s, "GET", "/api/stations/S1/bans", "", adminHdr)
	if w.Body.String() != `[{"user_id":"U1","username":"Alice","station_id":"S1","reason":"r","expiration":null,"revoked":false}]` {
		t.Fatalf("bans = %s", w.Body.String())
	}
	if w := do(t, s, "GET", "/api/stations/s1/bans", "", adminHdr); w.Body.String() != `[]` {
		t.Fatalf("in-memory station filter must be case-sensitive: %s", w.Body.String())
	}
	if w := do(t, s, "PATCH", "/api/users/U1/unban", `{"station_id":"S1"}`, adminHdr); w.Body.String() != `{"success":true,"error":null,"id":null}` {
		t.Fatalf("unban = %s", w.Body.String())
	}
	if w := do(t, s, "PATCH", "/api/users/U1/unban", `{}`, adminHdr); w.Body.String() != `{"success":false,"error":"station_id required","id":null}` {
		t.Fatalf("unban no station = %s", w.Body.String())
	}

	// Roles: create, assign, members, permissions, delete.
	w = do(t, s, "POST", "/api/stations/S1/roles", `{"role_name":"Mod"}`, adminHdr)
	// LiteDB stores "" as null (EmptyStringToNull), so a description created as "" reads back null.
	roleId := stj.MustParse(w.Body.Bytes()).GetProperty("id").Str()
	if w := do(t, s, "GET", "/api/stations/S1/roles", "", adminHdr); w.Body.String() != `[{"role_id":"`+roleId+`","station_id":"S1","role_name":"Mod","role_description":null,"permissions":[]}]` {
		t.Fatalf("roles = %s", w.Body.String())
	}
	do(t, s, "POST", "/api/stations/S1/users/U1/roles/"+roleId, "", adminHdr)
	if w := do(t, s, "GET", "/api/stations/S1/members", "", adminHdr); w.Body.String() != `[{"user_id":"U1","username":"Alice","platform":null,"roles":["`+roleId+`"]}]` {
		t.Fatalf("members = %s", w.Body.String())
	}
	do(t, s, "PATCH", "/api/stations/S1/roles/"+roleId+"/permissions", `{"permissions":["a","b"]}`, adminHdr)
	if w := do(t, s, "GET", "/api/stations/S1/roles", "", adminHdr); !strings.Contains(w.Body.String(), `"permissions":["a","b"]`) {
		t.Fatalf("permissions = %s", w.Body.String())
	}
	if w := do(t, s, "DELETE", "/api/stations/S1/roles/"+roleId, "", adminHdr); w.Body.String() != `{"success":true,"error":null,"id":null}` {
		t.Fatalf("delete role = %s", w.Body.String())
	}
	if w := do(t, s, "DELETE", "/api/stations/S1/users/U1/roles/"+roleId, "", adminHdr); w.Body.String() != `{"success":false,"error":null,"id":null}` {
		t.Fatalf("role must have been stripped from the user: %s", w.Body.String())
	}

	// Station events.
	w = do(t, s, "POST", "/api/stations/S1/events", `{"title":"Party","duration":60,"public":true}`, adminHdr)
	evId := stj.MustParse(w.Body.Bytes()).GetProperty("id").Str()
	w = do(t, s, "GET", "/api/stations/S1/events", "", adminHdr)
	ev := stj.MustParse(w.Body.Bytes()).EnumerateArray()
	if len(ev) != 1 || ev[0].GetProperty("deployment_id").ValueKind() != stj.Null || ev[0].GetProperty("description").Str() != "" {
		t.Fatalf("events = %s", w.Body.String())
	}
	if w := do(t, s, "DELETE", "/api/stations/S1/events/"+evId, "", adminHdr); !strings.Contains(w.Body.String(), `"success":true`) {
		t.Fatalf("delete event = %s", w.Body.String())
	}
}

func TestDashboardDraftersRequireAdminKey(t *testing.T) {
	off := "0"
	resetDashAuth(t, &off) // the preprocessor lets it through; the handler still demands the key
	withTempDB(t)
	s := newAresDashboardServer()
	w := do(t, s, "GET", "/api/drafters/global", "", nil)
	if w.Code != 403 || w.Body.String() != `{"error":"Dashboard administrator required"}` || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("no key = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/drafters/bad!scope", "", adminHdr); w.Code != 404 || w.Body.String() != `{"error":"Unknown scope"}` {
		t.Fatalf("bad scope = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "POST", "/api/drafters/global/nobody", "", adminHdr); w.Code != 400 || w.Body.String() != `{"error":"Select a known user"}` {
		t.Fatalf("unknown user = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/drafters/global", "", adminHdr); w.Code != 200 || w.Body.String() != `{"scope":"global","role":"Drafter","accounts":[],"grants":[]}` {
		t.Fatalf("list = %d %s", w.Code, w.Body.String())
	}
}

func TestDashboardMiscEndpoints(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()
	if w := do(t, s, "GET", "/api/ws-clients/none/messages", "", adminHdr); w.Code != 404 || w.Body.Len() != 0 {
		t.Fatalf("ws messages = %d", w.Code)
	}
	if w := do(t, s, "GET", "/api/telemetry?limit=abc", "", adminHdr); w.Code != 200 || w.Body.String() != "[]" {
		t.Fatalf("telemetry = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/spec/levels/bad%2Fname", "", adminHdr); w.Code != 400 || w.Body.String() != `{"error":"bad level name"}` {
		t.Fatalf("spec level = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "PATCH", "/api/spec/levels/Lvl", `{}`, adminHdr); w.Code != 400 || w.Body.String() != `{"error":"body must be { autoload?: bool, loaded?: bool }"}` {
		t.Fatalf("spec patch = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "PUT", "/api/stations/x/whitelist", `[]`, adminHdr); w.Code != 400 || w.Body.String() != `{"error":"body must be {`+"\\"+"u0022usernames"+"\\"+"u0022"+`: [..]}"}` {
		t.Fatalf("whitelist bad body = %d %s", w.Code, w.Body.String())
	}
	if w := do(t, s, "GET", "/api/agents", "", adminHdr); w.Body.String() != `{"agents":[]}` {
		t.Fatalf("agents = %s", w.Body.String())
	}
	if w := do(t, s, "POST", "/api/sessions", `null`, adminHdr); w.Body.String() != `{"success":false,"error":"invalid body","id":null}` {
		t.Fatalf("sessions null = %s", w.Body.String())
	}
	if w := do(t, s, "GET", "/api/netvars/dump?deployment_id=zz", "", adminHdr); w.Code != 404 ||
		w.Body.String() != `{"error":"no dump received yet","deployment_id":"zz","type":"netvars"}` {
		t.Fatalf("netvar dump = %d %s", w.Code, w.Body.String())
	}
}

func TestDashboardGenerateApiKeyShape(t *testing.T) {
	k := dashboardGenerateApiKey("owner", "service")
	parts := strings.Split(k, ".")
	if len(parts) != 3 {
		t.Fatal(k)
	}
	p, err := dashboardB64UrlDecode(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	el := stj.MustParse(p)
	keys := []string{}
	for _, x := range el.EnumerateObject() {
		keys = append(keys, x.Name)
	}
	if strings.Join(keys, ",") != "key_id,key_type,owner_id,created_at" || len(el.GetProperty("created_at").Str()) != len("2006-01-02 15:04:05.000000") {
		t.Fatalf("payload = %s", p)
	}
}

// Solara: a station's dashboard "last_online" follows its deployments' heartbeat stamp (LastEvent),
// not only the stored station row (which moves on register / posted events only).
func TestDashboardStationLastOnlineFollowsHeartbeat(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	s := newAresDashboardServer()

	old := dotnet.Now().AddMinutes(-51)
	beat := dotnet.Now()
	stations := docdb.GetCollection(Database, true, models.NewStationDbObject)
	st := models.NewStationDbObject()
	st.StationId, st.StationName, st.LastOnline = dashStrPtr("st1"), dashStrPtr("Halcyon"), &old
	stations.Insert(st)
	deps := docdb.GetCollection(Database, true, models.NewDeploymentDbObject)
	d := models.NewDeploymentDbObject()
	d.DeploymentId, d.StationId, d.Online, d.LastEvent = dashStrPtr("dep1"), dashStrPtr("st1"), true, &beat
	deps.Insert(d)

	w := do(t, s, "GET", "/api/stations", "", adminHdr)
	arr := stj.MustParse(w.Body.Bytes()).EnumerateArray()
	if len(arr) != 1 {
		t.Fatalf("stations = %s", w.Body.String())
	}
	// The document store keeps millisecond precision, so compare to the second ("2006-01-02T15:04:05).
	want := stj.MarshalString(beat)[:20]
	if got := arr[0].GetProperty("last_online").GetRawText(); !strings.HasPrefix(got, want) {
		t.Fatalf("last_online = %s, want the heartbeat stamp %s", got, want)
	}
	// The stored station row is untouched (display-only change).
	if stored := stations.FindOne(func(x *models.StationDbObject) bool { return dashLiteEq(x.StationId, "st1") }); stored.LastOnline.Sub(old).Abs() > time.Millisecond {
		t.Fatalf("station row LastOnline was modified")
	}
}
