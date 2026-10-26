package pak

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRealPak11 is opt-in: A2_PAK11_TEST=<path to a real v11 pak> (e.g. the
// Quest A2-Android_ASTC.pak.orig). Steps (b)-(d) also need
// PAK_EDITOR_OODLE_DLL. The real pak is only read; all writes happen on a
// copy in a temp directory.
func TestRealPak11(t *testing.T) {
	real := os.Getenv("A2_PAK11_TEST")
	if real == "" {
		t.Skip("set A2_PAK11_TEST to a real v11 pak path to run")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "A2-Android_ASTC.pak")
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
	var comp, oodle int
	for _, e := range p.Entries {
		if e.Compressed() {
			comp++
			if p.MethodName(e) == MethodOodle {
				oodle++
			}
		}
	}
	x := p.v11
	t.Logf("version %d, %d entries (%d compressed, %d Oodle), mount %q, seed %#x, methods %v", p.Footer.Version, len(p.Entries), comp, oodle, p.MountPoint, x.seed, p.CompressionMethods())
	t.Logf("index [%d,+%d) phi [%d,+%d) fdi [%d,+%d); encoded blob order differs from directory order: %v; pruned dirs %d",
		p.Footer.IndexOffset, p.Footer.IndexSize, x.phiOff, x.phiSize, x.fdiOff, x.fdiSize, x.encOrder[2] != p.Entries[2], len(x.pruned))

	// FNV path hashes: every stored hash must equal HashPath(name, seed).
	for _, r := range x.phi {
		if HashPath(r.entry.Name, x.seed) != r.hash {
			t.Fatalf("path hash mismatch for %q", r.entry.Name)
		}
	}
	t.Logf("all %d path hashes match HashPath", len(x.phi))

	// Full verify of the untouched pak (every inline header and SHA-1).
	rep, err := Verify(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("verify: %+v", rep)

	// (a) no-op rebuild is byte-identical
	same := filepath.Join(dir, "same.pak")
	if _, err := Rebuild(src, same, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sha256File(t, same), srcSum) {
		t.Fatal("no-op rebuild is NOT byte-identical")
	}
	t.Logf("(a) no-op rebuild byte-identical: sha256 %x", srcSum)
	os.Remove(same)

	dll := os.Getenv(OodleEnvVar)
	if dll == "" {
		t.Skip("set " + OodleEnvVar + " to run the Oodle steps (b)-(d)")
	}
	oo, err := LoadOodle(dll)
	if err != nil {
		t.Fatal(err)
	}
	const game = "A2/Config/DefaultGame.ini"
	const engine = "A2/Config/DefaultEngine.ini"
	if _, err := p.ReadEntry(game); err == nil || !strings.Contains(err.Error(), "Oodle") {
		t.Fatalf("expected Oodle-required error without decompressor, got %v", err)
	}
	p.Decompressor = oo
	ge, _ := p.Entry(game)
	ee, _ := p.Entry(engine)
	t.Logf("%s: stored %d, raw %d, blocks %d; %s: stored %d, raw %d, blocks %d", game, ge.Size, ge.UncompressedSize, len(ge.Blocks), engine, ee.Size, ee.UncompressedSize, len(ee.Blocks))
	origGame, err := p.ReadEntry(game)
	if err != nil {
		t.Fatal(err)
	}
	origEngine, err := p.ReadEntry(engine)
	if err != nil {
		t.Fatal(err)
	}
	// (d) the 2-block DefaultEngine.ini decompresses to its full size
	if int64(len(origEngine)) != ee.UncompressedSize || len(ee.Blocks) != 2 {
		t.Fatalf("DefaultEngine.ini: %d bytes, %d blocks", len(origEngine), len(ee.Blocks))
	}
	doc, err := DecodeIni(origGame)
	if err != nil {
		t.Fatal(err)
	}
	if doc.LineEnding != CRLF || !bytes.Equal(doc.Encode(doc.Text), origGame) {
		t.Fatal("ini round trip not identical / not CRLF")
	}
	newGame := doc.Encode(doc.Text + "; edited by solara pak editor test\n")

	for _, recompress := range []bool{false, true} {
		repl := map[string][]byte{game: newGame}
		res, err := Save(src, repl, &SaveOptions{Codec: oo, Recompress: recompress})
		if err != nil {
			t.Fatalf("recompress=%v: %v", recompress, err)
		}
		if _, err := Verify(src, &VerifyOptions{Source: res.OriginalBackup, Expect: repl, Decompressor: oo}); err != nil {
			t.Fatal(err)
		}
		np, err := Open(src)
		if err != nil {
			t.Fatal(err)
		}
		np.Decompressor = oo
		ne, _ := np.Entry(game)
		if ne.Compressed() != recompress || (len(res.Recompressed) == 1) != recompress {
			t.Fatalf("recompress=%v: entry compressed=%v, result %+v", recompress, ne.Compressed(), res)
		}
		got, err := np.ReadEntry(game)
		if err != nil || !bytes.Equal(got, newGame) {
			t.Fatalf("recompress=%v: re-read edited entry mismatch: %v", recompress, err)
		}
		gotEngine, err := np.ReadEntry(engine)
		if err != nil || !bytes.Equal(gotEngine, origEngine) {
			t.Fatalf("(d) DefaultEngine.ini differs after save: %v", err)
		}
		op, _ := Open(res.OriginalBackup)
		of, _ := os.Open(res.OriginalBackup)
		nf, _ := os.Open(src)
		b1, b2 := make([]byte, 1<<20), make([]byte, 1<<20)
		same := 0
		for i, e := range np.Entries {
			oe := op.Entries[i]
			if e.Name != oe.Name {
				t.Fatalf("entry order changed at %d", i)
			}
			if e.Name == game {
				continue
			}
			if e.Hash == oe.Hash && e.Size == oe.Size && e.RegionSize() == oe.RegionSize() && sameBytes(nf, e.Offset, of, oe.Offset, e.RegionSize(), b1, b2) {
				same++
			}
		}
		of.Close()
		nf.Close()
		if same != len(np.Entries)-1 {
			t.Fatalf("only %d of %d untouched entries are byte-identical", same, len(np.Entries)-1)
		}
		st, _ := os.Stat(src)
		t.Logf("(%s) recompress=%v: new size %d (delta %+d), %s stored %d bytes (method %d, %d blocks); %d other entries byte-identical; DefaultEngine.ini round trip ok",
			map[bool]string{false: "b", true: "c"}[recompress], recompress, st.Size(), st.Size()-p.FileSize, game, ne.Size, ne.CompressionMethod, len(ne.Blocks), same)
		if _, err := Restore(src, res.OriginalBackup, nil); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(sha256File(t, src), srcSum) {
			t.Fatal("restore of .original not identical")
		}
	}
}
