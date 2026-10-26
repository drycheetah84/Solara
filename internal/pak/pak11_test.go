package pak

import (
	"bytes"
	"compress/flate"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// flateCodec is a test codec registered under the "Oodle" method name: it
// lets the compressed code paths (multi-block entries, recompress) be
// tested without the Oodle DLL.
type flateCodec struct{ calls int }

func (c *flateCodec) Compress(method string, raw []byte) ([]byte, error) {
	if method != MethodOodle {
		return nil, fmt.Errorf("method %q", method)
	}
	c.calls++
	var b bytes.Buffer
	w, _ := flate.NewWriter(&b, flate.BestCompression)
	w.Write(raw)
	w.Close()
	return b.Bytes(), nil
}

func (c *flateCodec) Decompress(method string, comp []byte, rawLen int) ([]byte, error) {
	if method != MethodOodle {
		return nil, fmt.Errorf("method %q", method)
	}
	out, err := io.ReadAll(flate.NewReader(bytes.NewReader(comp)))
	if err != nil {
		return nil, err
	}
	if len(out) != rawLen {
		return nil, fmt.Errorf("got %d bytes, want %d", len(out), rawLen)
	}
	return out, nil
}

var bigIni = strings.Repeat("[/Script/Engine.RendererSettings]\r\nr.Shadow.Virtual.Enable=1\r\nr.DefaultFeature.MotionBlur=False\r\n", 150)

// v11Files are grouped by directory (directory-index order = files order).
func v11Files() []NewFile {
	return []NewFile{
		{Name: "README.txt", Data: []byte("root level file")},
		{Name: "A2/A2.uproject", Data: []byte("{\"FileVersion\": 3}")},
		{Name: "A2/Config/DefaultEngine.ini", Data: []byte(bigIni), CompressionMethod: 1, CompressionBlockSize: 4096},
		{Name: "A2/Config/DefaultGame.ini", Data: []byte(gameIni), CompressionMethod: 1},
		{Name: "A2/Config/DefaultInput.ini", Data: []byte("[X]\r\nA=1\r\n")},
		{Name: "A2/Content/Big.uasset", Data: bytes.Repeat([]byte{1, 2, 3, 4, 5}, 5000)},
		{Name: "A2/Content/Secret.uasset", Data: []byte("encrypted-bytes!!"), Encrypted: true},
		{Name: "A2/Content/Empty.bin", Data: nil},
		{Name: "A2/Content/Ünïcode.uasset", Data: []byte("utf16 name")},
	}
}

func v11Opts(c Compressor) *WriteOptions {
	return &WriteOptions{
		Version:          VersionFnv64BugFix,
		Compressor:       c,
		CompressionNames: []string{MethodOodle},
		PhysicalOrder:    []int{5, 0, 3, 8, 2, 7, 1, 6, 4},
		GapBefore:        map[int]int{2: 7},
		EncodedOrder:     []int{8, 7, 6, 5, 4, 3, 2, 1, 0},
		NonEncoded:       map[int]bool{4: true},
		PathHashSeed:     0xabd9dff0,
	}
}

func writeV11(t *testing.T, dir string, c Compressor) string {
	t.Helper()
	path := filepath.Join(dir, "Test-Android_ASTC.pak")
	if err := WriteNew(path, "../../../", v11Files(), v11Opts(c)); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestV11ParseAndReadFlate(t *testing.T) {
	codec := &flateCodec{}
	path := writeV11(t, t.TempDir(), codec)
	p, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Footer.Version != 11 || len(p.Entries) != 9 || p.MountPoint != "../../../" || p.PathHashSeed() != 0xabd9dff0 {
		t.Fatalf("version %d entries %d mount %q", p.Footer.Version, len(p.Entries), p.MountPoint)
	}
	for i, f := range v11Files() {
		if p.Entries[i].Name != f.Name || p.Entries[i].IndexPos != i {
			t.Fatalf("entry %d is %q, want %q", i, p.Entries[i].Name, f.Name)
		}
	}
	eng, _ := p.Entry("A2/Config/DefaultEngine.ini")
	if !eng.Compressed() || len(eng.Blocks) < 3 || eng.CompressionBlockSize != 4096 || !eng.Editable() || p.MethodName(eng) != MethodOodle {
		t.Fatalf("engine entry %+v", eng)
	}
	if _, err := p.ReadEntry("A2/Config/DefaultEngine.ini"); !errors.Is(err, ErrOodleRequired) {
		t.Fatalf("want ErrOodleRequired, got %v", err)
	}
	p.Decompressor = codec
	for _, f := range v11Files() {
		if f.Encrypted {
			continue
		}
		got, err := p.ReadEntry(f.Name)
		if err != nil || !bytes.Equal(got, f.Data) {
			t.Fatalf("%s: %v", f.Name, err)
		}
	}
	if sec, _ := p.Entry("A2/Content/Secret.uasset"); sec.Editable() {
		t.Fatal("encrypted entry editable")
	}
	if _, err := Verify(path, nil); err != nil {
		t.Fatal(err)
	}
	if len(p.v11.nonEnc) != 1 || p.v11.nonEnc[0].Name != "A2/Config/DefaultInput.ini" || len(p.v11.pruned) != 1 || len(p.v11.pruned[0].files) != 3 {
		t.Fatalf("non-encoded %d pruned %+v", len(p.v11.nonEnc), p.v11.pruned)
	}
}

func TestV11UnsupportedMethod(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "z.pak")
	o := v11Opts(nil)
	o.CompressionNames = []string{"Zlib"}
	if err := WriteNew(path, "../../../", v11Files(), o); err != nil {
		t.Fatal(err)
	}
	p, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p.Decompressor = &flateCodec{}
	var uc *UnsupportedCompressionError
	if _, err := p.ReadEntry("A2/Config/DefaultGame.ini"); !errors.As(err, &uc) || uc.Method != "Zlib" {
		t.Fatalf("want UnsupportedCompressionError, got %v", err)
	}
}

func TestV11RebuildIdentityAndEdits(t *testing.T) {
	dir := t.TempDir()
	codec := &flateCodec{}
	src := writeV11(t, dir, codec)
	same := filepath.Join(dir, "same.pak")
	if _, err := Rebuild(src, same, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileBytes(t, src), fileBytes(t, same)) {
		t.Fatal("v11 no-op rebuild is not byte-identical")
	}

	newEngine := []byte(bigIni + "; edited\r\n")
	newInput := []byte("[X]\r\nA=2\r\n; non-encoded entry edited\r\n")
	newRoot := []byte("root level file, edited and longer")
	repl := map[string][]byte{
		"A2/Config/DefaultEngine.ini": newEngine, // compressed -> uncompressed
		"A2/Config/DefaultInput.ini":  newInput,  // non-encoded entry
		"README.txt":                  newRoot,
	}
	for _, recompress := range []bool{false, true} {
		dst := filepath.Join(dir, fmt.Sprintf("out-%v.pak", recompress))
		res, err := RebuildWith(src, dst, repl, &RebuildOptions{Recompress: recompress, Codec: codec})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Changed) != 3 {
			t.Fatalf("changed %v", res.Changed)
		}
		if recompress != (len(res.Recompressed) == 1 && res.Recompressed[0] == "A2/Config/DefaultEngine.ini") {
			t.Fatalf("recompressed %v", res.Recompressed)
		}
		if _, err := Verify(dst, &VerifyOptions{Source: src, Expect: repl, Decompressor: codec}); err != nil {
			t.Fatal(err)
		}
		if recompress {
			if _, err := Verify(dst, &VerifyOptions{Source: src, Expect: repl}); err == nil {
				t.Fatal("compressed replacement verified without a decompressor")
			}
		}
		p, err := Open(dst)
		if err != nil {
			t.Fatal(err)
		}
		p.Decompressor = codec
		e, _ := p.Entry("A2/Config/DefaultEngine.ini")
		if e.Compressed() != recompress {
			t.Fatalf("recompress=%v but compressed=%v", recompress, e.Compressed())
		}
		if recompress && (len(e.Blocks) != len(newEngine)/4096+1 || e.CompressionBlockSize != 4096) {
			t.Fatalf("blocks %d cbs %d", len(e.Blocks), e.CompressionBlockSize)
		}
		for _, f := range v11Files() {
			want := f.Data
			if r, ok := repl[f.Name]; ok {
				want = r
			}
			if f.Encrypted {
				continue
			}
			if got, err := p.ReadEntry(f.Name); err != nil || !bytes.Equal(got, want) {
				t.Fatalf("%s after rebuild: %v", f.Name, err)
			}
		}
		var problems []string
		p.checkV11Hashes(func(f string, a ...any) { problems = append(problems, fmt.Sprintf(f, a...)) })
		if len(problems) > 0 {
			t.Fatal(problems)
		}
		sp, _ := Open(src)
		if !bytes.Equal(p.canonicalV11(), sp.canonicalV11()) {
			t.Fatal("v11 structure changed")
		}
		if _, err := Verify(dst, &VerifyOptions{Source: src}); err == nil {
			t.Fatal("verify against source without expectations should fail")
		}
	}
	// Reverting the uncompressed edits of uncompressed entries restores
	// their exact bytes.
	dst := filepath.Join(dir, "rev1.pak")
	if _, err := Rebuild(src, dst, map[string][]byte{"README.txt": newRoot}, nil); err != nil {
		t.Fatal(err)
	}
	back := filepath.Join(dir, "rev2.pak")
	if _, err := Rebuild(dst, back, map[string][]byte{"README.txt": []byte("root level file")}, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileBytes(t, src), fileBytes(t, back)) {
		t.Fatal("reverting an edit is not byte-identical to the source")
	}
}

