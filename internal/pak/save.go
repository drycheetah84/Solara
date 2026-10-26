package pak

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Save pipeline phases reported through SaveOptions.Progress.
const (
	PhaseWriting   = "writing"
	PhaseVerifying = "verifying"
	PhaseBackingUp = "backing-up"
	PhaseSwapping  = "swapping"
)

const (
	tmpSuffix      = ".solara-tmp"
	originalSuffix = ".original"
	bakInfix       = ".bak-"
	bakTimeLayout  = "20060102-150405"
	// DefaultKeepBackups is the number of timestamped backups retained.
	DefaultKeepBackups = 10
	diskMargin         = 64 << 20
)

// ErrFileInUse is returned when the pak cannot be replaced because another
// process (normally the game or its launcher) holds it open.
var ErrFileInUse = errors.New("the pak file is in use by another program - close the game / launcher first")

// SaveOptions configures Save and Restore.
type SaveOptions struct {
	// Progress receives the current phase and a 0..1 fraction within it.
	Progress func(phase string, fraction float64)
	// KeepBackups is the number of timestamped backups to keep (default 10).
	KeepBackups int
	// Now overrides the clock (tests).
	Now func() time.Time
	// Recompress stores replaced Oodle entries of v11 paks compressed again
	// (see RebuildOptions.Recompress); requires Codec.
	Recompress bool
	// Codec is the Oodle codec used for recompressing and for checking
	// compressed replacements during verification (may be nil).
	Codec Codec

	afterWrite func() error // test hook: runs after the temp file is written
}

func (o *SaveOptions) phase(name string, f float64) {
	if o != nil && o.Progress != nil {
		o.Progress(name, f)
	}
}

func (o *SaveOptions) now() time.Time {
	if o != nil && o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *SaveOptions) codec() Codec {
	if o == nil {
		return nil
	}
	return o.Codec
}

func (o *SaveOptions) keep() int {
	if o != nil && o.KeepBackups > 0 {
		return o.KeepBackups
	}
	return DefaultKeepBackups
}

// SaveResult describes a completed save or restore.
type SaveResult struct {
	Backup         string   // timestamped backup of the previous pak
	OriginalBackup string   // path of the permanent .original backup
	NewSize        int64    // size of the new pak
	Changed        []string // entries whose content changed (save only)
	Recompressed   []string // entries stored Oodle-compressed (save only)
}

// TempPath returns the temporary output path used for pakPath.
func TempPath(pakPath string) string { return pakPath + tmpSuffix }

// OriginalPath returns the permanent first-save backup path for pakPath.
func OriginalPath(pakPath string) string { return pakPath + originalSuffix }

// Save safely replaces entries of the pak at pakPath:
//
//  1. check the pak is not in use and that there is enough free disk space;
//  2. stream a rebuilt pak to <pak>.solara-tmp (fsynced);
//  3. fully verify the temp file against the original;
//  4. back up the current pak (<pak>.original the first time ever, plus a
//     timestamped <pak>.bak-YYYYMMDD-HHMMSS, keeping the newest N);
//  5. atomically rename the temp file over the pak.
//
// On any failure the original pak is left untouched and the temp file is
// removed.
func Save(pakPath string, replacements map[string][]byte, opts *SaveOptions) (*SaveResult, error) {
	if len(replacements) == 0 {
		return nil, errors.New("no replacements given")
	}
	pakPath, err := filepath.Abs(pakPath)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(pakPath)
	if err != nil {
		return nil, err
	}
	src, err := Open(pakPath)
	if err != nil {
		return nil, err
	}
	if err := src.checkReplacements(replacements); err != nil {
		return nil, err
	}
	if err := checkNotInUse(pakPath); err != nil {
		return nil, err
	}
	newSize := st.Size()
	for name, d := range replacements {
		newSize += int64(len(d)) - src.byName[name].Size
	}
	if err := checkSpace(pakPath, st.Size(), newSize); err != nil {
		return nil, err
	}

	tmp := TempPath(pakPath)
	_ = os.Remove(tmp) // stale temp from an interrupted run
	defer os.Remove(tmp)

	opts.phase(PhaseWriting, 0)
	rr, err := RebuildWith(pakPath, tmp, replacements, &RebuildOptions{
		Progress:   func(d, t int64) { opts.phase(PhaseWriting, frac(d, t)) },
		Recompress: opts != nil && opts.Recompress,
		Codec:      opts.codec(),
	})
	if err != nil {
		return nil, fmt.Errorf("writing new pak: %w", err)
	}
	if opts != nil && opts.afterWrite != nil {
		if err := opts.afterWrite(); err != nil {
			return nil, err
		}
	}
	opts.phase(PhaseVerifying, 0)
	if _, err := Verify(tmp, &VerifyOptions{Source: pakPath, Expect: replacements, Decompressor: opts.codec(),
		Progress: func(d, t int64) { opts.phase(PhaseVerifying, frac(d, t)) }}); err != nil {
		return nil, fmt.Errorf("new pak failed verification (original untouched): %w", err)
	}
	res, err := backupAndSwap(pakPath, tmp, st, opts)
	if err != nil {
		return nil, err
	}
	res.NewSize = rr.Size
	res.Changed = rr.Changed
	res.Recompressed = rr.Recompressed
	return res, nil
}

