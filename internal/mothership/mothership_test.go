package mothership

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/mothership/models"
	"solara/internal/secrets"
	"solara/internal/stj"
)

const (
	testServerKey     = "test-server-key"
	testAutomationKey = "test-automation-key"
)

var testBaseDir string

func init() {
	logging.DisableFiles = true
	secrets.Set("JWT_SECRET", "test-jwt-secret")
	secrets.Set("META_APP_SECRET", "test-meta-secret")
	secrets.Set("AUTOMATION_KEY", testAutomationKey)
	secrets.Set("SERVER_API_KEY", "legacy-fallback")
	_ = os.Setenv("HALCYON_SERVER_API_KEY", "  "+testServerKey+"  ")
	_ = os.Unsetenv("MOTHERSHIP_INSECURE")
	_ = os.Unsetenv("MOTHERSHIP_ALLOW_NONSTORE")
	dir, err := os.MkdirTemp("", "mothership-base")
	if err != nil {
		panic(err)
	}
	testBaseDir = dir
	baseDirOverride = dir
}

func newTestServer(t *testing.T) *MothershipServer {
	t.Helper()
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 1, Name: "t", Path: filepath.Join(t.TempDir(), "t"+docdb.FileSuffix)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	Database = db
	return newMothershipServer()
}

func do(t *testing.T, s *MothershipServer, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	s.HTTP.ServeHTTP(rec, req)
	return rec
}

func expect(t *testing.T, rec *httptest.ResponseRecorder, code int, body string) {
	t.Helper()
	if rec.Code != code || rec.Body.String() != body {
		t.Fatalf("got %d %q\nwant %d %q", rec.Code, rec.Body.String(), code, body)
	}
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return m
}