func TestV11RecompressFallbackAndErrors(t *testing.T) {
	dir := t.TempDir()
	codec := &flateCodec{}
	src := writeV11(t, dir, codec)
	noise := make([]byte, 3000)
	rand.New(rand.NewSource(1)).Read(noise)
	repl := map[string][]byte{"A2/Config/DefaultGame.ini": noise}
	dst := filepath.Join(dir, "o.pak")
	res, err := RebuildWith(src, dst, repl, &RebuildOptions{Recompress: true, Codec: codec})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Recompressed) != 0 {
		t.Fatal("incompressible data should be stored uncompressed")
	}
	if _, err := Verify(dst, &VerifyOptions{Source: src, Expect: repl, Decompressor: codec}); err != nil {
		t.Fatal(err)
	}
	if _, err := RebuildWith(src, dst, repl, &RebuildOptions{Recompress: true}); err == nil {
		t.Fatal("recompress without codec accepted")
	}
	if _, err := Rebuild(src, dst, map[string][]byte{"A2/Content/Secret.uasset": []byte("x")}, nil); err == nil {
		t.Fatal("encrypted replacement accepted")
	}
	v8 := writeTestPak(t, dir)
	if _, err := RebuildWith(v8, dst, map[string][]byte{"A2/Config/DefaultGame.ini": []byte("x")}, &RebuildOptions{Recompress: true, Codec: codec}); err == nil {
		t.Fatal("recompress accepted for v8")
	}
}

