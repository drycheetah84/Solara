// Ground truth for the LiteDB string semantics of the Ares models: the same
// instances as tools/oracle-ares-db (which ran the original assemblies against
// LiteDB 5.0.21) are inserted through docdb, read back with FindAll() and
// serialized; the JSON must equal testdata/dbroundtrip.tsv byte for byte.
// Stored strings are trimmed, "" / whitespace-only become null (also inside
// lists and dictionary values), and a null read back overrides initializers.
package ares

import (
	"path/filepath"
	"strings"
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/stj"
)

func dbrtRoundTrip[T any](t *testing.T, db *docdb.Database, newT func() *T, v *T) string {
	t.Helper()
	c := docdb.GetCollection[T](db, true, newT)
	c.DeleteAll()
	c.Insert(v)
	all := c.FindAll()
	if len(all) != 1 {
		t.Fatalf("FindAll returned %d documents", len(all))
	}
	return stj.MarshalString(all[0])
}

func TestAresModelsLiteDbRoundTripMatchesOracle(t *testing.T) {
	want := map[string]string{}
	for _, l := range a2Lines(t, "testdata/dbroundtrip.tsv") {
		k, v, ok := strings.Cut(l, "\t")
		if ok {
			want[k] = v
		}
	}
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 1, Name: "rt", Path: filepath.Join(t.TempDir(), "rt"+docdb.FileSuffix)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	d := dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234567)
	special := "v<&'" + string(rune(0xE9)) + ">"
	p := a2Str
	cfg := func() map[string]*string {
		return map[string]*string{"a": p(" 2 "), "m": p("m"), "n": nil, "s": p(special), "w": p("  "), "z": p("")}
	}
	lst := func() []*string { return []*string{p(""), p("  x  "), p("y"), p("   "), nil, p(special)} }

	got := map[string]string{}
	rt := func(name string, s string) { got[name] = s }

	rt("default.DeploymentDbObject", dbrtRoundTrip(t, db, models.NewDeploymentDbObject, models.NewDeploymentDbObject()))
	dep := models.NewDeploymentDbObject()
	dep.DeploymentId, dep.StationId, dep.DeploymentName, dep.IpAddress, dep.Region = p(""), p("  st  "), nil, p("1.2.3.4:7777"), p("   ")
	dep.Version, dep.CreatedAt, dep.Online, dep.LastEvent, dep.PlayerCount = p(special), d, true, &d, 7
	dep.Config, dep.ApiKeys = cfg(), []string{"k1"}
	rt("mixed.DeploymentDbObject", dbrtRoundTrip(t, db, models.NewDeploymentDbObject, dep))

	rt("default.ServerEventDbObject", dbrtRoundTrip(t, db, models.NewServerEventDbObject, models.NewServerEventDbObject()))
	sev := models.NewServerEventDbObject()
	sev.DeploymentId, sev.EventType, sev.EventData, sev.Timestamp = p("  d  "), p(""), p(`{"x":1}`), d
	rt("mixed.ServerEventDbObject", dbrtRoundTrip(t, db, models.NewServerEventDbObject, sev))

	rt("default.StationEventDbObject", dbrtRoundTrip(t, db, models.NewStationEventDbObject, models.NewStationEventDbObject()))
	ev := models.NewStationEventDbObject()
	ev.EventId, ev.StationId, ev.DeploymentId, ev.Title, ev.Description = p("e"), p(""), p("  "), p(" T "), p("")
	ev.StartTime, ev.Duration, ev.Public, ev.SignupsOpen, ev.Config = d, 7, true, true, cfg()
	rt("mixed.StationEventDbObject", dbrtRoundTrip(t, db, models.NewStationEventDbObject, ev))

	rt("default.EventSignupDbObject", dbrtRoundTrip(t, db, models.NewEventSignupDbObject, models.NewEventSignupDbObject()))
	su := models.NewEventSignupDbObject()
	su.SignupId, su.EventId, su.UserId, su.Timestamp, su.Data = p(""), p(" e "), p("u"), d, p("")
	rt("mixed.EventSignupDbObject", dbrtRoundTrip(t, db, models.NewEventSignupDbObject, su))

	rt("default.RoleResponse", dbrtRoundTrip(t, db, models.NewRoleResponse, models.NewRoleResponse()))
	role := models.NewRoleResponse()
	role.RoleId, role.StationId, role.RoleName, role.RoleDescription, role.Permissions = p("r"), p(" s "), p("Admin"), p(""), lst()
	rt("mixed.RoleResponse", dbrtRoundTrip(t, db, models.NewRoleResponse, role))

	rt("default.StationDbObject", dbrtRoundTrip(t, db, models.NewStationDbObject, models.NewStationDbObject()))
	st := models.NewStationDbObject()
	st.StationId, st.StationName, st.CreatedAt, st.Online, st.LastOnline, st.Config = p("s"), p(""), d, true, &d, cfg()
	rt("mixed.StationDbObject", dbrtRoundTrip(t, db, models.NewStationDbObject, st))

	rt("default.UserDataResponse", dbrtRoundTrip(t, db, models.NewUserDataResponse, models.NewUserDataResponse()))
	u := models.NewUserDataResponse()
	u.UserId, u.Username, u.DiscordId, u.Platform, u.CreatedAt, u.LastLogin = p(" u "), p(""), nil, p("quest"), d, d
	u.Roles = lst()
	u.Bans = []*models.BanRequest{{UserId: p(""), StationId: p(" s "), Expiration: d, Reason: p(""), Revoked: true}, models.NewBanRequest()}
	rt("mixed.UserDataResponse", dbrtRoundTrip(t, db, models.NewUserDataResponse, u))

	rt("default.UserApiKeyDbObject", dbrtRoundTrip(t, db, models.NewUserApiKeyDbObject, models.NewUserApiKeyDbObject()))
	k := models.NewUserApiKeyDbObject()
	k.UserId, k.ApiKey = p(""), p(" k ")
	rt("mixed.UserApiKeyDbObject", dbrtRoundTrip(t, db, models.NewUserApiKeyDbObject, k))

	g := NewDrafterGrant()
	g.UserId, g.Scope, g.GrantedAt = "user_1", "global", d
	rt("valid.DrafterGrant", dbrtRoundTrip(t, db, NewDrafterGrant, g))

	if len(want) != len(got) {
		t.Errorf("fixture has %d cases, test built %d", len(want), len(got))
	}
	for name, w := range want {
		if g, ok := got[name]; !ok {
			t.Errorf("%s: case not built", name)
		} else if g != w {
			t.Errorf("%s:\n got  %s\n want %s", name, g, w)
		}
	}
}