// Restore replaces the pak with a verified copy of backupPath (which must be
// one of the pak's backups). The current pak is backed up first.
func Restore(pakPath, backupPath string, opts *SaveOptions) (*SaveResult, error) {
	pakPath, err := filepath.Abs(pakPath)
	if err != nil {
		return nil, err
	}
	backupPath, err = filepath.Abs(backupPath)
	if err != nil {
		return nil, err
	}
	if !isBackupOf(pakPath, backupPath) {
		return nil, fmt.Errorf("%s is not a backup of %s", backupPath, pakPath)
	}
	st, err := os.Stat(pakPath)
	if err != nil {
		return nil, err
	}
	bst, err := os.Stat(backupPath)
	if err != nil {
		return nil, err
	}
	if err := checkNotInUse(pakPath); err != nil {
		return nil, err
	}
	if err := checkSpace(pakPath, st.Size(), bst.Size()); err != nil {
		return nil, err
	}
	tmp := TempPath(pakPath)
	_ = os.Remove(tmp)
	defer os.Remove(tmp)

	opts.phase(PhaseWriting, 0)
	if _, err := copyFile(backupPath, tmp, func(d, t int64) { opts.phase(PhaseWriting, frac(d, t)) }); err != nil {
		return nil, fmt.Errorf("copying backup: %w", err)
	}
	if opts != nil && opts.afterWrite != nil {
		if err := opts.afterWrite(); err != nil {
			return nil, err
		}
	}
	opts.phase(PhaseVerifying, 0)
	if _, err := Verify(tmp, &VerifyOptions{Progress: func(d, t int64) { opts.phase(PhaseVerifying, frac(d, t)) }}); err != nil {
		return nil, fmt.Errorf("backup failed verification (pak untouched): %w", err)
	}
	if ok, err := sameFile(backupPath, tmp); err != nil || !ok {
		return nil, errors.New("copy of backup does not match the backup (pak untouched)")
	}
	res, err := backupAndSwap(pakPath, tmp, st, opts)
	if err != nil {
		return nil, err
	}
	res.NewSize = bst.Size()
	return res, nil
}

// backupAndSwap backs up the current pak and renames tmp over it.
func backupAndSwap(pakPath, tmp string, before os.FileInfo, opts *SaveOptions) (*SaveResult, error) {
	res := &SaveResult{OriginalBackup: OriginalPath(pakPath)}
	opts.phase(PhaseBackingUp, 0)
	if !unchanged(pakPath, before) {
		return nil, errors.New("the pak changed on disk during the save; aborted (pak untouched)")
	}
	if _, err := os.Stat(res.OriginalBackup); errors.Is(err, os.ErrNotExist) {
		if err := verifiedCopy(pakPath, res.OriginalBackup); err != nil {
			return nil, fmt.Errorf("creating %s: %w", filepath.Base(res.OriginalBackup), err)
		}
	} else if err != nil {
		return nil, err
	}
	opts.phase(PhaseBackingUp, 0.5)
	bak := timestampedPath(pakPath, opts.now())
	if err := verifiedCopy(pakPath, bak); err != nil {
		return nil, fmt.Errorf("creating backup: %w", err)
	}
	res.Backup = bak
	opts.phase(PhaseBackingUp, 1)

	opts.phase(PhaseSwapping, 0)
	if !unchanged(pakPath, before) {
		os.Remove(bak)
		return nil, errors.New("the pak changed on disk during the save; aborted (pak untouched)")
	}
	if err := replaceFile(tmp, pakPath); err != nil {
		os.Remove(bak)
		if isInUse(err) {
			return nil, ErrFileInUse
		}
		if isAccessDenied(err) {
			return nil, fmt.Errorf("%w (access denied replacing the pak; if the game is closed, the folder may need write permission): %v", ErrFileInUse, err)
		}
		return nil, fmt.Errorf("replacing pak (original untouched): %w", err)
	}
	syncDir(filepath.Dir(pakPath))
	opts.phase(PhaseSwapping, 1)
	pruneBackups(pakPath, opts.keep())
	return res, nil
}

func frac(d, t int64) float64 {
	if t <= 0 {
		return 0
	}
	f := float64(d) / float64(t)
	if f > 1 {
		f = 1
	}
	return f
}

func unchanged(path string, before os.FileInfo) bool {
	st, err := os.Stat(path)
	return err == nil && st.Size() == before.Size() && st.ModTime().Equal(before.ModTime())
}

func timestampedPath(pakPath string, t time.Time) string {
	base := pakPath + bakInfix + t.Format(bakTimeLayout)
	p := base
	for i := 2; ; i++ {
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return p
		}
		p = fmt.Sprintf("%s-%d", base, i)
	}
}