func TestV11VerifyDetectsCorruption(t *testing.T) {
	dir := t.TempDir()
	src := writeV11(t, dir, &flateCodec{})
	orig := fileBytes(t, src)
	p, _ := Open(src)
	big, _ := p.Entry("A2/Content/Big.uasset")
	x := p.v11
	cases := map[string]func(b []byte){
		"data byte":     func(b []byte) { b[big.Offset+big.HeaderSize()+100] ^= 0xFF },
		"inline size":   func(b []byte) { b[big.Offset+8] ^= 0x01 },
		"primary index": func(b []byte) { b[p.Footer.IndexOffset+30] ^= 0x01 },
		"phi byte":      func(b []byte) { b[x.phiOff+6] ^= 0x01 },
		"fdi byte":      func(b []byte) { b[x.fdiOff+10] ^= 0x01 },
		"index hash":    func(b []byte) { b[len(b)-221+41] ^= 0x01 },
	}
	for name, mutate := range cases {
		b := append([]byte(nil), orig...)
		mutate(b)
		bad := filepath.Join(dir, "bad.pak")
		os.WriteFile(bad, b, 0o644)
		if _, err := Verify(bad, nil); err == nil {
			t.Errorf("%s: corruption not detected", name)
		}
	}
	// A wrong path hash with all SHA-1s fixed up is caught by Verify.
	b := append([]byte(nil), orig...)
	b[x.phiOff+4] ^= 0x01 // first hash value
	phi := b[x.phiOff : x.phiOff+x.phiSize]
	h := sha1.Sum(phi)
	// the PHI hash sits after mount(14) + count(4) + seed(8) + flag(4) + off(8) + size(8)
	idx := b[p.Footer.IndexOffset : p.Footer.IndexOffset+p.Footer.IndexSize]
	copy(idx[14+4+8+4+16:], h[:])
	ih := sha1.Sum(idx)
	copy(b[len(b)-221+41:], ih[:])
	bad := filepath.Join(dir, "bad-phi.pak")
	os.WriteFile(bad, b, 0o644)
	if _, err := Open(bad); err != nil {
		t.Fatalf("fixed-up pak should parse: %v", err)
	}
	if _, err := Verify(bad, nil); err == nil || !strings.Contains(err.Error(), "path hash") {
		t.Fatalf("path hash mismatch not detected: %v", err)
	}
}

