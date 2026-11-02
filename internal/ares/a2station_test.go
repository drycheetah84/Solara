// Tests for the A2StationDbServer port and the static stores
// (NetvarDumpStore, NetvarOverridesFile, SpecLevelsStore, StationAcl,
// DrafterPermissions). Oracle fixtures in testdata/ come from
// tools/oracle-ares run against the original built assemblies.
package ares

import (
	"bufio"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/eos"
	"solara/internal/secrets"
	"solara/internal/stj"
)

const a2TestMasterKey = "test-master-key"

func init() {
	secrets.Set("ServerMasterKey", a2TestMasterKey)
	secrets.Set("QUEST_APP_SECRET", "test-quest-secret")
	secrets.Set("SERVER_API_KEY", "test-server-api-key")
}

// a2TestEnv points Database at a fresh store and the working directory at a
// temp dir, and empties the in-memory EOS sessions.
func a2TestEnv(t *testing.T) (*A2StationDbServer, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 1, Name: "a2", Path: filepath.Join(dir, "a2"+docdb.FileSuffix)})
	if err != nil {
		t.Fatal(err)
	}
	old := Database
	Database = db
	wd, _ := os.Getwd()
	_ = os.Chdir(dir)
	eos.ClearSessions()
	a2NvInitMu.Lock()
	a2NvInit = false
	a2NvInitMu.Unlock()
	t.Cleanup(func() {
		eos.ClearSessions()
		Database = old
		_ = db.Close()
		_ = os.Chdir(wd)
	})
	return newA2StationDbServer(), dir
}

func a2Do(t *testing.T, s *A2StationDbServer, method, target, body string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	for k, v := range hdr {
		if k == "Host" {
			r.Host = v
			continue
		}
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.HTTP.ServeHTTP(w, r)
	return w
}

func a2Json(t *testing.T, w *httptest.ResponseRecorder) stj.Element {
	t.Helper()
	el, err := stj.Parse(w.Body.Bytes())
	if err != nil {
		t.Fatalf("bad json %q: %v", w.Body.String(), err)
	}
	return el
}

func a2Lines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		out = append(out, strings.TrimRight(sc.Text(), "\r"))
	}
	return out
}

// ─── Route table ────────────────────────────────────────────────────────────

func TestA2StationDbServerRouteTable(t *testing.T) {
	var want []string
	for _, l := range a2Lines(t, "../../docs/original_routes.tsv") {
		if strings.HasPrefix(l, "A2StationDbServer\t") {
			want = append(want, strings.TrimPrefix(l, "A2StationDbServer\t"))
		}
	}
	s := newA2StationDbServer()
	var got []string
	for _, e := range s.HTTP.Endpoints() {
		got = append(got, e.Method+"\t"+e.RouteUri+"\t"+e.Name)
	}
	// The 86 original routes come first and in order; Solara additions follow.
	extra := []string{"POST\t/v1/deployments/{deployment_id}/gaia/events\tSolara/PostGaiaEvents", "GET\t/v1/deployments/{deployment_id}/gaia/next\tSolara/GetGaiaNext",
		"POST\t/v1/fleets/{fleet_id}/challenges/{challenge_id}/add\tSolara/AddChallengeRecord",
		"POST\t/v1/fleets/{fleet_id}/challenges/{challenge_id}/list\tSolara/ListChallengeRecords"}
	if len(got) != len(want)+len(extra) || len(want) != 86 || strings.Join(got[len(want):], "\n") != strings.Join(extra, "\n") {
		t.Fatalf("endpoint count: got %d want %d (86) + %v: %v", len(got), len(want), extra, got[len(want):])
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("route %d: got %q want %q", i, got[i], want[i])
		}
	}
	if s.HTTP.Port != 78 || s.HTTP.Hostname != "localhost" {
		t.Errorf("bind: %s:%d", s.HTTP.Hostname, s.HTTP.Port)
	}
}

// ─── Oracle-checked helpers ────────────────────────────────────────────────

