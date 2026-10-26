package pak

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealPak is opt-in: A2_PAK_TEST=<path to a real v8 pak>. The real pak
// is only read; all writes happen on a copy in a temp directory.
func TestRealPak(t *testing.T) {
	real := os.Getenv("A2_PAK_TEST")
	if real == "" {
		t.Skip("set A2_PAK_TEST to a real pak path to run")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, filepath.Base(real))
	if _, err := copyFile(real, src, nil); err != nil {
		t.Fatal(err)
	}
	srcSum := sha256File(t, src)
	if !bytes.Equal(srcSum, sha256File(t, real)) {
		t.Fatal("copy of real pak differs")
	}
	p, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("version %d, %d entries, mount %q, index at %d (+%d)", p.Footer.Version, len(p.Entries), p.MountPoint, p.Footer.IndexOffset, p.Footer.IndexSize)

	// (a) no-op rebuild is byte-identical
	same := filepath.Join(dir, "same.pak")
	if _, err := Rebuild(src, same, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sha256File(t, same), srcSum) {
		t.Fatal("no-op rebuild is NOT byte-identical")
	}
	t.Logf("no-op rebuild byte-identical: sha256 %x", srcSum)
	os.Remove(same)

	// (b) edit DefaultGame.ini through the safe save pipeline
	const name = "A2/Config/DefaultGame.ini"
	raw, err := p.ReadEntry(name)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := DecodeIni(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.LineEnding != CRLF || doc.BOM {
		t.Fatalf("unexpected ini format: %+v", doc.LineEnding)
	}
	if !bytes.Equal(doc.Encode(doc.Text), raw) {
		t.Fatal("ini decode/encode round trip not identical")
	}
	edited := doc.Text + "; edited by solara pak editor test\n"
	newBytes := doc.Encode(edited)
	res, err := Save(src, map[string][]byte{name: newBytes}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("save result: %+v", res)
	if _, err := Verify(src, &VerifyOptions{Source: res.OriginalBackup, Expect: map[string][]byte{name: newBytes}}); err != nil {
		t.Fatal(err)
	}
	np, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := np.ReadEntry(name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBytes) || !strings.HasSuffix(string(got), "; edited by solara pak editor test\r\n") {
		t.Fatal("re-read edited entry mismatch / not CRLF")
	}
	// All other entries identical (data + hash) to the original.
	op, _ := Open(res.OriginalBackup)
	of, _ := os.Open(res.OriginalBackup)
	defer of.Close()
	nf, _ := os.Open(src)
	defer nf.Close()
	b1, b2 := make([]byte, 1<<20), make([]byte, 1<<20)
	diff := 0
	for i, e := range np.Entries {
		oe := op.Entries[i]
		if e.Name == name {
			continue
		}
		if e.Hash != oe.Hash || e.Size != oe.Size || !sameBytes(nf, e.Offset+e.HeaderSize(), of, oe.Offset+oe.HeaderSize(), e.Size, b1, b2) {
			diff++
		}
	}
	if diff != 0 {
		t.Fatalf("%d untouched entries differ", diff)
	}
	t.Logf("%d other entries identical", len(np.Entries)-1)
	of.Close()
	nf.Close() // our own handles would block the replace on Windows
	// Restoring .original gives back the exact original bytes.
	if _, err := Restore(src, res.OriginalBackup, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sha256File(t, src), srcSum) {
		t.Fatal("restore of .original not identical")
	}
}

func sha256File(t *testing.T, p string) []byte {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return h.Sum(nil)
}