// checkSpace makes sure the volume can hold the temp file plus backups.
func checkSpace(pakPath string, curSize, newSize int64) error {
	need := newSize + curSize + diskMargin // temp + timestamped backup
	if _, err := os.Stat(OriginalPath(pakPath)); errors.Is(err, os.ErrNotExist) {
		need += curSize
	}
	free, err := freeSpace(filepath.Dir(pakPath))
	if err != nil {
		return nil // cannot determine; proceed (writes still fail safely)
	}
	if free < uint64(need) {
		return fmt.Errorf("not enough free disk space: need about %s, only %s available on the pak's drive", human(need), human(int64(free)))
	}
	return nil
}

func human(n int64) string {
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}

// copyFile copies src to dst (fsynced), returning the SHA-256 of the data.
func copyFile(src, dst string, progress ProgressFunc) ([]byte, error) {
	in, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	st, err := in.Stat()
	if err != nil {
		return nil, err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	cw := &countingWriter{w: io.MultiWriter(out, h), total: st.Size(), progress: progress}
	_, err = io.CopyBuffer(cw, in, make([]byte, 1<<20))
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(dst)
		return nil, err
	}
	return h.Sum(nil), nil
}

// verifiedCopy copies src to dst via dst+".partial" and checks the result.
func verifiedCopy(src, dst string) error {
	part := dst + ".partial"
	sum, err := copyFile(src, part, nil)
	if err != nil {
		return err
	}
	got, err := fileSHA256(part)
	if err != nil || string(got) != string(sum) {
		os.Remove(part)
		return errors.New("backup copy does not match the source")
	}
	if err := os.Rename(part, dst); err != nil {
		os.Remove(part)
		return err
	}
	return nil
}

func fileSHA256(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.CopyBuffer(h, f, make([]byte, 1<<20)); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func sameFile(a, b string) (bool, error) {
	ha, err := fileSHA256(a)
	if err != nil {
		return false, err
	}
	hb, err := fileSHA256(b)
	if err != nil {
		return false, err
	}
	return string(ha) == string(hb), nil
}

// Backup describes a backup file of a pak.
type Backup struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
	Kind    string // "original" or "timestamped"
}

// ListBackups returns the pak's backups: the .original first, then
// timestamped backups newest first.
func ListBackups(pakPath string) ([]Backup, error) {
	pakPath, err := filepath.Abs(pakPath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(pakPath)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var orig []Backup
	var ts []Backup
	for _, de := range ents {
		if de.IsDir() {
			continue
		}
		full := filepath.Join(dir, de.Name())
		kind := backupKind(pakPath, full)
		if kind == "" {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		b := Backup{Path: full, Name: de.Name(), Size: info.Size(), ModTime: info.ModTime(), Kind: kind}
		if kind == "original" {
			orig = append(orig, b)
		} else {
			ts = append(ts, b)
		}
	}
	sort.Slice(ts, func(i, j int) bool { return bakSortKey(ts[i].Name) > bakSortKey(ts[j].Name) })
	return append(orig, ts...), nil
}

// bakSortKey orders same-second suffixes (-2, -3...) after the plain name.
func bakSortKey(name string) string {
	i := strings.LastIndex(name, bakInfix)
	rest := name[i+len(bakInfix):]
	stamp, n := rest, 1
	if len(rest) > len(bakTimeLayout) {
		stamp = rest[:len(bakTimeLayout)]
		n, _ = strconv.Atoi(rest[len(bakTimeLayout)+1:])
	}
	return fmt.Sprintf("%s-%06d", stamp, n)
}

func backupKind(pakPath, candidate string) string {
	if !strings.EqualFold(filepath.Dir(pakPath), filepath.Dir(candidate)) {
		return ""
	}
	base := filepath.Base(pakPath)
	name := filepath.Base(candidate)
	if !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
		return ""
	}
	rest := name[len(base):]
	if strings.EqualFold(rest, originalSuffix) {
		return "original"
	}
	if strings.HasPrefix(rest, bakInfix) {
		stamp := rest[len(bakInfix):]
		if len(stamp) < len(bakTimeLayout) {
			return ""
		}
		if _, err := time.Parse(bakTimeLayout, stamp[:len(bakTimeLayout)]); err != nil {
			return ""
		}
		suf := stamp[len(bakTimeLayout):]
		if suf == "" {
			return "timestamped"
		}
		if suf[0] == '-' && len(suf) > 1 && strings.Trim(suf[1:], "0123456789") == "" {
			return "timestamped"
		}
	}
	return ""
}

func isBackupOf(pakPath, candidate string) bool { return backupKind(pakPath, candidate) != "" }

// pruneBackups keeps the newest keep timestamped backups. The .original
// backup is never deleted.
func pruneBackups(pakPath string, keep int) {
	bs, err := ListBackups(pakPath)
	if err != nil {
		return
	}
	n := 0
	for _, b := range bs {
		if b.Kind != "timestamped" {
			continue
		}
		n++
		if n > keep {
			os.Remove(b.Path)
		}
	}
}