func TestA2HelpersMatchOracle(t *testing.T) {
	n := 0
	for _, l := range a2Lines(t, "testdata/helpers.tsv") {
		f := strings.Split(l, "\t")
		if len(f) != 3 {
			continue // the "\t7\n" case splits across lines in the fixture
		}
		kind, in, want := f[0], f[1], f[2]
		n++
		switch kind {
		case "typed":
			if !strings.HasPrefix(want, "{") && !strings.HasPrefix(want, "EXC") {
				continue // the "\t7\n" input spans two fixture lines
			}
			res := a2ToTypedConfig(map[string]*string{"k": a2Str(in)})
			got := func() (s string) {
				defer func() {
					if p := recover(); p != nil {
						v := res["k"].(float64)
						name := "NaN"
						if math.IsInf(v, 1) {
							name = "∞"
						} else if math.IsInf(v, -1) {
							name = "-∞"
						}
						s = "EXC Double " + name + " ArgumentException"
					}
				}()
				return stj.MarshalString(res)
			}()
			if got != want {
				t.Errorf("ToTypedConfig(%q): got %s want %s", in, got, want)
			}
		case "board":
			if got := PublicBoardBase(in); got != want {
				t.Errorf("PublicBoardBase(%q): got %s want %s", in, got, want)
			}
		case "sanitise":
			if got := a2SanitiseForFileName(in); got != want {
				t.Errorf("SanitiseForFileName(%q): got %s want %s", in, got, want)
			}
		case "cleanname":
			got := "<null>"
			if p := SpecLevelsStoreCleanName(&in); p != nil {
				got = *p
			}
			if got != want {
				t.Errorf("CleanName(%q): got %s want %s", in, got, want)
			}
		case "aclparse":
			var raw string
			stj.MustUnmarshal([]byte(in), &raw)
			if got := stj.MarshalString(Parse(&raw)); got != want {
				t.Errorf("StationAcl.Parse(%q): got %s want %s", raw, got, want)
			}
		case "validid":
			if got := dotnet.FormatBool(DrafterPermissionsValidId(in)); got != want {
				t.Errorf("ValidId(%q): got %s want %s", in, got, want)
			}
		default:
			n--
		}
	}
	// buildtext lines carry JSON-encoded strings
	for _, l := range a2Lines(t, "testdata/helpers.tsv") {
		k, v, _ := strings.Cut(l, "\t")
		var want string
		switch k {
		case "buildtext":
			stj.MustUnmarshal([]byte(v), &want)
			st := models.NewStationDbObject()
			st.Config = map[string]*string{"nv.module.3.Speed": a2Str("\"12\""), "nv.world.config/player/brakeStrength": a2Str(" 0.5\t"),
				"nv.module.bad": a2Str("x"), "nv.module.*.Mode": a2Str("DRIFT"), "other": a2Str("y"), "nv.zzz": a2Str("q"), "nv.module.x.": a2Str("z")}
			if got := BuildText(st); got != want {
				t.Errorf("BuildText: got %q want %q", got, want)
			}
			n++
		case "buildtext.null":
			stj.MustUnmarshal([]byte(v), &want)
			if got := BuildText(nil); got != want {
				t.Errorf("BuildText(null): got %q want %q", got, want)
			}
			n++
		}
	}
	if n < 60 {
		t.Fatalf("only %d oracle cases checked", n)
	}
}

func TestA2FuzzyMatchesOracle(t *testing.T) {
	in := a2Lines(t, "testdata/fuzzy_in.tsv")
	out := a2Lines(t, "testdata/fuzzy_out.tsv")
	if len(in) != len(out) || len(in) < 20 {
		t.Fatalf("fixture sizes %d/%d", len(in), len(out))
	}
	for i := range in {
		q, cs, _ := strings.Cut(in[i], "\t")
		var choices []*string
		for _, c := range strings.Split(cs, "|") {
			c := c
			choices = append(choices, &c)
		}
		var parts []string
		for _, r := range a2ExtractTop(q, choices) {
			parts = append(parts, *r.Value+":"+a2Itoa(r.Score))
		}
		if got := strings.Join(parts, "|"); got != out[i] {
			t.Errorf("ExtractTop(%q):\n got  %s\n want %s", q, got, out[i])
		}
	}
}

func TestA2StoreModelsMatchOracle(t *testing.T) {
	fx := map[string]string{}
	for _, l := range a2Lines(t, "models/testdata/models.tsv") {
		k, v, _ := strings.Cut(l, "\t")
		fx[k] = v
	}
	d1 := dotnet.New(2024, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234567)
	d2 := dotnet.New(2023, 12, 31, 23, 59, 59, dotnet.Unspecified)
	check := func(k string, v any) {
		if got := stj.MarshalString(v); got != fx[k] {
			t.Errorf("%s:\n got  %s\n want %s", k, got, fx[k])
		}
	}
	check("DrafterGrant", &DrafterGrant{UserId: "u", Scope: "global", GrantedAt: d1})
	g := NewDrafterGrant()
	g.GrantedAt = d2
	check("DrafterGrant.empty", g)
	check("SpecEntry", &SpecLevelsStoreEntry{Autoload: true, Loaded: true, Updated: "x", Size: 3, ServerLoaded: []string{"d"}, ServerSeen: "y"})
	check("SpecEntry.empty", NewSpecLevelsStoreEntry())
}

