package docdb

import (
	"path/filepath"
	"testing"

	"solara/internal/dotnet"
)

// ImportedWidget matches the C# class in tools/oracle used to create
// testdata/export with the real LiteDB 5.0.21 (encrypted) and
// tools/litedb-export.
type ImportedWidget struct {
	AstraDbObject
	Name   string
	Owner  *string
	Tags   []string
	When   dotnet.DateTime
	Big    int64
	Ratio  float64
	Config map[string]string
	Count  int
}

func (ImportedWidget) DotnetTypeName() string { return "Widget" }
func (ImportedWidget) DotnetFullName() string { return "Widget" }

func TestImportLiteDBExport(t *testing.T) {
	pw := "pw"
	db, err := Open(&Descriptor{Name: "i", Path: filepath.Join(t.TempDir(), "i"+FileSuffix), Password: &pw})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rep, err := ImportLiteDBExport(db, "testdata/export", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Collections[CollectionName("Widget")] != 2 {
		t.Fatalf("report %v (expected collection %s)", rep.Collections, CollectionName("Widget"))
	}
	c := GetCollection[ImportedWidget](db, false, nil)
	if c == nil {
		t.Fatal("collection not registered in meta")
	}
	all := c.FindAll()
	a, b := all[0], all[1]
	if a.Id.Hex() != "6ade83ed5b02020cf8c6c1d7" || a.Name != "Alice" || a.Big != 9007199254740993 || a.Ratio != 0.1 || a.Config["k"] != "v" || a.Count != 3 || a.Owner != nil {
		t.Errorf("alice: %+v", a)
	}
	// stored with ms precision; LiteDB reads it back as Local
	want := dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1230000)
	if a.When.Kind() != dotnet.Local || a.When.ToUniversalTime().Ticks() != want.Ticks() {
		t.Errorf("when %s", a.When.FormatO())
	}
	if b.Owner == nil || *b.Owner != "alice" || b.When != dotnet.MinValue {
		t.Errorf("bob: %+v when=%s", b, b.When.FormatO())
	}
	// a second import must not clobber existing data
	if _, err := ImportLiteDBExport(db, "testdata/export", false); err == nil {
		t.Error("re-import without overwrite must be refused")
	}
}
