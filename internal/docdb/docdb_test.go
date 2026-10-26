package docdb

import (
	"os"
	"path/filepath"
	"reflect"
	"solara/internal/stj"
	"strings"
	"testing"

	"solara/internal/dotnet"
	"solara/internal/logging"
)

func init() { logging.DisableFiles = true }

type Widget struct {
	AstraDbObject
	Name   string
	Owner  *string
	Tags   []string
	When   dotnet.DateTime
	Maybe  *dotnet.DateTime
	Config map[string]string
	Count  int
}

func NewWidget() *Widget { return &Widget{Name: "", Tags: []string{}, Config: map[string]string{}} }

func TestCollectionName(t *testing.T) {
	// "_" + md5("Widget")[:7] + "_widget"
	if got := CollectionName("Widget"); len(got) != 1+7+1+6 || got[0] != '_' || got[9:] != "widget" {
		t.Fatalf("%s", got)
	}
}

func TestCrudRoundTripEncrypted(t *testing.T) {
	dir := t.TempDir()
	pw := "pw"
	db, err := Open(&Descriptor{DatabaseId: 1, Name: "t", Path: filepath.Join(dir, "t"+FileSuffix), Password: &pw})
	if err != nil {
		t.Fatal(err)
	}
	c := GetCollection[Widget](db, true, NewWidget)
	w := NewWidget()
	w.Name = "Alice"
	w.Tags = []string{"Red"}
	w.When = dotnet.New(2025, 1, 2, 3, 4, 5, dotnet.Utc).AddTicks(1234567)
	c.Insert(w)
	c.Insert(&Widget{Name: "bob", Count: 2})

	got := c.FindOne(func(x *Widget) bool { return EqFold(x.Name, "alice") })
	if got == nil || got.Name != "Alice" || got.Owner != nil || got.Maybe != nil {
		t.Fatalf("FindOne: %+v", got)
	}
	if got.When.Kind() != dotnet.Local || got.When.ToUniversalTime().Ticks() != w.When.Ticks()-4567 {
		t.Errorf("date not ms-truncated/Local: %v %v", got.When.Kind(), got.When.FormatO())
	}
	if !c.Exists(func(x *Widget) bool { return ContainsFold(x.Tags, "red") }) {
		t.Error("ContainsFold")
	}
	// absent members keep constructor defaults (bob was inserted with nil Tags -> not stored)
	bob := c.FindOne(func(x *Widget) bool { return x.Count == 2 })
	if bob.Tags == nil || len(bob.Tags) != 0 {
		t.Errorf("default not applied: %#v", bob.Tags)
	}
	if order := c.FindAll(); order[0].Name != "Alice" || order[1].Name != "bob" {
		t.Error("_id order")
	}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("duplicate insert must panic")
			}
		}()
		c.Insert(got)
	}()
	if c.Update(NewWidget()) {
		t.Error("update of unknown doc must be false")
	}
	if !c.Upsert(NewWidget()) {
		t.Error("upsert new must be true")
	}
	if n := c.DeleteMany(func(x *Widget) bool { return x.Name == "" }); n != 1 {
		t.Errorf("DeleteMany %d", n)
	}
	_ = db.Close()

	wrong := "nope"
	if _, err := Open(&Descriptor{Name: "t", Path: filepath.Join(dir, "t"+FileSuffix), Password: &wrong}); err == nil {
		t.Error("wrong password must fail")
	}
	if _, err := os.Stat(filepath.Join(dir, "t"+FileSuffix+".meta")); err != nil {
		t.Error("meta sidecar missing")
	}
	meta, err := NewBytedata(filepath.Join(dir, "t"+FileSuffix+".meta"))
	if err != nil || len(meta.Collections) != 1 || meta.Collections[0].MaxItems != 100000 {
		t.Errorf("meta: %v %+v", err, meta)
	}
}

func TestEncodeObjWithNil(t *testing.T) {
	type Doc struct {
		AstraDbObject
		Data stj.Obj
	}
	raw, err := encodeDoc(reflect.ValueOf(Doc{Data: stj.O("a", nil, "b", 1)}))
	if err != nil || !strings.Contains(string(raw), `"Data":{"a":null,"b":1}`) {
		t.Fatalf("%v %s", err, raw)
	}
}

// LiteDB BsonMapper defaults (verified against LiteDB 5.0.21 via tools/oracle):
// strings are trimmed, "" is written as an explicit null (overriding the
// initializer on read), a genuinely null member is omitted (initializer kept).
func TestEmptyStringToNullAndTrim(t *testing.T) {
	type D struct {
		AstraDbObject
		A *string
		B *string
		G *string
		H *string
		L []*string
		M map[string]*string
	}
	e, sp, pad, x := "", "  ", "  pad  ", " x "
	raw, err := encodeDoc(reflect.ValueOf(D{A: &e, B: &pad, H: &sp, L: []*string{&e, &x}, M: map[string]*string{"k": &e}}))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"A":null`, `"B":"pad"`, `"H":null`, `"L":[null,"x"]`, `"M":{"k":null}`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s in %s", want, s)
		}
	}
	if strings.Contains(s, `"G"`) {
		t.Errorf("null member must be omitted: %s", s)
	}
	init := "init"
	d := D{A: &init, G: &init}
	if err := decodeDoc(raw, reflect.ValueOf(&d).Elem()); err != nil {
		t.Fatal(err)
	}
	if d.A != nil || d.G == nil || *d.G != "init" || d.L[0] != nil || d.M["k"] != nil {
		t.Errorf("read-back: %+v", d)
	}
}