func TestA2SpecIndexIndentedMatchesOracle(t *testing.T) {
	lines := a2Lines(t, "testdata/specindex.txt")
	var want, wantEmpty string
	stj.MustUnmarshal([]byte(lines[0]), &want)
	stj.MustUnmarshal([]byte(lines[1]), &wantEmpty)
	o := stj.Obj{}
	o.Set("Zeta level", &SpecLevelsStoreEntry{Autoload: true, Updated: "2024-01-02T03:04:05.1234567Z", Size: 12, ServerLoaded: []string{"dep-1", "dep<2>"}})
	o.Set("alpha", NewSpecLevelsStoreEntry())
	nl := "\r\n" // fixture captured on Windows (Environment.NewLine)
	if got := string(a2IndentJSON(stj.MustMarshal(o), nl)); got != want {
		t.Errorf("indent:\n got  %q\n want %q", got, want)
	}
	if got := string(a2IndentJSON(stj.MustMarshal(stj.Obj{}), nl)); got != wantEmpty {
		t.Errorf("indent empty: got %q want %q", got, wantEmpty)
	}
}

func TestA2ParseHelpers(t *testing.T) {
	for in, want := range map[string]int{"5": 5, " -7 ": -7, "+3": 3, "2147483647": 2147483647, "-2147483648": -2147483648} {
		if got := a2ParseInt(in); got != want {
			t.Errorf("ParseInt(%q)=%d", in, got)
		}
	}
	for in, typ := range map[string]string{"": "System.FormatException", "1.5": "System.FormatException", "2147483648": "System.OverflowException", "abc": "System.FormatException"} {
		func() {
			defer func() {
				if e, ok := recover().(*dotnet.Exception); !ok || e.Type != typ {
					t.Errorf("ParseInt(%q): want %s", in, typ)
				}
			}()
			a2ParseInt(in)
		}()
	}
	if !a2ParseBool(" TRUE ") || a2ParseBool("false") {
		t.Error("ParseBool")
	}
	func() {
		defer func() {
			if e, ok := recover().(*dotnet.Exception); !ok || e.Message != "String 'yes' was not recognized as a valid Boolean." {
				t.Errorf("ParseBool(yes): %v", e)
			}
		}()
		a2ParseBool("yes")
	}()
	if d := a2ParseDateTime("2024-05-06T07:08:09"); d.Kind() != dotnet.Unspecified || d.FormatO() != "2024-05-06T07:08:09.0000000" {
		t.Errorf("ParseDateTime: %s", d.FormatO())
	}
	if d := a2ParseDateTime("2024-05-06T07:08:09Z"); d.Kind() != dotnet.Local {
		t.Error("ParseDateTime Z must yield Local")
	}
	if d := a2ParseDateTime("05/06/2024 07:08:09"); d.Month() != 5 || d.Day() != 6 {
		t.Error("invariant MM/dd/yyyy")
	}
}

// ─── Handlers ──────────────────────────────────────────────────────────────

