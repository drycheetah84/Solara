package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"solara/internal/docdb"
)

// VerifyResult is the outcome of Verify (also cached in verify.json).
type VerifyResult struct {
	Snapshot   string   `json:"snapshot"`
	VerifiedAt string   `json:"verified_at"`
	OK         bool     `json:"ok"`
	Problems   []string `json:"problems"`
	Notes      []string `json:"notes"`
	// PasswordChecked: a password was supplied and tested against the
	// encrypted store; PasswordOK: it decrypts the "__solara" check value.
	PasswordChecked bool `json:"password_checked"`
	PasswordOK      bool `json:"password_ok"`
}

// Verify recomputes every file hash against the manifest, opens the database
// copy read-only and runs bbolt's structural check, compares the document
// counts with the manifest and, when password is non-nil, confirms it
// decrypts an encrypted store. The result is cached in verify.json.
func Verify(snapshotDir string, password *string) *VerifyResult {
	r := &VerifyResult{Snapshot: filepath.Base(snapshotDir), VerifiedAt: time.Now().UTC().Format(time.RFC3339), Problems: []string{}, Notes: []string{}}
	defer func() {
		r.OK = len(r.Problems) == 0
		if raw, err := json.MarshalIndent(r, "", "  "); err == nil {
			_ = os.WriteFile(filepath.Join(snapshotDir, VerifyCacheName), append(raw, '\n'), 0o600)
		}
	}()
	m, err := ReadManifest(snapshotDir)
	if err != nil {
		r.Problems = append(r.Problems, err.Error())
		return r
	}
	hasDB := false
	for _, f := range m.Files {
		if f.Name != filepath.Base(f.Name) || f.Name == ManifestName {
			r.Problems = append(r.Problems, fmt.Sprintf("%s: invalid file name in manifest", f.Name))
			continue
		}
		if f.Name == m.DBFile {
			hasDB = true
		}
		size, sum, err := hashFile(filepath.Join(snapshotDir, f.Name))
		switch {
		case errors.Is(err, os.ErrNotExist):
			r.Problems = append(r.Problems, fmt.Sprintf("%s: missing", f.Name))
		case err != nil:
			r.Problems = append(r.Problems, fmt.Sprintf("%s: %v", f.Name, err))
		case size != f.Size:
			r.Problems = append(r.Problems, fmt.Sprintf("%s: size %d, manifest says %d", f.Name, size, f.Size))
		case sum != f.SHA256:
			r.Problems = append(r.Problems, fmt.Sprintf("%s: sha256 mismatch (file was modified)", f.Name))
		}
	}
	if !hasDB {
		r.Problems = append(r.Problems, fmt.Sprintf("manifest does not list the database file %q", m.DBFile))
		return r
	}
	if len(r.Problems) > 0 {
		return r // do not open a file that failed its hash check
	}
	rep, err := docdb.InspectFile(filepath.Join(snapshotDir, m.DBFile), password, 2*time.Second)
	if err != nil {
		r.Problems = append(r.Problems, fmt.Sprintf("open database copy: %v", err))
		return r
	}
	for _, c := range rep.CheckErrors {
		r.Problems = append(r.Problems, "bbolt check: "+c)
	}
	if rep.Encrypted != m.Encrypted {
		r.Problems = append(r.Problems, fmt.Sprintf("encryption flag mismatch (file encrypted=%v, manifest=%v)", rep.Encrypted, m.Encrypted))
	}
	if d := diffCounts(m.Collections, rep.Collections); d != "" {
		r.Problems = append(r.Problems, "collection counts differ from manifest: "+d)
	}
	switch {
	case rep.Encrypted && password == nil:
		r.Notes = append(r.Notes, "store is encrypted; no password supplied, decryption not checked")
	case rep.Encrypted:
		r.PasswordChecked, r.PasswordOK = rep.PasswordChecked, rep.PasswordOK
		if !rep.PasswordOK {
			r.Problems = append(r.Problems, "the configured password does not decrypt this snapshot")
		}
	case password != nil:
		r.Problems = append(r.Problems, "snapshot store is NOT encrypted but a password is configured for this database")
	}
	return r
}