func loadOracle(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "oracle.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// ---- route table -----------------------------------------------------------

func TestRouteTableMatchesOriginal(t *testing.T) {
	s := newTestServer(t)
	f, err := os.Open(filepath.Join("..", "..", "docs", "original_routes.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var want []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		cols := strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t")
		if len(cols) == 4 && cols[0] == "MothershipServer" {
			want = append(want, cols[1]+" "+cols[2]+" "+cols[3])
		}
	}
	var got []string
	for _, e := range s.HTTP.Endpoints() {
		if strings.HasPrefix(e.Name, "Static/") { // static.aa-mothership.com objects (static.go), appended after the original table
			continue
		}
		got = append(got, e.Method+" "+e.RouteUri+" "+e.Name)
	}
	if len(want) != 114 {
		t.Fatalf("expected 114 original routes, found %d", len(want))
	}
	if !reflect.DeepEqual(got, want) {
		for i := range want {
			if i >= len(got) || got[i] != want[i] {
				t.Fatalf("route %d: got %v want %q", i, got[min(i, len(got)-1)], want[i])
			}
		}
		t.Fatalf("got %d routes, want %d", len(got), len(want))
	}
}

// ---- models vs. the .NET oracle --------------------------------------------

var modelCtors = map[string]func() any{
	"MothershipAnalyticsEventDbObject":          func() any { return models.NewMothershipAnalyticsEventDbObject() },
	"MothershipApiKeyDbObject":                  func() any { return models.NewMothershipApiKeyDbObject() },
	"MothershipAssociationDbObject":             func() any { return models.NewMothershipAssociationDbObject() },
	"MothershipBanDbObject":                     func() any { return models.NewMothershipBanDbObject() },
	"MothershipDeploymentDbObject":              func() any { return models.NewMothershipDeploymentDbObject() },
	"MothershipEntitlementDbObject":             func() any { return models.NewMothershipEntitlementDbObject() },
	"MothershipEnvironmentDbObject":             func() any { return models.NewMothershipEnvironmentDbObject() },
	"MothershipFriendEntryDbObject":             func() any { return models.NewMothershipFriendEntryDbObject() },
	"MothershipInventoryItemDbObject":           func() any { return models.NewMothershipInventoryItemDbObject() },
	"MothershipLinkDbObject":                    func() any { return models.NewMothershipLinkDbObject() },
	"MothershipMuteDbObject":                    func() any { return models.NewMothershipMuteDbObject() },
	"MothershipNotificationDbObject":            func() any { return models.NewMothershipNotificationDbObject() },
	"MothershipOfferBindingDbObject":            func() any { return models.NewMothershipOfferBindingDbObject() },
	"MothershipOfferDbObject":                   func() any { return models.NewMothershipOfferDbObject() },
	"MothershipOfferDisplayDbObject":            func() any { return models.NewMothershipOfferDisplayDbObject() },
	"MothershipPlayerDbObject":                  func() any { return models.NewMothershipPlayerDbObject() },
	"MothershipPrivacyStateDbObject":            func() any { return models.NewMothershipPrivacyStateDbObject() },
	"MothershipProgressionTrackBindingDbObject": func() any { return models.NewMothershipProgressionTrackBindingDbObject() },
	"MothershipProgressionTrackDbObject":        func() any { return models.NewMothershipProgressionTrackDbObject() },
	"MothershipProgressionTreeBindingDbObject":  func() any { return models.NewMothershipProgressionTreeBindingDbObject() },
	"MothershipProgressionTreeDbObject":         func() any { return models.NewMothershipProgressionTreeDbObject() },
	"MothershipReportDbObject":                  func() any { return models.NewMothershipReportDbObject() },
	"MothershipSessionDbObject":                 func() any { return models.NewMothershipSessionDbObject() },
	"MothershipSharedGroupDbObject":             func() any { return models.NewMothershipSharedGroupDbObject() },
	"MothershipTitleDataDbObject":               func() any { return models.NewMothershipTitleDataDbObject() },
	"MothershipTitleDbObject":                   func() any { return models.NewMothershipTitleDbObject() },
	"MothershipTransactionCatalogDbObject":      func() any { return models.NewMothershipTransactionCatalogDbObject() },
	"MothershipUserDataDbObject":                func() any { return models.NewMothershipUserDataDbObject() },
	"MothershipUserDataMetadataDbObject":        func() any { return models.NewMothershipUserDataMetadataDbObject() },
	"MothershipV2PlayerDbObject":                func() any { return models.NewMothershipV2PlayerDbObject() },
}

// populate applies the same rules as tools/oracle-mothership Populate().
func populate(v any) {
	special := "v<&'" + string(rune(0xE9)) + "+>"
	rv := reflect.ValueOf(v).Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Anonymous {
			continue
		}
		fv := rv.Field(i)
		switch fv.Interface().(type) {
		case *string:
			fv.Set(reflect.ValueOf(models.P(f.Name + "_" + special)))
		case int:
			fv.SetInt(7)
		case int64:
			fv.SetInt(9007199254740993)
		case bool:
			fv.SetBool(!fv.Bool())
		case dotnet.DateTime:
			fv.Set(reflect.ValueOf(dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234567)))
		case []*string:
			fv.Set(reflect.ValueOf(models.L("a", "b")))
		case stj.Obj:
			fv.Set(reflect.ValueOf(stj.O("z", "1", "a", "2")))
		default:
			panic("unhandled " + f.Name)
		}
	}
}

func TestModelsMatchOracle(t *testing.T) {
	oracle := loadOracle(t)
	n := 0
	for k, want := range oracle {
		name, ok := strings.CutPrefix(k, "model.default.")
		full := false
		if !ok {
			if name, ok = strings.CutPrefix(k, "model.full."); !ok {
				continue
			}
			full = true
		}
		ctor, ok := modelCtors[name]
		if !ok {
			t.Errorf("no Go model for %s", name)
			continue
		}
		v := ctor()
		if full {
			populate(v)
		}
		if got := stj.MarshalString(v); got != want {
			t.Errorf("%s:\n got %s\nwant %s", k, got, want)
		}
		n++
	}
	if n != 2*len(modelCtors) {
		t.Fatalf("compared %d model fixtures, want %d", n, 2*len(modelCtors))
	}
}

// ---- JSON / codec helpers vs. the oracle -----------------------------------

func TestJwtFormatMatchesOracle(t *testing.T) {
	oracle := loadOracle(t)
	tok := generateJwt(stj.O(
		"sub", "player-1", "did", "dev-1", "env", ENVIRONMENT_ID,
		"externalService", nil, "externalServiceId", "ext<1>", "tid", TENANT_ID,
		"tags", nil, "orgScopedExternalServiceId", "ext<1>",
		"nbf", int64(1700000000), "exp", int64(1700003600), "iat", int64(1700000000)))
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[0]+"."+parts[1] != oracle["jwt.headerbody"] {
		t.Fatalf("got %s want %s", tok, oracle["jwt.headerbody"])
	}
	if parts[2] != hmacBase64Url(parts[0]+"."+parts[1]) || strings.ContainsAny(tok, "=+/") {
		t.Fatal("bad signature encoding")
	}
}

