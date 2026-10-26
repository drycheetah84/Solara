// Package snapshot takes, verifies, lists, prunes and restores point-in-time
// copies of a Solara document store (the bbolt "<name>.solara.db" file, its
// ".meta" sidecar and, optionally, the EOS gateway's sessions.json /
// lobbies.json). Snapshots are byte copies: an encrypted store stays
// encrypted, nothing is ever decrypted. See docs/SNAPSHOTS.md.
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"solara/internal/docdb"
)

const (
	// Format identifies the snapshot layout in manifest.json.
	Format = "solara-snapshot/1"
	// ManifestName is the manifest file inside a snapshot directory.
	ManifestName = "manifest.json"
	// VerifyCacheName caches the last verification result (not hashed).
	VerifyCacheName = "verify.json"
	// DefaultKeep is the default number of snapshots retained by Prune.
	DefaultKeep = 28
	// DefaultIntervalHours is the default automatic snapshot interval.
	DefaultIntervalHours = 6

	// LabelKeep and LabelPreRestore are protected from Prune unless forced.
	LabelKeep       = "keep"
	LabelPreRestore = "pre-restore"

	timeLayout = "20060102T150405Z"
	tmpPrefix  = ".tmp-"
)

// EOSFiles are the EOS gateway state files captured when present.
var EOSFiles = []string{"sessions.json", "lobbies.json"}

// FileEntry is one captured file.
type FileEntry struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is manifest.json.
type Manifest struct {
	Format       string                  `json:"format"`
	CreatedAt    string                  `json:"created_at"`
	Label        string                  `json:"label"`
	Reason       string                  `json:"reason"`
	DBName       string                  `json:"db_name"`
	DBFile       string                  `json:"db_file"`
	Encrypted    bool                    `json:"encrypted"`
	Files        []FileEntry             `json:"files"`
	Collections  []docdb.CollectionCount `json:"collections"`
	SourceHost   string                  `json:"source_host"`
	EOSCaptured  bool                    `json:"eos_captured"`
	MetaCaptured bool                    `json:"meta_captured"`
}

// Created returns CreatedAt parsed (zero time when malformed).
func (m *Manifest) Created() time.Time {
	t, _ := time.Parse(time.RFC3339, m.CreatedAt)
	return t
}

// Snapshot is a snapshot directory on disk.
type Snapshot struct {
	Name     string
	Path     string
	Manifest *Manifest
}

// Options controls Create.
type Options struct {
	Dir    string // snapshots directory (DefaultDir() when empty)
	EOSDir string // directory holding sessions.json / lobbies.json; "" = do not capture
	Label  string // default "manual"
	Reason string
	Now    func() time.Time // tests
}

// DefaultDir is $SOLARA_SNAPSHOT_DIR, else "snapshots" in the working
// directory (where the database files live).
func DefaultDir() string {
	if v := strings.TrimSpace(os.Getenv("SOLARA_SNAPSHOT_DIR")); v != "" {
		return v
	}
	wd, err := os.Getwd()
	if err != nil {
		return "snapshots"
	}
	return filepath.Join(wd, "snapshots")
}

var labelRe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// SanitizeLabel maps a label to the characters allowed in directory names.
func SanitizeLabel(l string) string {
	l = strings.Trim(labelRe.ReplaceAllString(strings.TrimSpace(l), "-"), "-.")
	if l == "" {
		return "manual"
	}
	if len(l) > 64 {
		l = l[:64]
	}
	return l
}

// DBNameFromPath returns "production" for ".../production.solara.db".
func DBNameFromPath(p string) string {
	return strings.TrimSuffix(filepath.Base(p), docdb.FileSuffix)
}

// Create takes a hot snapshot of an open database: a consistent copy through
// a bbolt read transaction, so writers keep running.
func Create(db *docdb.Database, o Options) (*Snapshot, error) {
	if db == nil {
		return nil, errors.New("snapshot: nil database")
	}
	path := db.Descriptor().Path
	return create(path, o, func(w io.Writer) (*docdb.SnapshotInfo, error) { return db.WriteSnapshot(w) })
}

// CreateOffline snapshots a store file no process has open (CLI, and the
// pre-restore safety snapshot). Returns docdb.ErrDatabaseInUse when a running
// server holds the file.
func CreateOffline(dbPath string, o Options) (*Snapshot, error) {
	return create(dbPath, o, func(w io.Writer) (*docdb.SnapshotInfo, error) {
		return docdb.SnapshotFile(dbPath, w, 2*time.Second)
	})
}