func diffCounts(want, got []docdb.CollectionCount) string {
	w := map[string]int{}
	for _, c := range want {
		w[c.Name] = c.Documents
	}
	var out []string
	for _, c := range got {
		if n, ok := w[c.Name]; !ok {
			out = append(out, c.Name+" not in manifest")
		} else if n != c.Documents {
			out = append(out, fmt.Sprintf("%s %d vs %d", c.Name, c.Documents, n))
		}
		delete(w, c.Name)
	}
	for n := range w {
		out = append(out, n+" missing")
	}
	sort.Strings(out)
	return strings.Join(out, "; ")
}

// CachedVerify returns the last cached verification, or nil.
func CachedVerify(snapshotDir string) *VerifyResult {
	raw, err := os.ReadFile(filepath.Join(snapshotDir, VerifyCacheName))
	if err != nil {
		return nil
	}
	var r VerifyResult
	if json.Unmarshal(raw, &r) != nil {
		return nil
	}
	return &r
}

// Entry is one row of List.
type Entry struct {
	Snapshot
	Size    int64         // bytes of captured files
	Created time.Time     // manifest created_at
	Verify  *VerifyResult // cached (or fresh when List was asked to verify)
	Err     error         // manifest unreadable
}

// List returns the snapshots in dir, newest first. Temporary (in-progress)
// directories are skipped. When verify is true every snapshot is verified
// now with passwordFor(manifest.DBName) (nil function = no password).
func List(dir string, verify bool, passwordFor func(dbName string) *string) ([]Entry, error) {
	if dir == "" {
		dir = DefaultDir()
	}
	des, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, de := range des {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, de.Name())
		e := Entry{Snapshot: Snapshot{Name: de.Name(), Path: p}}
		m, err := ReadManifest(p)
		if err != nil {
			e.Err = err
			if info, err2 := de.Info(); err2 == nil {
				e.Created = info.ModTime().UTC()
			}
		} else {
			e.Manifest, e.Created = m, m.Created()
			for _, f := range m.Files {
				e.Size += f.Size
			}
		}
		if verify {
			var pw *string
			if passwordFor != nil && m != nil {
				pw = passwordFor(m.DBName)
			}
			e.Verify = Verify(p, pw)
		} else {
			e.Verify = CachedVerify(p)
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.After(out[j].Created)
		}
		return out[i].Name > out[j].Name
	})
	return out, nil
}

// Resolve maps "latest" or a snapshot name to its directory.
func Resolve(dir, name string) (string, error) {
	if dir == "" {
		dir = DefaultDir()
	}
	if name == "latest" {
		es, err := List(dir, false, nil)
		if err != nil {
			return "", err
		}
		for _, e := range es {
			if e.Err == nil {
				return e.Path, nil
			}
		}
		return "", fmt.Errorf("no snapshots in %s", dir)
	}
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("invalid snapshot name %q", name)
	}
	p := filepath.Join(dir, name)
	if _, err := os.Stat(filepath.Join(p, ManifestName)); err != nil {
		return "", fmt.Errorf("snapshot %q not found in %s", name, dir)
	}
	return p, nil
}

// Prune keeps the newest keep snapshots and deletes the rest, except that it
// never deletes the newest verified snapshot (cached verify.json ok), and
// never deletes snapshots labelled "keep" or "pre-restore" unless force is
// set. Unreadable (manifest-less) directories are left alone. Returns the
// deleted snapshot names.
func Prune(dir string, keep int, force bool) ([]string, error) {
	if keep < 0 {
		return nil, fmt.Errorf("keep must be >= 0")
	}
	es, err := List(dir, false, nil)
	if err != nil {
		return nil, err
	}
	newestVerified := ""
	for _, e := range es {
		if e.Err == nil && e.Verify != nil && e.Verify.OK {
			newestVerified = e.Name
			break
		}
	}
	var deleted []string
	kept := 0
	for _, e := range es {
		if e.Err != nil {
			continue
		}
		if kept < keep {
			kept++
			continue
		}
		if e.Name == newestVerified {
			continue
		}
		if !force && (e.Manifest.Label == LabelKeep || e.Manifest.Label == LabelPreRestore) {
			continue
		}
		if err := os.RemoveAll(e.Path); err != nil {
			return deleted, err
		}
		deleted = append(deleted, e.Name)
	}
	return deleted, nil
}