func TestA2RegisterServerAndPlayerCount(t *testing.T) {
	s, _ := a2TestEnv(t)

	// invalid body -> success:false
	w := a2Do(t, s, "POST", "/register_server", "not json", nil)
	if w.Code != 200 || w.Body.String() != `{"success":false,"station_id":null,"deployment_id":null}` {
		t.Fatalf("bad body: %d %s", w.Code, w.Body.String())
	}

	w = a2Do(t, s, "POST", "/register_server", `{"ip":"1.2.3.4","port":"7777","deployment_id":"dep-1","server_name":"My Server","max_players":12}`, nil)
	el := a2Json(t, w)
	if !el.GetProperty("success").GetBoolean() || el.GetProperty("deployment_id").Str() != "dep-1" {
		t.Fatalf("register: %s", w.Body.String())
	}
	stationId := el.GetProperty("station_id").Str()
	if len(stationId) != 16 {
		t.Errorf("station id %q", stationId)
	}
	deps := a2Deployments().FindAll()
	if len(deps) != 1 || *deps[0].IpAddress != "1.2.3.4:7777" || *deps[0].Version != "22284" || *deps[0].Region != "NAE" || !deps[0].Online {
		t.Fatalf("deployment row: %+v", deps)
	}
	if roles := a2Roles().FindAll(); len(roles) != 3 || *roles[0].RoleName != "Admin" {
		t.Fatalf("default roles: %d", len(roles))
	}
	sess := eos.SessionsSnapshot()
	if len(sess) != 1 || sess[0].MaxPlayers != 12 || sess[0].Bucket != "22284" || sess[0].BuildId != 22284 ||
		*sess[0].StationId != stationId || *sess[0].DeploymentId != "dep-1" || len(sess[0].Id) != 36 {
		t.Fatalf("eos session: %+v", sess)
	}

	// re-register under the same id: station reused, one session; the row
	// keeps its old name (preserved bug: Update with a fresh ObjectId).
	w = a2Do(t, s, "POST", "/register_server", `{"ip":"5.6.7.8","deployment_id":"dep-1","server_name":"Renamed"}`, nil)
	if a2Json(t, w).GetProperty("station_id").Str() != stationId {
		t.Fatal("station not reused")
	}
	deps = a2Deployments().FindAll()
	if len(deps) != 1 || *deps[0].IpAddress != "1.2.3.4:7777" || *deps[0].DeploymentName != "My Server" {
		t.Fatalf("re-register must not update the row: %+v", deps[0])
	}
	if sess = eos.SessionsSnapshot(); len(sess) != 1 || sess[0].ServerName != "My Server" || sess[0].IpAddress != "5.6.7.8" || sess[0].Port != "7777" {
		t.Fatalf("eos session after re-register: %+v", sess)
	}
	if len(a2Stations().FindAll()) != 1 {
		t.Fatal("station duplicated")
	}

	// update_player_count
	w = a2Do(t, s, "POST", "/update_player_count", `{"player_count":3}`, nil)
	if w.Body.String() != `{"success":false,"error":"deployment_id required"}` {
		t.Fatalf("upc missing id: %s", w.Body.String())
	}
	w = a2Do(t, s, "POST", "/update_player_count", `{"deployment_id":"dep-1","player_count":-4}`, nil)
	if w.Body.String() != `{"success":true,"deployment_id":"dep-1","player_count":0,"session_matched":true}` {
		t.Fatalf("upc: %s", w.Body.String())
	}
	if d := a2Deployments().FindAll()[0]; d.LastEvent == nil || d.PlayerCount != 0 {
		t.Fatal("LastEvent not stamped")
	}

	// heartbeat (no HalcyonSocket instance in tests)
	w = a2Do(t, s, "POST", "/server_heartbeat", `{"deployment_id":"dep-1","pid":1,"seq":2}`, nil)
	if w.Body.String() != `{"success":true}` {
		t.Fatalf("heartbeat: %s", w.Body.String())
	}

	// player count totals (also via the station route)
	w = a2Do(t, s, "GET", "/stations/player_count", "", nil)
	if w.Body.String() != "0" || w.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("player_count: %q %q", w.Body.String(), w.Header().Get("Content-Type"))
	}
}

