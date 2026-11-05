package docdb

import (
	"os"
	"path/filepath"
	"testing"
)

// A metadata file copied from another machine stores that machine's path; it must still save where it now is.
func TestBytedataMovedPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.db.meta")
	b, err := NewBytedata(p)
	if err != nil {
		t.Fatal(err)
	}
	b.CompletePath = `C:\elsewhere\x.db.meta`
	if err := os.WriteFile(p, b.encode(), 0o600); err != nil {
		t.Fatal(err)
	}
	b2, err := NewBytedata(p)
	if err != nil {
		t.Fatal(err)
	}
	if b2.CompletePath != p {
		t.Fatalf("CompletePath = %q, want %q", b2.CompletePath, p)
	}
	b2.Add("c1", "T")
	b2.Save()
	b3, err := NewBytedata(p)
	if err != nil {
		t.Fatal(err)
	}
	if !b3.Has("c1") {
		t.Fatal("collection list was not saved")
	}
}