func create(dbPath string, o Options, write func(io.Writer) (*docdb.SnapshotInfo, error)) (*Snapshot, error) {
	dir := o.Dir
	if dir == "" {
		dir = DefaultDir()
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	created := now().UTC()
	label := SanitizeLabel(o.Label)
	base := created.Format(timeLayout) + "_" + label
	name := base
	for i := 2; ; i++ {
		_, e1 := os.Stat(filepath.Join(dir, name))
		_, e2 := os.Stat(filepath.Join(dir, tmpPrefix+name))
		if errors.Is(e1, os.ErrNotExist) && errors.Is(e2, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	tmp := filepath.Join(dir, tmpPrefix+name)
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(tmp)
		}
	}()

	host, _ := os.Hostname()
	m := &Manifest{
		Format: Format, CreatedAt: created.Format(time.RFC3339), Label: label, Reason: o.Reason,
		DBName: DBNameFromPath(dbPath), DBFile: filepath.Base(dbPath), SourceHost: host,
		Files: []FileEntry{},
	}

	// Database (consistent copy) + sidecar from the same instant.
	var info *docdb.SnapshotInfo
	fe, err := writeFile(filepath.Join(tmp, m.DBFile), func(w io.Writer) error {
		var err error
		info, err = write(w)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("snapshot database: %w", err)
	}
	m.Files = append(m.Files, fe)
	m.Encrypted, m.Collections = info.Encrypted, info.Collections
	if m.Collections == nil {
		m.Collections = []docdb.CollectionCount{}
	}
	if info.Meta != nil {
		fe, err := writeFile(filepath.Join(tmp, m.DBFile+".meta"), func(w io.Writer) error {
			_, err := w.Write(info.Meta)
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("snapshot metadata: %w", err)
		}
		m.Files = append(m.Files, fe)
		m.MetaCaptured = true
	}

	if o.EOSDir != "" {
		m.EOSCaptured = true
		for _, f := range EOSFiles {
			raw, err := readStableJSON(filepath.Join(o.EOSDir, f))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("snapshot %s: %w", f, err)
			}
			fe, err := writeFile(filepath.Join(tmp, f), func(w io.Writer) error {
				_, err := w.Write(raw)
				return err
			})
			if err != nil {
				return nil, fmt.Errorf("snapshot %s: %w", f, err)
			}
			m.Files = append(m.Files, fe)
		}
	}

	mj, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if _, err := writeFile(filepath.Join(tmp, ManifestName), func(w io.Writer) error {
		_, err := w.Write(append(mj, '\n'))
		return err
	}); err != nil {
		return nil, err
	}
	syncDir(tmp)
	final := filepath.Join(dir, name)
	if err := os.Rename(tmp, final); err != nil {
		return nil, err
	}
	syncDir(dir)
	ok = true
	return &Snapshot{Name: name, Path: final, Manifest: m}, nil
}

// readStableJSON reads an EOS state file. The gateway rewrites these files in
// place (os.WriteFile), so a read can race a write; retry until the content
// is valid JSON (an invalid file after the retries is captured as-is).
func readStableJSON(p string) ([]byte, error) {
	var raw []byte
	var err error
	for i := 0; i < 5; i++ {
		raw, err = os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if json.Valid(raw) {
			return raw, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return raw, nil
}

// writeFile creates p, streams content through a SHA-256 hasher and fsyncs.
func writeFile(p string, content func(io.Writer) error) (FileEntry, error) {
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return FileEntry{}, err
	}
	h := sha256.New()
	cw := &countWriter{w: io.MultiWriter(f, h)}
	if err := content(cw); err != nil {
		f.Close()
		return FileEntry{}, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return FileEntry{}, err
	}
	if err := f.Close(); err != nil {
		return FileEntry{}, err
	}
	return FileEntry{Name: filepath.Base(p), Size: cw.n, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// hashFile returns size and hex SHA-256 of p.
func hashFile(p string) (int64, string, error) {
	f, err := os.Open(p)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

// syncDir fsyncs a directory so a rename is durable (unsupported on Windows,
// where the error is ignored; NTFS journals the rename itself).
func syncDir(dir string) {
	if f, err := os.Open(dir); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
}

// ReadManifest loads <snapshotDir>/manifest.json.
func ReadManifest(snapshotDir string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(snapshotDir, ManifestName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}
	if m.Format != Format {
		return nil, fmt.Errorf("manifest: unsupported format %q (want %q)", m.Format, Format)
	}
	return &m, nil
}