func TestA2DeploymentFetchWithStationConfig(t *testing.T) {
	s, dir := a2TestEnv(t)
	t.Setenv("BOARD_PUBLIC_BASE", "")
	w := a2Do(t, s, "POST", "/register_server", `{"deployment_id":"dep-9"}`, nil)
	stationId := a2Json(t, w).GetProperty("station_id").Str()

	st := a2FindStation(stationId)
	st.Config["BoardTextureUrl1"] = a2Str("https://cdn.example/x.png")
	st.Config["Custom"] = a2Str("v")
	st.Config["nv.world.config/x"] = a2Str("1")
	a2Stations().Update(st)

	w = a2Do(t, s, "GET", "/v1/deployments/dep-9?include_station_config=true", "", map[string]string{"Host": "127.0.0.1:78"})
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	el := a2Json(t, w)
	cfg := el.GetProperty("config")
	if cfg.GetProperty("BoardTextureUrl1").Str() != "https://cdn.example/x.png" {
		t.Error("station config must win over board defaults")
	}
	if cfg.GetProperty("BoardTextureUrl2").Str() != "http://209.126.2.136:78/board/_default.png" {
		t.Errorf("board default (loopback host -> public fallback): %s", cfg.GetProperty("BoardTextureUrl2").Str())
	}
	if cfg.GetProperty("StationAnnouncement").Str() != "Welcome to Halcyon Ring" || cfg.GetProperty("Custom").Str() != "v" {
		t.Error("station config / defaults missing")
	}
	if _, ok := cfg.TryGetProperty("nv.world.config/x"); ok {
		t.Error("nv. overrides must not be served in the config fetch")
	}
	if el.GetProperty("deployment_id").Str() != "dep-9" || el.GetProperty("version").Str() != "22284" {
		t.Errorf("deployment fields: %s", w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "boards", "_default.png")); err != nil {
		t.Error("default board image not written")
	}

	// unknown fixed id -> synthetic deployment bound to the latest online station
	w = a2Do(t, s, "GET", "/v1/deployments/halcyon", "", nil)
	el = a2Json(t, w)
	if el.GetProperty("station_id").Str() != stationId || el.GetProperty("deployment_id").Str() != "halcyon" {
		t.Errorf("synthetic deployment: %s", w.Body.String())
	}

	// include_deployment_config=false still serves station config
	w = a2Do(t, s, "GET", "/v1/deployments/dep-9?include_deployment_config=false&include_station_config=false", "", nil)
	if a2Json(t, w).GetProperty("config").GetRawText() != "{}" {
		t.Errorf("config should be empty: %s", w.Body.String())
	}

	// netvar overrides text
	w = a2Do(t, s, "GET", "/v1/deployments/dep-9/netvar_overrides", "", nil)
	// Solara: the station's board URL rides along as config/stationConfig/<key> (stationConfigWorldLines).
	if !strings.HasSuffix(w.Body.String(), "\nworld\t\tconfig/x\t1\nworld\t\tconfig/stationConfig/BoardTextureUrl1\thttps://cdn.example/x.png") ||
		w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("netvar overrides: %q", w.Body.String())
	}
	if b, err := os.ReadFile(filepath.Join(dir, "netvar_overrides", "dep-9.txt")); err != nil || string(b) != w.Body.String() {
		t.Error("override file not written by EnsureAllWritten")
	}

	// bad bool -> FormatException -> 500 page
	w = a2Do(t, s, "GET", "/v1/deployments/dep-9?include_station_config=yes", "", nil)
	if w.Code != 500 || !strings.Contains(w.Body.String(), "GetDeploymentV1") {
		t.Errorf("bad bool: %d", w.Code)
	}
}

func TestA2AuthAndKeys(t *testing.T) {
	s, _ := a2TestEnv(t)

	// purge without / with a wrong master key
	for _, h := range []map[string]string{nil, {"x-api-key": "nope"}} {
		w := a2Do(t, s, "POST", "/v1/server/admin/purge_stations", "", h)
		if w.Code != 200 || w.Body.String() != `{"success":false}` {
			t.Fatalf("purge auth: %d %s", w.Code, w.Body.String())
		}
	}
	w := a2Do(t, s, "POST", "/v1/server/admin/prune_sessions", "", nil)
	if w.Body.String() != `{"success":false}` {
		t.Fatal("prune auth")
	}
	w = a2Do(t, s, "POST", "/v1/board/upload?station_id=x&key=SignArenaPrime", "\x89PNG\r\n\x1a\n", nil)
	if w.Code != 401 || w.Body.String() != `{"success":false}` {
		t.Fatalf("upload auth: %d %s", w.Code, w.Body.String())
	}

	// log_in_with_key: none, unknown, master
	if w = a2Do(t, s, "POST", "/users/log_in_with_key", "", nil); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("no key -> empty 200")
	}
	if w = a2Do(t, s, "POST", "/v1/users/log_in_with_key", "", map[string]string{"x-api-key": "unknown"}); w.Body.Len() != 0 {
		t.Fatal("unknown key -> empty")
	}
	w = a2Do(t, s, "POST", "/v1/users/log_in_with_key", "", map[string]string{"x-api-key": a2TestMasterKey})
	el := a2Json(t, w)
	if el.GetProperty("user").GetRawText() != `{"user_id":"halcyon-server","username":"HalcyonServer","discord_id":null,"platform":null,"created":"0001-01-01T00:00:00","last_login":"0001-01-01T00:00:00"}` ||
		len(el.GetProperty("permissions").EnumerateArray()) != len(a2AllPermissions) {
		t.Fatalf("master login: %s", w.Body.String())
	}

	// player login -> key -> log_in_with_key round trip
	w = a2Do(t, s, "POST", "/users/log_in", `{"platform_id":"p1","userName":"Pat","platform":"quest"}`, nil)
	el = a2Json(t, w)
	key := el.GetProperty("api_key").Str()
	if !el.GetProperty("success").GetBoolean() || el.GetProperty("org_scoped_id").Str() != "p1" || strings.Count(key, ".") != 2 {
		t.Fatalf("log_in: %s", w.Body.String())
	}
	w = a2Do(t, s, "POST", "/users/log_in_with_key", "", map[string]string{"x-api-key": key})
	if a2Json(t, w).GetProperty("user").GetProperty("username").Str() != "Pat" {
		t.Fatalf("log_in_with_key: %s", w.Body.String())
	}
	// v2 login: slim response
	w = a2Do(t, s, "POST", "/v2/users/log_in", `{"platform_id":"p1","userName":"Pat2"}`, nil)
	if el = a2Json(t, w); len(el.EnumerateObject()) != 3 || el.GetProperty("org_scoped_id").Str() != "p1" {
		t.Fatalf("v2 log_in: %s", w.Body.String())
	}
	if w = a2Do(t, s, "POST", "/v2/users/log_in", `{}`, nil); w.Body.String() != `{"success":false,"api_key":"","org_scoped_id":""}` {
		t.Fatalf("v2 empty: %s", w.Body.String())
	}

	// purge with the key wipes everything
	a2Do(t, s, "POST", "/register_server", `{}`, nil)
	w = a2Do(t, s, "POST", "/v1/server/admin/purge_stations", "", map[string]string{"x-api-key": a2TestMasterKey})
	if w.Body.String() != `{"success":true}` || len(a2Stations().FindAll()) != 0 || len(eos.SessionsSnapshot()) != 0 {
		t.Fatal("purge")
	}
}

