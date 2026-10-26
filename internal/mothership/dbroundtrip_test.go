package mothership

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

// populateMixed applies the same rule as tools/oracle-mothership-db Populate():
// the k-th string member gets "", "  pad_<Name>  ", its initializer, or
// "<Name>_" + special (k%4); lists/dictionaries carry "" and padded values.
func populateMixed(v any) {
	special := "v<&'" + string(rune(0xE9)) + ">"
	rv := reflect.ValueOf(v).Elem()
	rt := rv.Type()
	k := 0
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Anonymous {
			continue
		}
		fv := rv.Field(i)
		switch fv.Interface().(type) {
		case *string:
			switch k % 4 {
			case 0:
				fv.Set(reflect.ValueOf(models.P("")))
			case 1:
				fv.Set(reflect.ValueOf(models.P("  pad_" + f.Name + "  ")))
			case 3:
				fv.Set(reflect.ValueOf(models.P(f.Name + "_" + special)))
			}
			k++
		case int:
			fv.SetInt(7)
		case int64:
			fv.SetInt(9007199254740993)
		case bool:
			fv.SetBool(!fv.Bool())
		case dotnet.DateTime:
			fv.Set(reflect.ValueOf(dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234567)))
		case []*string:
			fv.Set(reflect.ValueOf(models.L("", "  x  ", "y", "   ")))
		case stj.Obj:
			fv.Set(reflect.ValueOf(stj.O("z", "", "a", " 2 ", "m", "m", "w", "  ")))
		default:
			panic("unhandled " + f.Name)
		}
	}
}

func roundTrip[T any](t *testing.T, db *docdb.Database, newT func() *T, mixed bool) string {
	t.Helper()
	c := docdb.GetCollection[T](db, true, newT)
	c.DeleteAll()
	v := newT()
	if mixed {
		populateMixed(v)
	}
	c.Insert(v)
	all := c.FindAll()
	if len(all) != 1 {
		t.Fatalf("FindAll returned %d documents", len(all))
	}
	return stj.MarshalString(all[0])
}

func rtBoth[T any](newT func() *T) func(*testing.T, *docdb.Database, bool) string {
	return func(t *testing.T, db *docdb.Database, mixed bool) string { return roundTrip(t, db, newT, mixed) }
}

var roundTrips = map[string]func(*testing.T, *docdb.Database, bool) string{
	"MothershipAnalyticsEventDbObject":          rtBoth(models.NewMothershipAnalyticsEventDbObject),
	"MothershipApiKeyDbObject":                  rtBoth(models.NewMothershipApiKeyDbObject),
	"MothershipAssociationDbObject":             rtBoth(models.NewMothershipAssociationDbObject),
	"MothershipBanDbObject":                     rtBoth(models.NewMothershipBanDbObject),
	"MothershipDeploymentDbObject":              rtBoth(models.NewMothershipDeploymentDbObject),
	"MothershipEntitlementDbObject":             rtBoth(models.NewMothershipEntitlementDbObject),
	"MothershipEnvironmentDbObject":             rtBoth(models.NewMothershipEnvironmentDbObject),
	"MothershipFriendEntryDbObject":             rtBoth(models.NewMothershipFriendEntryDbObject),
	"MothershipInventoryItemDbObject":           rtBoth(models.NewMothershipInventoryItemDbObject),
	"MothershipLinkDbObject":                    rtBoth(models.NewMothershipLinkDbObject),
	"MothershipMuteDbObject":                    rtBoth(models.NewMothershipMuteDbObject),
	"MothershipNotificationDbObject":            rtBoth(models.NewMothershipNotificationDbObject),
	"MothershipOfferBindingDbObject":            rtBoth(models.NewMothershipOfferBindingDbObject),
	"MothershipOfferDbObject":                   rtBoth(models.NewMothershipOfferDbObject),
	"MothershipOfferDisplayDbObject":            rtBoth(models.NewMothershipOfferDisplayDbObject),
	"MothershipPlayerDbObject":                  rtBoth(models.NewMothershipPlayerDbObject),
	"MothershipPrivacyStateDbObject":            rtBoth(models.NewMothershipPrivacyStateDbObject),
	"MothershipProgressionTrackBindingDbObject": rtBoth(models.NewMothershipProgressionTrackBindingDbObject),
	"MothershipProgressionTrackDbObject":        rtBoth(models.NewMothershipProgressionTrackDbObject),
	"MothershipProgressionTreeBindingDbObject":  rtBoth(models.NewMothershipProgressionTreeBindingDbObject),
	"MothershipProgressionTreeDbObject":         rtBoth(models.NewMothershipProgressionTreeDbObject),
	"MothershipReportDbObject":                  rtBoth(models.NewMothershipReportDbObject),
	"MothershipSessionDbObject":                 rtBoth(models.NewMothershipSessionDbObject),
	"MothershipSharedGroupDbObject":             rtBoth(models.NewMothershipSharedGroupDbObject),
	"MothershipTitleDataDbObject":               rtBoth(models.NewMothershipTitleDataDbObject),
	"MothershipTitleDbObject":                   rtBoth(models.NewMothershipTitleDbObject),
	"MothershipTransactionCatalogDbObject":      rtBoth(models.NewMothershipTransactionCatalogDbObject),
	"MothershipUserDataDbObject":                rtBoth(models.NewMothershipUserDataDbObject),
	"MothershipUserDataMetadataDbObject":        rtBoth(models.NewMothershipUserDataMetadataDbObject),
	"MothershipV2PlayerDbObject":                rtBoth(models.NewMothershipV2PlayerDbObject),
}

