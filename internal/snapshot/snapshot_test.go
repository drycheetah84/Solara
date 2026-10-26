package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"solara/internal/docdb"
	"solara/internal/logging"
)

func init() { logging.DisableFiles = true }

type Item struct {
	docdb.AstraDbObject
	Name string
	N    int
}

func newItem() *Item { return &Item{} }

const secretMarker = "PLAINTEXT-MARKER-7f3a"

func openDB(t *testing.T, path string, pw *string) *docdb.Database {
	t.Helper()
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 1, Name: DBNameFromPath(path), Path: path, Password: pw})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func items(t *testing.T, db *docdb.Database) []string {
	t.Helper()
	col := docdb.GetCollection(db, true, newItem)
	var out []string
	for _, it := range col.FindAll() {
		out = append(out, fmt.Sprintf("%s:%d", it.Name, it.N))
	}
	sort.Strings(out)
	return out
}

func insert(db *docdb.Database, name string, n int) {
	docdb.GetCollection(db, true, newItem).Insert(&Item{Name: name, N: n})
}

type env struct {
	dir, dbPath, snaps, eos string
}

func newEnv(t *testing.T) env {
	d := t.TempDir()
	e := env{dir: d, dbPath: filepath.Join(d, "production"+docdb.FileSuffix), snaps: filepath.Join(d, "snapshots"), eos: filepath.Join(d, "eos")}
	if err := os.MkdirAll(e.eos, 0o700); err != nil {
		t.Fatal(err)
	}
	return e
}

func clock(start time.Time) func() time.Time {
	var mu sync.Mutex
	cur := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		cur = cur.Add(time.Minute)
		return cur
	}
}