func TestA2StationsRolesAndUsers(t *testing.T) {
	s, _ := a2TestEnv(t)
	w := a2Do(t, s, "POST", "/stations/create", `{"station_name":"Alpha","station_id":"st1"}`, map[string]string{"x-api-key": a2TestMasterKey})
	if w.Body.String() != `{"station_id":"st1"}` {
		t.Fatalf("create: %s", w.Body.String())
	}
	a2Do(t, s, "POST", "/users/log_in", `{"platform_id":"u1","userName":"Bob"}`, nil)

	// grant by name (in-memory, case-sensitive) then roles
	if w = a2Do(t, s, "POST", "/stations/st1/users/u1/grant/admin", "", map[string]string{"x-api-key": a2TestMasterKey}); w.Body.String() != `{"success":false}` {
		t.Fatal("grant must be case-sensitive")
	}
	if w = a2Do(t, s, "POST", "/stations/st1/users/u1/grant/Admin", "", map[string]string{"x-api-key": a2TestMasterKey}); w.Body.String() != `{"success":true}` {
		t.Fatal("grant")
	}
	w = a2Do(t, s, "GET", "/stations/st1/users/u1/roles", "", nil)
	roles := a2Json(t, w).GetProperty("roles").EnumerateArray()
	if len(roles) != 1 || roles[0].GetProperty("role_name").Str() != "Admin" {
		t.Fatalf("roles: %s", w.Body.String())
	}
	// LiteDB collation: user lookup by route id is case-insensitive
	w = a2Do(t, s, "GET", "/v1/users/U1?include_roles=true&include_bans=true", "", nil)
	el := a2Json(t, w)
	if el.GetProperty("username").Str() != "Bob" || len(el.GetProperty("roles").EnumerateArray()) != 1 || el.GetProperty("ban").GetRawText() != "[]" {
		t.Fatalf("v1 user: %s", w.Body.String())
	}
	// v0 perms
	w = a2Do(t, s, "GET", "/users/u1", "", nil)
	if n := len(a2Json(t, w).EnumerateArray()); n != len(a2AllPermissions) {
		t.Fatalf("perms %d", n)
	}
	// station users v1 with fuzzy search
	w = a2Do(t, s, "GET", "/v1/stations/st1/users?search_string=bob", "", nil)
	el = a2Json(t, w)
	if el.GetProperty("page").GetProperty("total_items").GetInt32() != 1 {
		t.Fatalf("station users: %s", w.Body.String())
	}
	// config merge / delete
	w = a2Do(t, s, "POST", "/stations/st1/config", `{"b":"2","a":"1"}`, map[string]string{"x-api-key": a2TestMasterKey})
	if w.Body.String() != `{"a":"1","b":"2"}` {
		t.Fatalf("config set: %s", w.Body.String())
	}
	w = a2Do(t, s, "DELETE", "/stations/st1/config", `["a"]`, map[string]string{"x-api-key": a2TestMasterKey})
	if w.Body.String() != `{"b":"2"}` {
		t.Fatalf("config delete: %s", w.Body.String())
	}
	// fleets v2 (typed config)
	w = a2Do(t, s, "GET", "/v2/fleets/st1", "", nil)
	if a2Json(t, w).GetProperty("config").GetRawText() != `{"b":2}` {
		t.Fatalf("fleet: %s", w.Body.String())
	}
	// unknown station -> empty 200
	if w = a2Do(t, s, "GET", "/stations/nope", "", nil); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("unknown station")
	}
	// board traversal guard
	if w = a2Do(t, s, "GET", "/board/..%5Cx.png", "", nil); w.Code != 404 {
		t.Fatalf("traversal: %d", w.Code)
	}
	// bans
	a2Do(t, s, "POST", "/v1/stations/st1/users/u1/ban?duration=3600", `{"reason":"spam"}`, map[string]string{"x-api-key": a2TestMasterKey})
	w = a2Do(t, s, "GET", "/v1/stations/st1/bans", "", nil)
	bans := a2Json(t, w).GetProperty("bans").EnumerateArray()
	if len(bans) != 1 || bans[0].GetProperty("reason").Str() != "spam" {
		t.Fatalf("bans: %s", w.Body.String())
	}
	if w = a2Do(t, s, "PATCH", "/v1/stations/st1/users/u1/unban", "", map[string]string{"x-api-key": a2TestMasterKey}); w.Body.String() != `{"success":true}` {
		t.Fatal("unban")
	}
	// paging error: page=0 -> ArgumentException -> 500
	if w = a2Do(t, s, "GET", "/v1/users?page=0", "", nil); w.Code != 500 {
		t.Fatalf("page=0: %d", w.Code)
	}
}