// Differential scenario: a station config value set to "" (or whitespace)
// is stored by LiteDB as null, so the original served it as null to the
// game-server DLL (GET /v1/deployments/{id}?include_station_config=true) and
// to the dashboard; the key still shadows the board default (TryAdd).
func TestStationConfigEmptyValueServedAsNull(t *testing.T) {
	s, _ := a2TestEnv(t)
	t.Setenv("BOARD_PUBLIC_BASE", "")
	resetDashAuth(t, nil)
	ds := newAresDashboardServer()

	w := a2Do(t, s, "POST", "/register_server", `{"deployment_id":"dep-9"}`, nil)
	sid := a2Json(t, w).GetProperty("station_id").Str()

	// Dashboard PATCH: the response is the in-memory dictionary (still "").
	w = do(t, ds, "PATCH", "/api/stations/"+sid+"/config", `{"Custom":"","BoardTextureUrl1":"  ","Keep":" v "}`, adminHdr)
	if w.Code != 200 {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}
	// (Key order is not asserted: Go maps serialize sorted, see the Ares
	// port notes on dictionary key order.)
	dbrtExpectProps(t, "patch response", w.Body.Bytes(), map[string]string{"Custom": `""`, "BoardTextureUrl1": `"  "`, "Keep": `" v "`})
	// Station API merge (A2 server), same semantics.
	w = a2Do(t, s, "POST", "/stations/"+sid+"/config", `{"Other":""}`, map[string]string{"x-api-key": a2TestMasterKey})
	if got := a2Json(t, w).GetProperty("Other").GetRawText(); got != `""` {
		t.Errorf("A2 set response Other = %s", got)
	}

	w = a2Do(t, s, "GET", "/v1/deployments/dep-9?include_station_config=true", "", map[string]string{"Host": "127.0.0.1:78"})
	dbrtExpectProps(t, "deployment fetch config", []byte(a2Json(t, w).GetProperty("config").GetRawText()), map[string]string{
		"Custom": "null", "BoardTextureUrl1": "null", "Keep": `"v"`, "Other": "null",
		"BoardTextureUrl2": `"http://209.126.2.136:78/board/_default.png"`,
	})

	w = do(t, ds, "GET", "/api/stations/"+sid+"/config", "", adminHdr)
	dbrtExpectProps(t, "dashboard station config", w.Body.Bytes(), map[string]string{
		"BoardTextureUrl1": "null", "Custom": "null", "Keep": `"v"`, "Other": "null",
	})
	w = do(t, ds, "GET", "/api/stations/"+sid+"/board", "", adminHdr)
	board := stj.MustParse(w.Body.Bytes())
	if v := board.GetProperty("BoardTextureUrl1").GetRawText(); v != "null" {
		t.Errorf("board BoardTextureUrl1 = %s", v)
	}
	if v := board.GetProperty("BoardTextureUrl2").GetRawText(); v != `""` {
		t.Errorf("board BoardTextureUrl2 = %s", v)
	}
}

func dbrtExpectProps(t *testing.T, what string, body []byte, want map[string]string) {
	t.Helper()
	el := stj.MustParse(body)
	for k, w := range want {
		v, ok := el.TryGetProperty(k)
		if !ok || v.GetRawText() != w {
			t.Errorf("%s[%s] = %v %s, want %s (body %s)", what, k, ok, v.GetRawText(), w, body)
		}
	}
}