func TestCreateWhileConcurrentWritersInsert(t *testing.T) {
	e := newEnv(t)
	db := openDB(t, e.dbPath, nil)
	defer db.Close()
	for i := 0; i < 50; i++ {
		insert(db, "seed", i)
	}
	var stop atomic.Bool
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; !stop.Load(); i++ {
				insert(db, fmt.Sprintf("w%d", w), i)
			}
		}(w)
	}
	var snaps []*Snapshot
	for i := 0; i < 3; i++ {
		s, err := Create(db, Options{Dir: e.snaps, Label: "concurrent"})
		if err != nil {
			t.Fatal(err)
		}
		snaps = append(snaps, s)
		time.Sleep(20 * time.Millisecond)
	}
	stop.Store(true)
	wg.Wait()

	for _, s := range snaps {
		if v := Verify(s.Path, nil); !v.OK {
			t.Fatalf("%s: %v", s.Name, v.Problems)
		}
		// The copy opens and holds exactly the documents the manifest counted.
		cp := filepath.Join(t.TempDir(), "production"+docdb.FileSuffix)
		copyFile(t, filepath.Join(s.Path, s.Manifest.DBFile), cp)
		copyFile(t, filepath.Join(s.Path, s.Manifest.DBFile+".meta"), cp+".meta")
		sdb := openDB(t, cp, nil)
		got := len(items(t, sdb))
		sdb.Close()
		want := -1
		for _, c := range s.Manifest.Collections {
			if c.Name == docdb.CollectionName("Item") {
				want = c.Documents
			}
		}
		if got != want || got < 50 {
			t.Fatalf("%s: copy has %d docs, manifest %d", s.Name, got, want)
		}
	}
	if !strings.HasSuffix(snaps[0].Name, "_concurrent") || snaps[1].Name == snaps[0].Name {
		t.Fatalf("names: %s %s", snaps[0].Name, snaps[1].Name)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManifestHashesAndLayout(t *testing.T) {
	e := newEnv(t)
	db := openDB(t, e.dbPath, nil)
	defer db.Close()
	insert(db, "a", 1)
	s, err := Create(db, Options{Dir: e.snaps, Label: "my label/../x", Reason: "test", Now: func() time.Time { return time.Date(2026, 10, 25, 1, 2, 3, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "20261025T010203Z_my-label-..-x" && !strings.HasPrefix(s.Name, "20261025T010203Z_") {
		t.Fatalf("name %s", s.Name)
	}
	if strings.ContainsAny(s.Name[len("20261025T010203Z_"):], `/\`) {
		t.Fatalf("label not sanitized: %s", s.Name)
	}
	m, err := ReadManifest(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Format != Format || m.CreatedAt != "2026-10-25T01:02:03Z" || m.DBName != "production" || m.Reason != "test" || m.SourceHost == "" || !m.MetaCaptured {
		t.Fatalf("manifest %+v", m)
	}
	if len(m.Files) != 2 {
		t.Fatalf("files %+v", m.Files)
	}
	for _, f := range m.Files {
		size, sum, err := hashFile(filepath.Join(s.Path, f.Name))
		if err != nil || size != f.Size || sum != f.SHA256 {
			t.Fatalf("%s: %d %s vs %+v (%v)", f.Name, size, sum, f, err)
		}
	}
	// No temp dir left behind.
	des, _ := os.ReadDir(e.snaps)
	if len(des) != 1 {
		t.Fatalf("snapshots dir: %v", des)
	}
}

func TestVerifyDetectsTamperAndMissingFile(t *testing.T) {
	e := newEnv(t)
	db := openDB(t, e.dbPath, nil)
	insert(db, "a", 1)
	s1, err := Create(db, Options{Dir: e.snaps, Now: clock(time.Now())})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Create(db, Options{Dir: e.snaps, Now: clock(time.Now().Add(time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	p := filepath.Join(s1.Path, s1.Manifest.DBFile)
	raw, _ := os.ReadFile(p)
	raw[len(raw)/2] ^= 0xFF
	_ = os.WriteFile(p, raw, 0o600)
	v := Verify(s1.Path, nil)
	if v.OK || !strings.Contains(strings.Join(v.Problems, ";"), "sha256 mismatch") {
		t.Fatalf("tamper not detected: %+v", v)
	}
	if c := CachedVerify(s1.Path); c == nil || c.OK {
		t.Fatalf("cache %+v", c)
	}

	_ = os.Remove(filepath.Join(s2.Path, s2.Manifest.DBFile+".meta"))
	v = Verify(s2.Path, nil)
	if v.OK || !strings.Contains(strings.Join(v.Problems, ";"), "missing") {
		t.Fatalf("missing file not detected: %+v", v)
	}
}

func TestEncryptedSnapshotStaysEncrypted(t *testing.T) {
	e := newEnv(t)
	pw := "correct horse"
	db := openDB(t, e.dbPath, &pw)
	insert(db, secretMarker, 1)
	s, err := Create(db, Options{Dir: e.snaps})
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !s.Manifest.Encrypted {
		t.Fatal("manifest not marked encrypted")
	}
	raw, _ := os.ReadFile(filepath.Join(s.Path, s.Manifest.DBFile))
	if bytes.Contains(raw, []byte(secretMarker)) {
		t.Fatal("snapshot contains plaintext document data")
	}
	if v := Verify(s.Path, &pw); !v.OK || !v.PasswordChecked || !v.PasswordOK {
		t.Fatalf("right password: %+v", v)
	}
	wrong := "nope"
	if v := Verify(s.Path, &wrong); v.OK || !v.PasswordChecked || v.PasswordOK {
		t.Fatalf("wrong password accepted: %+v", v)
	}
	if v := Verify(s.Path, nil); !v.OK || v.PasswordChecked || len(v.Notes) == 0 {
		t.Fatalf("no password: %+v", v)
	}
	// Restore with a wrong or missing password is refused and leaves the live DB alone.
	before, _ := os.ReadFile(e.dbPath)
	if _, err := Restore(s.Name, RestoreOptions{Dir: e.snaps, DBPath: e.dbPath, Password: &wrong}); err == nil {
		t.Fatal("restore with wrong password succeeded")
	}
	if _, err := Restore(s.Name, RestoreOptions{Dir: e.snaps, DBPath: e.dbPath}); err == nil {
		t.Fatal("restore without password succeeded")
	}
	after, _ := os.ReadFile(e.dbPath)
	if !bytes.Equal(before, after) {
		t.Fatal("live database changed by a refused restore")
	}
	// An unencrypted snapshot is refused when a password is configured.
	e2 := newEnv(t)
	db2 := openDB(t, e2.dbPath, nil)
	s2, _ := Create(db2, Options{Dir: e2.snaps})
	db2.Close()
	if v := Verify(s2.Path, &pw); v.OK {
		t.Fatal("unencrypted snapshot verified with a configured password")
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	e := newEnv(t)
	pw := "pw"
	db := openDB(t, e.dbPath, &pw)
	insert(db, "a", 1)
	insert(db, "b", 2)
	at := items(t, db)
	s, err := Create(db, Options{Dir: e.snaps, Label: "good", Now: clock(time.Now())})
	if err != nil {
		t.Fatal(err)
	}
	insert(db, "evil", 666)
	docdb.GetCollection(db, true, newItem).DeleteMany(func(i *Item) bool { return i.Name == "a" })
	db.Close()

	res, err := Restore(s.Name, RestoreOptions{Dir: e.snaps, DBPath: e.dbPath, Password: &pw, Now: clock(time.Now().Add(time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	db = openDB(t, e.dbPath, &pw)
	got := items(t, db)
	db.Close()
	if strings.Join(got, ",") != strings.Join(at, ",") {
		t.Fatalf("after restore %v, want %v", got, at)
	}
	// Safety snapshot holds the replaced state.
	if res.SafetySnapshot == "" {
		t.Fatal("no safety snapshot")
	}
	sp := filepath.Join(e.snaps, res.SafetySnapshot)
	m, err := ReadManifest(sp)
	if err != nil || m.Label != LabelPreRestore {
		t.Fatalf("safety manifest %+v %v", m, err)
	}
	if v := Verify(sp, &pw); !v.OK {
		t.Fatalf("safety snapshot: %v", v.Problems)
	}
	cp := filepath.Join(t.TempDir(), "production"+docdb.FileSuffix)
	copyFile(t, filepath.Join(sp, m.DBFile), cp)
	sdb := openDB(t, cp, &pw)
	if g := strings.Join(items(t, sdb), ","); g != "b:2,evil:666" {
		t.Fatalf("safety snapshot content %s", g)
	}
	sdb.Close()
	// Replaced/staging files are cleaned up.
	des, _ := os.ReadDir(e.dir)
	for _, d := range des {
		if strings.Contains(d.Name(), ".replaced-") || strings.Contains(d.Name(), ".restore-tmp") {
			t.Fatalf("leftover %s", d.Name())
		}
	}
	if len(res.Replaced) != 2 {
		t.Fatalf("replaced %v", res.Replaced)
	}
}

func TestRestoreRefusedWhileDatabaseOpen(t *testing.T) {
	e := newEnv(t)
	db := openDB(t, e.dbPath, nil)
	defer db.Close()
	insert(db, "a", 1)
	s, err := Create(db, Options{Dir: e.snaps})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Restore(s.Name, RestoreOptions{Dir: e.snaps, DBPath: e.dbPath})
	if !errors.Is(err, ErrInUse) {
		t.Fatalf("got %v, want ErrInUse", err)
	}
	if _, err := CreateOffline(e.dbPath, Options{Dir: e.snaps}); !errors.Is(err, docdb.ErrDatabaseInUse) {
		t.Fatalf("offline create while open: %v", err)
	}
}

func TestEOSFilesIncludedAndExcluded(t *testing.T) {
	e := newEnv(t)
	sess := filepath.Join(e.eos, "sessions.json")
	_ = os.WriteFile(sess, []byte(`[{"id":"s1"}]`), 0o644)
	db := openDB(t, e.dbPath, nil)
	with, err := Create(db, Options{Dir: e.snaps, EOSDir: e.eos, Label: "with", Now: clock(time.Now())})
	if err != nil {
		t.Fatal(err)
	}
	without, err := Create(db, Options{Dir: e.snaps, Label: "without", Now: clock(time.Now().Add(time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	names := func(m *Manifest) string {
		var n []string
		for _, f := range m.Files {
			n = append(n, f.Name)
		}
		return strings.Join(n, ",")
	}
	if got := names(with.Manifest); got != "production.solara.db,production.solara.db.meta,sessions.json" || !with.Manifest.EOSCaptured {
		t.Fatalf("with: %s", got)
	}
	if got := names(without.Manifest); strings.Contains(got, "sessions") || without.Manifest.EOSCaptured {
		t.Fatalf("without: %s", got)
	}

	// Restore without --include-eos leaves EOS files alone; with it, the
	// snapshot's sessions.json comes back and a lobbies.json created later
	// (absent from the snapshot) is removed.
	_ = os.WriteFile(sess, []byte(`[{"id":"s2"}]`), 0o644)
	lob := filepath.Join(e.eos, "lobbies.json")
	_ = os.WriteFile(lob, []byte(`[]`), 0o644)
	if _, err := Restore(with.Name, RestoreOptions{Dir: e.snaps, DBPath: e.dbPath, EOSDir: e.eos}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(sess); string(raw) != `[{"id":"s2"}]` {
		t.Fatalf("EOS touched without --include-eos: %s", raw)
	}
	res, err := Restore(with.Name, RestoreOptions{Dir: e.snaps, DBPath: e.dbPath, EOSDir: e.eos, IncludeEOS: true})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(sess); string(raw) != `[{"id":"s1"}]` {
		t.Fatalf("sessions.json not restored: %s", raw)
	}
	if _, err := os.Stat(lob); !errors.Is(err, os.ErrNotExist) || len(res.Removed) != 1 {
		t.Fatalf("lobbies.json should be removed: %v %v", err, res.Removed)
	}
	if _, err := Restore(without.Name, RestoreOptions{Dir: e.snaps, DBPath: e.dbPath, EOSDir: e.eos, IncludeEOS: true}); err == nil {
		t.Fatal("--include-eos accepted for a snapshot without EOS files")
	}
}

func TestPruneRetention(t *testing.T) {
	e := newEnv(t)
	db := openDB(t, e.dbPath, nil)
	defer db.Close()
	now := clock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	var all []*Snapshot
	labels := []string{"auto", "keep", "pre-restore", "auto", "auto", "auto", "auto", "auto"}
	for _, l := range labels { // oldest first
		s, err := Create(db, Options{Dir: e.snaps, Label: l, Now: now})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, s)
	}
	// Only the oldest "auto" is verified: it is the newest verified one.
	if v := Verify(all[0].Path, nil); !v.OK {
		t.Fatal(v.Problems)
	}
	deleted, err := Prune(e.snaps, 2, false)
	if err != nil {
		t.Fatal(err)
	}
	// Kept: 2 newest (7,6), keep (1), pre-restore (2), newest verified (0).
	want := []string{all[5].Name, all[4].Name, all[3].Name}
	if strings.Join(deleted, ",") != strings.Join(want, ",") {
		t.Fatalf("deleted %v want %v", deleted, want)
	}
	deleted, err = Prune(e.snaps, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	// Forced: keep/pre-restore go, newest verified still stays.
	want = []string{all[6].Name, all[2].Name, all[1].Name}
	if strings.Join(deleted, ",") != strings.Join(want, ",") {
		t.Fatalf("forced deleted %v want %v", deleted, want)
	}
	es, _ := List(e.snaps, false, nil)
	if len(es) != 2 || es[0].Name != all[7].Name || es[1].Name != all[0].Name {
		t.Fatalf("remaining %v", es)
	}
	if es[1].Verify == nil || !es[1].Verify.OK || es[0].Size == 0 {
		t.Fatalf("list entry %+v", es[1])
	}
	if p, err := Resolve(e.snaps, "latest"); err != nil || filepath.Base(p) != all[7].Name {
		t.Fatalf("latest %s %v", p, err)
	}
}

func TestSchedulerFiresAndStops(t *testing.T) {
	e := newEnv(t)
	db := openDB(t, e.dbPath, nil)
	insert(db, "a", 1)
	s := StartScheduler(db, SchedulerConfig{Dir: e.snaps, Interval: 40 * time.Millisecond, Keep: 3, OnStart: true})
	deadline := time.Now().Add(10 * time.Second)
	for len(s.Taken()) < 5 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.Stop()
	s.Stop() // idempotent
	taken := s.Taken()
	if len(taken) < 5 {
		t.Fatalf("only %d snapshots", len(taken))
	}
	if !strings.HasSuffix(taken[0], "_startup") || !strings.Contains(taken[1], "_auto") {
		t.Fatalf("labels %v", taken)
	}
	n := len(s.Taken())
	time.Sleep(150 * time.Millisecond)
	if len(s.Taken()) != n {
		t.Fatal("scheduler kept running after Stop")
	}
	es, _ := List(e.snaps, false, nil)
	// Pruned to 3, plus at most the protected newest-verified one.
	if len(es) < 3 || len(es) > 4 {
		t.Fatalf("%d snapshots after pruning", len(es))
	}
	for _, x := range es {
		if x.Verify == nil || !x.Verify.OK {
			t.Fatalf("%s not verified", x.Name)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Disabled interval + no startup snapshot: nothing is taken, Stop returns.
	e2 := newEnv(t)
	db2 := openDB(t, e2.dbPath, nil)
	s2 := StartScheduler(db2, SchedulerConfig{Dir: e2.snaps})
	time.Sleep(50 * time.Millisecond)
	s2.Stop()
	if len(s2.Taken()) != 0 {
		t.Fatal("disabled scheduler took a snapshot")
	}
	db2.Close()
}

func TestSchedulerLogsFailuresWithoutCrashing(t *testing.T) {
	e := newEnv(t)
	db := openDB(t, e.dbPath, nil)
	defer db.Close()
	// Snapshots dir is a regular file: every snapshot fails.
	bad := filepath.Join(e.dir, "notadir")
	_ = os.WriteFile(bad, nil, 0o600)
	s := StartScheduler(db, SchedulerConfig{Dir: bad, Interval: 20 * time.Millisecond, OnStart: true})
	time.Sleep(100 * time.Millisecond)
	s.Stop()
	if len(s.Taken()) != 0 {
		t.Fatal("snapshot unexpectedly succeeded")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("SOLARA_SNAPSHOT_DIR", "/x/snaps")
	t.Setenv("SOLARA_SNAPSHOT_INTERVAL_HOURS", "0")
	t.Setenv("SOLARA_SNAPSHOT_KEEP", "5")
	t.Setenv("SOLARA_SNAPSHOT_ON_START", "0")
	c := ConfigFromEnv(nil)
	if c.Dir != "/x/snaps" || c.Interval != 0 || c.Keep != 5 || c.OnStart {
		t.Fatalf("%+v", c)
	}
	t.Setenv("SOLARA_SNAPSHOT_INTERVAL_HOURS", "bogus")
	t.Setenv("SOLARA_SNAPSHOT_KEEP", "")
	t.Setenv("SOLARA_SNAPSHOT_ON_START", "")
	c = ConfigFromEnv(nil)
	if c.Interval != 6*time.Hour || c.Keep != 28 || !c.OnStart {
		t.Fatalf("%+v", c)
	}
}