func TestA2SpecLevelsStoreAndRoutes(t *testing.T) {
	s, dir := a2TestEnv(t)
	if w := a2Do(t, s, "PUT", "/v1/spec/levels/My%20Level", "line1\nline2", nil); w.Body.String() != "ok" {
		t.Fatalf("put: %s", w.Body.String())
	}
	if w := a2Do(t, s, "GET", "/v1/spec/levels/my%20level", "", nil); w.Body.String() != "line1\nline2" {
		t.Fatalf("get (case-insensitive file): %q", w.Body.String())
	}
	if w := a2Do(t, s, "GET", "/v1/spec/levels/bad.name", "", nil); w.Code != 400 || w.Body.String() != "bad level name" {
		t.Fatalf("bad name: %d %s", w.Code, w.Body.String())
	}
	if w := a2Do(t, s, "POST", "/v1/spec/levels/My%20Level/loaded?value=maybe", "", nil); w.Code != 400 {
		t.Fatal("bad value")
	}
	w := a2Do(t, s, "GET", "/v1/spec/levels", "", nil)
	f := strings.Split(w.Body.String(), "\t")
	if len(f) != 5 || f[0] != "My Level" || f[1] != "0" || f[2] != "1" || f[4] != "11" {
		t.Fatalf("list: %q", w.Body.String())
	}
	if w = a2Do(t, s, "GET", "/v1/spec/desired", "", nil); w.Body.String() != "My Level" {
		t.Fatalf("desired: %q", w.Body.String())
	}
	if w = a2Do(t, s, "POST", "/v1/spec/status?deployment=d1", "my level\r\n", nil); w.Body.String() != "ok" {
		t.Fatal("status")
	}
	idx, _ := os.ReadFile(filepath.Join(dir, "spec_levels", "_index.json"))
	if !strings.Contains(string(idx), `"serverLoaded": [`) || !strings.Contains(string(idx), `"d1"`) {
		t.Fatalf("index: %s", idx)
	}
	// boot: loaded := autoload (false) and d1 dropped from serverLoaded
	if w = a2Do(t, s, "POST", "/v1/spec/boot?deployment=d1", "", nil); w.Body.String() != "" {
		t.Fatalf("boot: %q", w.Body.String())
	}
	if SpecLevelsStoreList()[0].Entry.Loaded || len(SpecLevelsStoreList()[0].Entry.ServerLoaded) != 0 {
		t.Fatal("boot state")
	}
	if w = a2Do(t, s, "POST", "/v1/spec/status", "", nil); w.Code != 400 || w.Body.String() != "deployment required" {
		t.Fatal("status needs deployment")
	}
	// corrupt index -> rebuilt from the .txt files
	_ = os.WriteFile(filepath.Join(dir, "spec_levels", "_index.json"), []byte("{oops"), 0o644)
	if l := SpecLevelsStoreList(); len(l) != 1 || l[0].Name != "My Level" || l[0].Entry.Size != 11 {
		t.Fatalf("rebuild: %+v", l)
	}
	if !SpecLevelsStoreDelete("MY LEVEL") || len(SpecLevelsStoreList()) != 0 {
		t.Fatal("delete")
	}
}