func TestV11SaveRestore(t *testing.T) {
	dir := t.TempDir()
	codec := &flateCodec{}
	pak := writeV11(t, dir, codec)
	orig := fileBytes(t, pak)
	opts := &SaveOptions{Now: clock(time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)), Codec: codec, Recompress: true}
	const name = "A2/Config/DefaultEngine.ini"
	content := []byte(bigIni + "; save test\r\n")
	res, err := Save(pak, map[string][]byte{name: content}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Recompressed) != 1 || len(res.Changed) != 1 {
		t.Fatalf("result %+v", res)
	}
	p, _ := Open(pak)
	p.Decompressor = codec
	if got, err := p.ReadEntry(name); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("saved content: %v", err)
	}
	if !bytes.Equal(fileBytes(t, OriginalPath(pak)), orig) {
		t.Fatal(".original is not the pristine pak")
	}
	if _, err := Restore(pak, OriginalPath(pak), opts); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileBytes(t, pak), orig) {
		t.Fatal("restore not identical")
	}
}

// blocksFromSizes lays out blocks the way DecodePakEntry derives them.
func blocksFromSizes(e *Entry, sizes ...int64) {
	e.Blocks = make([]Block, len(sizes))
	cur := e.HeaderSize()
	for i, s := range sizes {
		e.Blocks[i] = Block{Start: cur, End: cur + s}
		if e.Encrypted() {
			s = align16(s)
		}
		cur += s
	}
}

