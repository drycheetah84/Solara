package snapshot

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"solara/internal/docdb"
)

// RestoreOptions controls Restore.
type RestoreOptions struct {
	Dir        string  // snapshots directory (DefaultDir() when empty)
	DBPath     string  // live "<name>.solara.db" to replace
	EOSDir     string  // live directory of sessions.json / lobbies.json
	IncludeEOS bool    // also restore the EOS files
	Password   *string // the database password (nil when unencrypted)
	Now        func() time.Time
}

// RestoreResult reports what Restore did.
type RestoreResult struct {
	Snapshot       string
	SafetySnapshot string // "" when there was no live database to save
	Replaced       []string
	Removed        []string // live files absent from the snapshot (EOS only)
	Verify         *VerifyResult
}

// ErrInUse is returned when the live database is open by a running server.
var ErrInUse = errors.New("the live database is in use: stop the Solara service before restoring")

// Restore replaces the live database (+ .meta, + EOS files when requested)
// with a snapshot. Order: refuse while the live file is locked; verify the
// snapshot (a wrong/missing password for an encrypted snapshot is a refusal);
// take a "pre-restore" safety snapshot of the current state; stage copies
// next to the live files; swap them in with renames, keeping the replaced
// files as "*.replaced-<ts>" until every swap succeeded (rolled back on
// failure); then delete the replaced files and re-check the restored store.
func Restore(name string, o RestoreOptions) (*RestoreResult, error) {
	if o.DBPath == "" {
		return nil, errors.New("restore: live database path not set")
	}
	snapDir, err := Resolve(o.Dir, name)
	if err != nil {
		return nil, err
	}
	m, err := ReadManifest(snapDir)
	if err != nil {
		return nil, err
	}
	if m.DBFile != filepath.Base(o.DBPath) {
		return nil, fmt.Errorf("snapshot %s is of %s, not %s", filepath.Base(snapDir), m.DBFile, filepath.Base(o.DBPath))
	}
	res := &RestoreResult{Snapshot: filepath.Base(snapDir)}

	liveExists := true
	switch err := docdb.ProbeUnlocked(o.DBPath, 2*time.Second); {
	case errors.Is(err, docdb.ErrDatabaseInUse):
		return nil, ErrInUse
	case errors.Is(err, os.ErrNotExist):
		liveExists = false
	case err != nil:
		return nil, fmt.Errorf("check live database: %w", err)
	}

	v := Verify(snapDir, o.Password)
	res.Verify = v
	if !v.OK {
		return res, fmt.Errorf("snapshot failed verification, refusing to restore: %s", strings.Join(v.Problems, "; "))
	}
	if m.Encrypted && o.Password == nil {
		return res, errors.New("snapshot is encrypted and no database password is configured, refusing to restore")
	}

	if liveExists {
		safety, err := CreateOffline(o.DBPath, Options{Dir: o.Dir, EOSDir: o.EOSDir, Label: LabelPreRestore, Reason: "automatic safety snapshot before restoring " + res.Snapshot, Now: o.Now})
		if errors.Is(err, docdb.ErrDatabaseInUse) {
			return res, ErrInUse
		}
		if err != nil {
			return res, fmt.Errorf("safety snapshot failed, refusing to restore: %w", err)
		}
		res.SafetySnapshot = safety.Name
	}

	have := map[string]FileEntry{}
	for _, f := range m.Files {
		have[f.Name] = f
	}
	type swap struct {
		src  *FileEntry // nil: remove the live file
		dst  string
		tmp  string
		old  string
		done bool
		had  bool
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	ts := now().UTC().Format(timeLayout)
	var swaps []*swap
	add := func(snapName, dst string, removeIfAbsent bool) {
		if f, ok := have[snapName]; ok {
			f := f
			swaps = append(swaps, &swap{src: &f, dst: dst})
		} else if removeIfAbsent {
			if _, err := os.Stat(dst); err == nil {
				swaps = append(swaps, &swap{dst: dst})
			}
		}
	}
	add(m.DBFile, o.DBPath, false)
	add(m.DBFile+".meta", o.DBPath+".meta", false) // a snapshot without a sidecar keeps the live one
	if o.IncludeEOS {
		if o.EOSDir == "" {
			return res, errors.New("--include-eos: EOS directory not set")
		}
		if !m.EOSCaptured {
			return res, errors.New("--include-eos: this snapshot did not capture the EOS files")
		}
		for _, f := range EOSFiles {
			add(f, filepath.Join(o.EOSDir, f), true)
		}
	}

	cleanupTmp := func() {
		for _, s := range swaps {
			if s.tmp != "" {
				_ = os.Remove(s.tmp)
			}
		}
	}
	// Stage: copy every file next to its destination and re-hash the copy.
	for _, s := range swaps {
		if s.src == nil {
			continue
		}
		s.tmp = s.dst + ".restore-tmp"
		_ = os.Remove(s.tmp)
		if err := copyVerified(filepath.Join(snapDir, s.src.Name), s.tmp, *s.src); err != nil {
			cleanupTmp()
			return res, fmt.Errorf("stage %s: %w", s.src.Name, err)
		}
	}
	// Swap.
	rollback := func() {
		for i := len(swaps) - 1; i >= 0; i-- {
			s := swaps[i]
			if !s.done {
				continue
			}
			if s.src != nil {
				_ = os.Remove(s.dst)
			}
			if s.had {
				_ = os.Rename(s.old, s.dst)
			}
		}
		cleanupTmp()
	}
	for _, s := range swaps {
		s.old = s.dst + ".replaced-" + ts
		if _, err := os.Stat(s.dst); err == nil {
			if err := os.Rename(s.dst, s.old); err != nil {
				rollback()
				return res, fmt.Errorf("move aside %s: %w", s.dst, err)
			}
			s.had = true
		}
		s.done = true
		if s.src != nil {
			if err := os.Rename(s.tmp, s.dst); err != nil {
				rollback()
				return res, fmt.Errorf("install %s: %w", s.dst, err)
			}
			s.tmp = ""
		}
	}
	syncDir(filepath.Dir(o.DBPath))
	if o.IncludeEOS {
		syncDir(o.EOSDir)
	}
	// Final check of the installed store before discarding the old files.
	if rep, err := docdb.InspectFile(o.DBPath, o.Password, 2*time.Second); err != nil || len(rep.CheckErrors) > 0 || (rep.PasswordChecked && !rep.PasswordOK) {
		rollback()
		if err == nil {
			err = fmt.Errorf("restored store failed its check: %v", rep.CheckErrors)
		}
		return res, fmt.Errorf("restore rolled back: %w", err)
	}
	for _, s := range swaps {
		if s.had {
			_ = os.Remove(s.old)
		}
		if s.src != nil {
			res.Replaced = append(res.Replaced, s.dst)
		} else {
			res.Removed = append(res.Removed, s.dst)
		}
	}
	return res, nil
}

func copyVerified(src, dst string, want FileEntry) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fe, err := writeFile(dst, func(w io.Writer) error {
		_, err := io.Copy(w, in)
		return err
	})
	if err != nil {
		_ = os.Remove(dst)
		return err
	}
	if fe.Size != want.Size || fe.SHA256 != want.SHA256 {
		_ = os.Remove(dst)
		return errors.New("copy does not match the manifest hash")
	}
	return nil
}
