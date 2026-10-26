package pak

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const engineIni = "[/Script/Engine.Engine]\r\n+ActiveGameNameRedirects=(OldGameName=\"A\",NewGameName=\"B\")\r\nbSmoothFrameRate=True\r\n"
const gameIni = "[/Script/EngineSettings.GeneralProjectSettings]\r\nProjectID=1234\r\n"

func testFiles() []NewFile {
	return []NewFile{
		{Name: "A2/A2.uproject", Data: []byte("{\"FileVersion\": 3}")},
		{Name: "A2/Config/DefaultEngine.ini", Data: []byte(engineIni)},
		{Name: "A2/Config/DefaultGame.ini", Data: []byte(gameIni)},
		{Name: "A2/Content/Big.uasset", Data: bytes.Repeat([]byte{1, 2, 3, 4, 5}, 5000)},
		{Name: "A2/Content/Packed.uasset", Data: []byte("compressed-bytes"), CompressionMethod: 1, CompressionBlockSize: 65536},
		{Name: "A2/Content/Secret.uasset", Data: []byte("encrypted-bytes!!"), Encrypted: true},
		{Name: "A2/Content/Empty.bin", Data: nil},
	}
}

// writeTestPak builds a pak whose physical order differs from index order
// and which has gap bytes, like real paks may.
func writeTestPak(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "Test-Windows.pak")
	err := WriteNew(path, "../../../", testFiles(), &WriteOptions{
		PhysicalOrder:    []int{0, 3, 2, 6, 1, 5, 4},
		GapBefore:        map[int]int{2: 7, 4: 3},
		GapBeforeIndex:   5,
		CompressionNames: []string{"Zlib"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func fileBytes(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParse(t *testing.T) {
	path := writeTestPak(t, t.TempDir())
	p, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if p.Footer.Version != 8 || p.MountPoint != "../../../" || len(p.Entries) != 7 {
		t.Fatalf("got version %d mount %q entries %d", p.Footer.Version, p.MountPoint, len(p.Entries))
	}
	if p.Footer.CompressionNames[0] != "Zlib" {
		t.Fatalf("compression names %v", p.Footer.CompressionNames)
	}
	e, ok := p.Entry("A2/Content/Packed.uasset")
	if !ok || !e.Compressed() || e.Editable() {
		t.Fatal("compressed entry not recognised")
	}
	data, err := p.ReadEntry("A2/Config/DefaultEngine.ini")
	if err != nil || string(data) != engineIni {
		t.Fatalf("ReadEntry: %v %q", err, data)
	}
	if _, err := p.ReadEntry("A2/Content/Packed.uasset"); err == nil {
		t.Fatal("expected error reading compressed entry")
	}
	if _, err := Verify(path, nil); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedVersion(t *testing.T) {
	dir := t.TempDir()
	path := writeTestPak(t, dir)
	b := fileBytes(t, path)
	b[len(b)-221+21] = 12 // version field (8 and 11 are supported)
	bad := filepath.Join(dir, "v12.pak")
	os.WriteFile(bad, b, 0o644)
	_, err := Open(bad)
	var uv *UnsupportedVersionError
	if !errors.As(err, &uv) || uv.Version != 12 {
		t.Fatalf("expected unsupported version 12, got %v", err)
	}
	f, _ := os.Open(bad)
	defer f.Close()
	ft, err := ReadFooter(f, int64(len(b)))
	if err != nil || ft.Version != 12 {
		t.Fatalf("ReadFooter: %v", err)
	}
	notPak := filepath.Join(dir, "x.pak")
	os.WriteFile(notPak, bytes.Repeat([]byte{7}, 500), 0o644)
	if _, err := Open(notPak); !errors.Is(err, ErrNotPak) {
		t.Fatalf("expected ErrNotPak, got %v", err)
	}
}

func TestRebuildIdentity(t *testing.T) {
	dir := t.TempDir()
	src := writeTestPak(t, dir)
	dst := filepath.Join(dir, "out.pak")
	res, err := Rebuild(src, dst, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileBytes(t, src), fileBytes(t, dst)) {
		t.Fatal("rebuild without replacements is not byte-identical")
	}
	if len(res.Changed) != 0 {
		t.Fatalf("changed = %v", res.Changed)
	}
}

func TestRebuildWithEdit(t *testing.T) {
	dir := t.TempDir()
	src := writeTestPak(t, dir)
	dst := filepath.Join(dir, "out.pak")
	newIni := []byte(engineIni + "; added by test\r\n" + strings.Repeat("x", 1000) + "\r\n")
	repl := map[string][]byte{"A2/Config/DefaultEngine.ini": newIni}
	var lastDone, lastTotal int64
	res, err := Rebuild(src, dst, repl, func(d, t int64) { lastDone, lastTotal = d, t })
	if err != nil {
		t.Fatal(err)
	}
	if lastDone != lastTotal || lastDone != res.Size {
		t.Fatalf("progress %d/%d size %d", lastDone, lastTotal, res.Size)
	}
	if len(res.Changed) != 1 || res.Changed[0] != "A2/Config/DefaultEngine.ini" {
		t.Fatalf("changed = %v", res.Changed)
	}
	if _, err := Verify(dst, &VerifyOptions{Source: src, Expect: repl}); err != nil {
		t.Fatal(err)
	}
	p, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.ReadEntry("A2/Config/DefaultEngine.ini")
	if err != nil || !bytes.Equal(got, newIni) {
		t.Fatalf("edited entry: %v", err)
	}
	// physical order and names preserved
	sp, _ := Open(src)
	po, _ := p.physicalOrder()
	so, _ := sp.physicalOrder()
	for i := range po {
		if po[i].IndexPos != so[i].IndexPos || p.Entries[i].Name != sp.Entries[i].Name {
			t.Fatal("order changed")
		}
	}
	// Verify must fail without Expect (content differs from source).
	if _, err := Verify(dst, &VerifyOptions{Source: src}); err == nil {
		t.Fatal("verify against source without expectations should fail")
	}
	// Rebuilding back to the original content restores identical bytes.
	back := filepath.Join(dir, "back.pak")
	if _, err := Rebuild(dst, back, map[string][]byte{"A2/Config/DefaultEngine.ini": []byte(engineIni)}, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileBytes(t, src), fileBytes(t, back)) {
		t.Fatal("reverting the edit is not byte-identical to the source")
	}
}

func TestRebuildRefusesCompressedAndEncrypted(t *testing.T) {
	dir := t.TempDir()
	src := writeTestPak(t, dir)
	for _, name := range []string{"A2/Content/Packed.uasset", "A2/Content/Secret.uasset", "missing"} {
		_, err := Rebuild(src, filepath.Join(dir, "o.pak"), map[string][]byte{name: []byte("x")}, nil)
		if err == nil {
			t.Fatalf("expected refusal for %s", name)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "o.pak")); statErr == nil {
			t.Fatal("output left behind after refusal")
		}
	}
}

func TestVerifyDetectsCorruption(t *testing.T) {
	dir := t.TempDir()
	src := writeTestPak(t, dir)
	orig := fileBytes(t, src)
	p, _ := Open(src)
	big, _ := p.Entry("A2/Content/Big.uasset")
	eng, _ := p.Entry("A2/Config/DefaultEngine.ini")

	cases := map[string]func(b []byte){
		"data byte":   func(b []byte) { b[big.Offset+big.HeaderSize()+100] ^= 0xFF },
		"inline hash": func(b []byte) { b[eng.Offset+28] ^= 0x01 },
		"inline size": func(b []byte) { b[eng.Offset+8] ^= 0x01 },
		"index byte":  func(b []byte) { b[p.Footer.IndexOffset+20] ^= 0x01 },
		"index hash":  func(b []byte) { b[len(b)-221+41] ^= 0x01 },
		"magic":       func(b []byte) { b[len(b)-221+17] ^= 0x01 },
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
	// Gap bytes are not covered by hashes but are by source comparison of
	// untouched entries' regions; a flipped entry byte must fail the source check.
	b := append([]byte(nil), orig...)
	b[big.Offset+big.HeaderSize()+5] ^= 0xFF
	// fix the hashes so only the source comparison can catch it
	data := b[big.Offset+big.HeaderSize() : big.Offset+big.HeaderSize()+big.Size]
	h := sha1.Sum(data)
	copy(b[big.Offset+28:], h[:])
	bad := filepath.Join(dir, "bad2.pak")
	os.WriteFile(bad, b, 0o644)
	if _, err := Verify(bad, &VerifyOptions{Source: src}); err == nil {
		t.Error("source comparison did not detect a modified entry")
	}
}

func TestIniRoundTrip(t *testing.T) {
	cases := map[string][]byte{
		"crlf":        []byte("[A]\r\nK=V\r\n"),
		"lf":          []byte("[A]\nK=V\n"),
		"no-trailing": []byte("[A]\r\nK=V"),
		"bom":         append([]byte{0xEF, 0xBB, 0xBF}, "[A]\r\nK=V\r\n"...),
		"utf16le":     {0xFF, 0xFE, '[', 0, 'A', 0, ']', 0, '\r', 0, '\n', 0, 'K', 0, '=', 0, 'V', 0},
		"empty":       {},
	}
	for name, raw := range cases {
		d, err := DecodeIni(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(d.Text, "\r") {
			t.Fatalf("%s: text not LF-normalised", name)
		}
		if got := d.Encode(d.Text); !bytes.Equal(got, raw) {
			t.Fatalf("%s: round trip %q != %q", name, got, raw)
		}
	}
	d, _ := DecodeIni([]byte("[A]\r\nK=V\r\n"))
	if d.LineEnding != CRLF || d.BOM {
		t.Fatal("wrong detection")
	}
	// Edited LF text gets CRLF back and the trailing newline restored.
	if got := string(d.Encode("[A]\nK=V\n; new")); got != "[A]\r\nK=V\r\n; new\r\n" {
		t.Fatalf("encode: %q", got)
	}
	if _, err := DecodeIni([]byte{0x80, 0x81}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
}

func TestValidateIni(t *testing.T) {
	text := strings.Join([]string{
		"; comment",
		"Orphan=1",                // 2 outside section
		"[Good]",                  // 3
		"+Arr=(A=1,B=\"x)\")",     // 4 ok: paren inside quotes
		"Key=",                    // 5 ok empty value
		"-Arr=(A=1",               // 6 unbalanced parens
		"Str=\"abc",               // 7 unbalanced quote
		"[Broken",                 // 8 missing ]
		"NoEquals",                // 9
		"Bad=\x00",                // 10 NUL
		"# hash comment",          // 11
		"!Clear=ClearArray",       // 12
		".Dup=1",                  // 13
		"   Indented = value ",    // 14
		"[/Script/Engine.Engine]", // 15
		"Ctl=a\x01b",              // 16
	}, "\n")
	issues, sections := ValidateIni(text)
	want := map[int]string{2: SevWarning, 6: SevWarning, 7: SevWarning, 8: SevWarning, 9: SevWarning, 10: SevError, 16: SevError}
	got := map[int]string{}
	for _, i := range issues {
		got[i.Line] = i.Severity
	}
	for line, sev := range want {
		if got[line] != sev {
			t.Errorf("line %d: want %s, got %q (all: %+v)", line, sev, got[line], issues)
		}
	}
	for line := range got {
		if _, ok := want[line]; !ok {
			t.Errorf("unexpected issue on line %d: %+v", line, issues)
		}
	}
	if len(sections) != 3 || sections[0] != (Section{"Good", 3}) || sections[2] != (Section{"/Script/Engine.Engine", 15}) {
		t.Errorf("sections = %+v", sections)
	}
	if !HasErrors(issues) {
		t.Error("HasErrors false")
	}
	if iss, _ := ValidateIni(engineIni); len(iss) != 0 {
		t.Errorf("clean ini has issues: %+v", iss)
	}
}

func clock(start time.Time) func() time.Time {
	n := 0
	return func() time.Time {
		n++
		return start.Add(time.Duration(n) * time.Second)
	}
}

func TestSaveBackupsAndRestore(t *testing.T) {
	dir := t.TempDir()
	pak := writeTestPak(t, dir)
	orig := fileBytes(t, pak)
	opts := &SaveOptions{Now: clock(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)), KeepBackups: 3}
	phases := map[string]bool{}
	opts.Progress = func(ph string, f float64) {
		phases[ph] = true
		if f < 0 || f > 1 {
			t.Errorf("fraction %v", f)
		}
	}
	const name = "A2/Config/DefaultGame.ini"
	for i := 0; i < 5; i++ {
		content := []byte(gameIni + strings.Repeat("; edit\r\n", i+1))
		res, err := Save(pak, map[string][]byte{name: content}, opts)
		if err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
		if len(res.Changed) != 1 || res.NewSize != int64(len(fileBytes(t, pak))) {
			t.Fatalf("result %+v", res)
		}
		p, err := Open(pak)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := p.ReadEntry(name); !bytes.Equal(got, content) {
			t.Fatal("saved content mismatch")
		}
	}
	for _, ph := range []string{PhaseWriting, PhaseVerifying, PhaseBackingUp, PhaseSwapping} {
		if !phases[ph] {
			t.Errorf("phase %s not reported", ph)
		}
	}
	if !bytes.Equal(fileBytes(t, OriginalPath(pak)), orig) {
		t.Fatal(".original is not the pristine pak")
	}
	bs, err := ListBackups(pak)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 4 || bs[0].Kind != "original" {
		t.Fatalf("backups = %+v", bs)
	}
	for i := 2; i < len(bs); i++ {
		if bs[i-1].Name < bs[i].Name {
			t.Fatal("timestamped backups not newest first")
		}
	}
	if _, err := os.Stat(TempPath(pak)); err == nil {
		t.Fatal("temp file left behind")
	}
	// Restore the .original; the current pak is backed up first.
	before := fileBytes(t, pak)
	res, err := Restore(pak, OriginalPath(pak), opts)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fileBytes(t, pak), orig) {
		t.Fatal("restore did not bring back the original")
	}
	if !bytes.Equal(fileBytes(t, res.Backup), before) {
		t.Fatal("restore did not back up the current pak")
	}
	if _, err := os.Stat(OriginalPath(pak)); err != nil {
		t.Fatal(".original deleted")
	}
	// Restoring an arbitrary file is refused.
	other := filepath.Join(dir, "other.pak")
	os.WriteFile(other, orig, 0o644)
	if _, err := Restore(pak, other, opts); err == nil {
		t.Fatal("restore from non-backup accepted")
	}
}

func TestSaveFailureLeavesOriginal(t *testing.T) {
	dir := t.TempDir()
	pak := writeTestPak(t, dir)
	orig := fileBytes(t, pak)
	repl := map[string][]byte{"A2/Config/DefaultGame.ini": []byte("[X]\r\nY=1\r\n")}

	// Failure injected after the temp file is written.
	opts := &SaveOptions{afterWrite: func() error { return errors.New("injected failure") }}
	if _, err := Save(pak, repl, opts); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("expected injected failure, got %v", err)
	}
	// Corrupt the temp file before verification: verify must reject it.
	opts = &SaveOptions{afterWrite: func() error {
		f, err := os.OpenFile(TempPath(pak), os.O_RDWR, 0)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteAt([]byte{0xFF, 0xFF}, 60)
		return err
	}}
	if _, err := Save(pak, repl, opts); err == nil || !IsVerifyError(errors.Unwrap(err)) {
		t.Fatalf("expected verification failure, got %v", err)
	}
	if !bytes.Equal(fileBytes(t, pak), orig) {
		t.Fatal("original pak modified after failed save")
	}
	if _, err := os.Stat(TempPath(pak)); err == nil {
		t.Fatal("temp file left behind")
	}
	if bs, _ := ListBackups(pak); len(bs) != 0 {
		t.Fatalf("backups created by failed saves: %+v", bs)
	}
	// Refusals for compressed entries happen before anything is written.
	if _, err := Save(pak, map[string][]byte{"A2/Content/Packed.uasset": []byte("x")}, nil); err == nil {
		t.Fatal("compressed replacement accepted")
	}
}

func TestBackupKind(t *testing.T) {
	pak := filepath.Join(t.TempDir(), "A2-Windows.pak")
	cases := map[string]string{
		"A2-Windows.pak.original":              "original",
		"A2-Windows.pak.bak-20260102-030405":   "timestamped",
		"A2-Windows.pak.bak-20260102-030405-2": "timestamped",
		"A2-Windows.pak.bak-2026":              "",
		"A2-Windows.pak.solara-tmp":            "",
		"A2-Windows.pak.original.partial":      "",
		"Other.pak.original":                   "",
		"A2-Windows.pak.bak-20260102-030405-x": "",
	}
	for name, want := range cases {
		if got := backupKind(pak, filepath.Join(filepath.Dir(pak), name)); got != want {
			t.Errorf("%s: got %q want %q", name, got, want)
		}
	}
}

func TestPruneKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	pak := writeTestPak(t, dir)
	os.WriteFile(OriginalPath(pak), []byte("orig"), 0o644)
	for i := 0; i < 15; i++ {
		p := timestampedPath(pak, time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC))
		os.WriteFile(p, []byte("b"), 0o644)
	}
	pruneBackups(pak, 10)
	bs, _ := ListBackups(pak)
	if len(bs) != 11 || bs[0].Kind != "original" || !strings.HasSuffix(bs[1].Name, "000014") {
		t.Fatalf("after prune: %d backups, first %+v", len(bs), bs[:2])
	}
}