func TestEncodedEntryRoundTrip(t *testing.T) {
	const big = int64(5) << 30 // > 4 GiB
	mk := func(off, size, usize int64, method uint32, cbs uint32, enc bool, sizes ...int64) (*Entry, int64) {
		e := &Entry{Name: "x", Size: size, UncompressedSize: usize, CompressionMethod: method, CompressionBlockSize: cbs, encFresh: true}
		if enc {
			e.Flags = FlagEncrypted
		}
		if len(sizes) > 0 {
			blocksFromSizes(e, sizes...)
		}
		return e, off
	}
	type tc struct {
		name               string
		e                  *Entry
		off                int64
		off32, us32, sz32  bool
		wantLow            uint32
		wantEscape         bool
		wantBlockSizesInIx bool
	}
	var cases []tc
	add := func(name string, e *Entry, off int64, wantLow uint32, esc, bsz bool) {
		cases = append(cases, tc{name: name, e: e, off: off, off32: off <= math.MaxUint32, us32: e.UncompressedSize <= math.MaxUint32, sz32: e.Size <= math.MaxUint32, wantLow: wantLow, wantEscape: esc, wantBlockSizesInIx: bsz})
	}
	e, o := mk(1234, 100, 100, 0, 0, false)
	add("small uncompressed", e, o, 0, false, false)
	e, o = mk(big, 100, 100, 0, 0, false)
	add("offset > 4GiB", e, o, 0, false, false)
	e, o = mk(77, big, big, 0, 0, false)
	add("uncompressed size > 4GiB", e, o, 0, false, false)
	e, o = mk(big+3, 5000, 5000*3, 1, 15000, false, 5000)
	add("single block, offset > 4GiB", e, o, 0, false, false)
	e, o = mk(9, 9844+6821, 124968, 1, 65536, false, 9844, 6821)
	add("two blocks 64K (DefaultEngine-like)", e, o, 32, false, true)
	e, o = mk(9, 300, 0x3f<<11+10, 2, 0x3f<<11, false, 100, 200)
	add("block size 0x3f<<11 needs the escape", e, o, 0x3f, true, true)
	e, o = mk(9, 300, 200000, 1, 65537, false, 150, 150)
	add("non-multiple block size escape", e, o, 0x3f, true, true)
	e, o = mk(16, 48, 100, 1, 100, true, 40)
	add("encrypted single block stores its size", e, o, 0, false, true)
	e, o = mk(16, 64, 300000, 3, 65536, true, 20, 30, 9)
	add("encrypted multi block, aligned starts", e, o, 32, false, true)
	e, o = mk(16, 64, 300000, 3, 131072, false, 20, 30, 9)
	add("128 KiB blocks need the escape", e, o, 0x3f, true, true)
	sizes := make([]int64, 3)
	for i := range sizes {
		sizes[i] = 1000
	}
	e, o = mk(big*2, big+10, big*3, 63, 1<<30, false, sizes...)
	add("all 64-bit, method 63", e, o, 0x3f, true, true)

	for _, c := range cases {
		var b bytes.Buffer
		if err := encodeEntry(&b, c.e, c.off); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		raw := b.Bytes()
		v := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
		if (v>>31&1 == 1) != c.off32 || (v>>30&1 == 1) != c.us32 || (v>>29&1 == 1) != c.sz32 {
			t.Errorf("%s: width flags %08x", c.name, v)
		}
		if v&0x3f != c.wantLow || (c.wantEscape != (v&0x3f == 0x3f)) {
			t.Errorf("%s: block size bits %d, want %d", c.name, v&0x3f, c.wantLow)
		}
		if int(v>>6&0xffff) != len(c.e.Blocks) || v>>23&0x3f != c.e.CompressionMethod || (v>>22&1 == 1) != c.e.Encrypted() {
			t.Errorf("%s: fields %08x", c.name, v)
		}
		// expected length
		want := 4
		if c.wantEscape {
			want += 4
		}
		w := func(is32 bool) int {
			if is32 {
				return 4
			}
			return 8
		}
		want += w(c.off32) + w(c.us32)
		if c.e.Compressed() {
			want += w(c.sz32)
			if c.wantBlockSizesInIx {
				want += 4 * len(c.e.Blocks)
			}
		}
		if len(raw) != want {
			t.Errorf("%s: encoded %d bytes, want %d", c.name, len(raw), want)
		}
		r := &reader{b: raw}
		d := &Entry{Name: "x"}
		decodeEntry(r, d)
		if r.err != nil || r.p != len(raw) {
			t.Fatalf("%s: decode: %v (consumed %d of %d)", c.name, r.err, r.p, len(raw))
		}
		if d.Offset != c.off || d.Size != c.e.Size || d.UncompressedSize != c.e.UncompressedSize || d.CompressionMethod != c.e.CompressionMethod ||
			d.Flags != c.e.Flags || d.CompressionBlockSize != c.e.CompressionBlockSize || len(d.Blocks) != len(c.e.Blocks) {
			t.Errorf("%s: decoded %+v\n want %+v", c.name, d, c.e)
			continue
		}
		for i := range d.Blocks {
			if d.Blocks[i] != c.e.Blocks[i] {
				t.Errorf("%s: block %d %+v want %+v", c.name, i, d.Blocks[i], c.e.Blocks[i])
			}
		}
		var b2 bytes.Buffer
		if err := encodeEntry(&b2, d, d.Offset); err != nil || !bytes.Equal(b2.Bytes(), raw) {
			t.Errorf("%s: re-encode not identical: %v", c.name, err)
		}
	}
	// Unencodable entries are rejected.
	tooMany := &Entry{Name: "m", CompressionMethod: 1, Blocks: make([]Block, 0x10000), encFresh: true}
	if err := encodeEntry(&bytes.Buffer{}, tooMany, 0); err == nil {
		t.Error("65536 blocks accepted")
	}
	if err := encodeEntry(&bytes.Buffer{}, &Entry{Name: "m", CompressionMethod: 64, encFresh: true}, 0); err == nil {
		t.Error("method 64 accepted")
	}
}