func TestA2NetvarDumpStoreAndAcl(t *testing.T) {
	_, dir := a2TestEnv(t)
	data := `{"x":1}`
	Record("dep.1", a2Str("netvars"), &data)
	Record("dep.1", a2Str("other"), &data) // not a dump type: ignored
	if d := Get(a2Str("dep.1"), "netvars"); d == nil || d.Data != data {
		t.Fatal("live dump")
	}
	if _, err := os.Stat(filepath.Join(dir, "netvar_dumps", "dep_1.netvars.json")); err != nil {
		t.Fatal("dump not persisted under the sanitised name")
	}
	if d := Get(nil, "netvars"); d == nil || d.DeploymentId != "dep.1" {
		t.Fatal("newest dump")
	}
	if Get(a2Str("dep.1"), "bogus") != nil {
		t.Fatal("non-dump type")
	}
	rows := NetvarDumpStoreList()
	if len(rows) < 1 {
		t.Fatal("list")
	}

	// StationAcl pushes into the EOS gateway; DrafterPermissions on the DB.
	st := models.NewStationDbObject()
	st.StationId = a2Str("st-acl")
	st.Config[WhitelistKey] = a2Str("bob, alice")
	a2Stations().Insert(st)
	Push(a2Str("st-acl"))
	PushAll()
	DrafterPermissionsSet("global", "user_1", true)
	DrafterPermissionsSet("GLOBAL", "USER_1", true) // collation: already present
	if l := DrafterPermissionsList(); len(l) != 1 {
		t.Fatalf("drafters: %d", len(l))
	}
	if got := DrafterPermissionsEffective("st-acl"); len(got) != 1 || got[0] != "user_1" {
		t.Fatalf("effective: %v", got)
	}
	DrafterPermissionsSet("global", "user_1", false)
	if len(DrafterPermissionsList()) != 0 {
		t.Fatal("revoke")
	}
}

func TestA2DrafterFeed(t *testing.T) {
	s, _ := a2TestEnv(t)
	if w := a2Do(t, s, "GET", "/v1/spec/drafters/nope", "", nil); w.Code != 404 || w.Body.String() != "Unknown deployment or station" {
		t.Fatalf("unknown: %d %s", w.Code, w.Body.String())
	}
	w := a2Do(t, s, "POST", "/register_server", `{"deployment_id":"d-1"}`, nil)
	stationId := a2Json(t, w).GetProperty("station_id").Str()
	DrafterPermissionsSet("global", "zed", true)
	DrafterPermissionsSet(stationId, "amy", true)
	w = a2Do(t, s, "GET", "/v1/spec/drafters/d-1", "", nil)
	if w.Body.String() != "halcyon-drafters-v1\nG|zed\nS|amy\n" || w.Header().Get("Content-Type") != "text/plain" {
		t.Fatalf("feed: %q", w.Body.String())
	}
}

func TestA2PublicHostBoardUrl(t *testing.T) {
	s, _ := a2TestEnv(t)
	t.Setenv("BOARD_PUBLIC_BASE", "")
	w := a2Do(t, s, "GET", "/deployments/x/config", "", map[string]string{"Host": "ring.example.org:78"})
	if got := a2Json(t, w).GetProperty("SignArenaPrime").Str(); got != "https://ring.example.org/board/_default.png" {
		t.Fatalf("public host: %s", got)
	}
	t.Setenv("BOARD_PUBLIC_BASE", "https://cdn.example//")
	w = a2Do(t, s, "GET", "/deployments/x/config", "", nil)
	if got := a2Json(t, w).GetProperty("SignArenaPrime").Str(); got != "https://cdn.example/board/_default.png" {
		t.Fatalf("BOARD_PUBLIC_BASE: %s", got)
	}
}