// TestDbRoundTripMatchesLiteDB stores every model through docdb, reads it back
// and compares the JSON with what the original produced after a real LiteDB
// 5.0.21 Insert + FindAll (testdata/dbroundtrip.json, tools/oracle-mothership-db).
func TestDbRoundTripMatchesLiteDB(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "dbroundtrip.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle map[string]string
	if err := json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 1, Name: "rt", Path: filepath.Join(t.TempDir(), "rt"+docdb.FileSuffix)})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	n := 0
	for name, rt := range roundTrips {
		for _, mode := range []string{"default", "mixed"} {
			want, ok := oracle["rt."+mode+"."+name]
			if !ok {
				t.Errorf("no fixture rt.%s.%s", mode, name)
				continue
			}
			if got := rt(t, db, mode == "mixed"); got != want {
				t.Errorf("rt.%s.%s:\n got %s\nwant %s", mode, name, got, want)
			}
			n++
		}
	}
	if n != len(oracle) {
		t.Errorf("compared %d fixtures, oracle has %d", n, len(oracle))
	}
}

// TestDbPredicateNullSemantics pins the LiteDB 5.0.21 predicate behaviour
// printed by tools/oracle-mothership-db: stored {"Abc", "" (-> null),
// "  pad  " (-> "pad")}; a captured value is compared verbatim, ignoring case.
func TestDbPredicateNullSemantics(t *testing.T) {
	stored := []*string{models.P("Abc"), nil, models.P("pad")}
	match := func(q string) (out []int) {
		for i, s := range stored {
			if dbEq(s, q) {
				out = append(out, i)
			}
		}
		return out
	}
	cases := map[string][]int{"": nil, " ": nil, "abc": {0}, " Abc ": nil, "pad": {2}, "  pad  ": nil}
	for q, want := range cases {
		if got := match(q); !reflect.DeepEqual(got, want) {
			t.Errorf("Token == %q -> %v, want %v", q, got, want)
		}
	}
	if !dbEqP(nil, nil) || dbEqP(nil, models.P("")) || dbEqP(models.P("x"), nil) || !dbEqP(models.P("X"), models.P("x")) {
		t.Error("dbEqP null semantics")
	}
	if !eqP(nil, nil) || eqP(nil, models.P("")) || eqP(models.P("X"), models.P("x")) || eqS(nil, "") {
		t.Error("eqP/eqS ordinal null semantics")
	}
}