func TestHashPath(t *testing.T) {
	// Stored in the real Quest pak (A2-Android_ASTC.pak, seed 0xabd9dff0).
	const seed = 0xabd9dff0
	if got := HashPath("A2/Config/DefaultGame.ini", seed); got != 0x12b4972246fccf69 {
		t.Errorf("DefaultGame.ini: %#x", got)
	}
	if got := HashPath("A2/Config/DefaultEngine.ini", seed); got != 0x51324ab73ac09cc9 {
		t.Errorf("DefaultEngine.ini: %#x", got)
	}
	if HashPath("A2/CONFIG/DefaultGame.INI", seed) != HashPath("a2/config/defaultgame.ini", seed) {
		t.Error("hash is not case-insensitive")
	}
	if HashPath("A2/Config/DefaultGame.ini", 0) == HashPath("A2/Config/DefaultGame.ini", seed) {
		t.Error("seed ignored")
	}
}

// TestV11RealOodleSynthetic needs PAK_EDITOR_OODLE_DLL.
func TestV11RealOodleSynthetic(t *testing.T) {
	dll := os.Getenv(OodleEnvVar)
	if dll == "" {
		t.Skip("set " + OodleEnvVar + " to run")
	}
	oo, err := LoadOodle(dll)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	src := writeV11(t, dir, oo)
	p, err := Open(src)
	if err != nil {
		t.Fatal(err)
	}
	p.Decompressor = oo
	got, err := p.ReadEntry("A2/Config/DefaultEngine.ini")
	if err != nil || string(got) != bigIni {
		t.Fatalf("read Oodle entry: %v", err)
	}
	repl := map[string][]byte{"A2/Config/DefaultEngine.ini": []byte(bigIni + "; oodle\r\n")}
	dst := filepath.Join(dir, "o.pak")
	res, err := RebuildWith(src, dst, repl, &RebuildOptions{Recompress: true, Codec: oo})
	if err != nil || len(res.Recompressed) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := Verify(dst, &VerifyOptions{Source: src, Expect: repl, Decompressor: oo}); err != nil {
		t.Fatal(err)
	}
}