func TestHelpersMatchOracle(t *testing.T) {
	oracle := loadOracle(t)
	if got := stableId("user-1:Hat_01"); got != oracle["stableid"] {
		t.Errorf("stableId %s want %s", got, oracle["stableid"])
	}
	if got := encodeQuestBlob(`{"a":1}`); got != oracle["questblob.encode"] {
		t.Errorf("encodeQuestBlob %s", got)
	}
	if got := decodeQuestBlob(oracle["questblob.encode"]); got != `{"a":1}` {
		t.Errorf("decodeQuestBlob %s", got)
	}
	inv := encodeQuestBlob(`{"equippedItems":["Hat","None","","Hat","Coat"]}`)
	if got := strings.Join(decodeEquippedItems(&inv), ","); got != oracle["equipped.decode"] {
		t.Errorf("decodeEquippedItems %s", got)
	}
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{ "b" : [1, 2.50, {"x" : "y\`+`u00e9\/"}], "a": true, "b": null , "n":-0.0e5}`))
	body := parseBody(hosting.NewRequest(req))
	if got := stj.MarshalString(body); got != oracle["body.roundtrip"] {
		t.Errorf("body %s want %s", got, oracle["body.roundtrip"])
	}
	if got := strings.Join(body.Keys(), ","); got != oracle["body.keys"] {
		t.Errorf("keys %s", got)
	}
	el := stj.MustParse([]byte(`{ "k" : [ 1 , "<" ] }`))
	if got := stj.MarshalString(stj.O("e", rewrittenElement{el})); got != oracle["element.reserialize"] {
		t.Errorf("element %s want %s", got, oracle["element.reserialize"])
	}
}

func TestApplyAutoCompletedQuestsMatchesOracle(t *testing.T) {
	newTestServer(t)
	oracle := loadOracle(t)
	if oracle["newline.length"] != "2" {
		t.Fatal("oracle was captured on a non-Windows host")
	}
	saved := newLine
	newLine = "\r\n" // the oracle (and production) ran on Windows
	defer func() { newLine = saved }()
	path := filepath.Join(testBaseDir, "auto_complete_quests.txt")
	if err := os.WriteFile(path, []byte("# comment\n0123456789abcdef0123456789ABCDEF 4\n  fedcba98-7654-3210-fedc-ba9876543210  # trailing\nbad 3\n11111111111111111111111111111111 x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	cl := userData()
	created := applyAutoCompletedQuests(cl, "user-1", nil)
	if created == nil || sv(created.Value) != oracle["quests.fromnull.value"] || created.Generation != 1 {
		t.Fatalf("from null: %+v", created)
	}

	sp := "<" + string(rune(0xE9)) + ">"
	existingJson := `{"lastUpdate":"2025.01.01-00.00.00","quests":[{"iD":"0123456789ABCDEF0123456789abcdef","progress":10,"completedVersion":1,"x":1.50},null,5,{"iD":"other","progress":255,"note":"` + sp + `","e":{},"arr":[]}],"savedQuestProgressionVersion":3,"extra":{"k":[1,true,null]}}`
	item := models.NewMothershipUserDataDbObject()
	item.DataId, item.UserId, item.KeyName, item.Value, item.Generation = strp("d1"), strp("user-2"), strp("player_quests"), strp(encodeQuestBlob(existingJson)), 5
	cl.Insert(item)
	updated := applyAutoCompletedQuests(cl, "user-2", item)
	if sv(updated.Value) != oracle["quests.existing.value"] || updated.Generation != 6 {
		t.Fatalf("existing:\n got %s\nwant %s", decodeQuestBlob(sv(updated.Value)), decodeQuestBlob(oracle["quests.existing.value"]))
	}
	// Written back: a second pass finds nothing to change.
	again := applyAutoCompletedQuests(cl, "user-2", findUserData(cl, "user-2", "player_quests"))
	if again.Generation != 6 {
		t.Fatalf("second pass changed the record: %d", again.Generation)
	}
	// A non-object blob is skipped (caught), the item is returned untouched.
	bad := models.NewMothershipUserDataDbObject()
	bad.Value = strp(encodeQuestBlob("[1]"))
	if got := applyAutoCompletedQuests(cl, "user-3", bad); got != bad || sv(got.Value) != encodeQuestBlob("[1]") {
		t.Fatal("bad blob not left alone")
	}
}
