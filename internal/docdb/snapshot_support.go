package docdb

// Snapshot support (used by internal/snapshot). Additive only: nothing here
// changes how documents are stored or encrypted.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
	berrors "go.etcd.io/bbolt/errors"
)

// CollectionCount is the number of stored documents (bucket keys) of one
// collection. Counting never decrypts a payload.
type CollectionCount struct {
	Name      string `json:"name"`
	Documents int    `json:"documents"`
}

// SnapshotInfo describes what WriteSnapshot captured.
type SnapshotInfo struct {
	Bytes       int64
	Meta        []byte // the .meta sidecar as it belongs to the captured state
	Encrypted   bool
	Collections []CollectionCount
}

// ErrDatabaseInUse is returned when the bbolt file lock is held by another
// handle (normally a running Solara process).
var ErrDatabaseInUse = errors.New("database file is in use (locked by another process or handle)")

// IsEncrypted reports whether this database encrypts document payloads.
func (d *Database) IsEncrypted() bool { return d.aead != nil }

// WriteSnapshot writes a consistent copy of the live bbolt file to w using a
// read transaction (writers keep running). The returned Meta is the sidecar
// encoding taken under the same lock that GetCollection/DropCollection hold
// while they change buckets and metadata, so database and sidecar agree.
func (d *Database) WriteSnapshot(w io.Writer) (*SnapshotInfo, error) {
	d.mu.Lock()
	tx, err := d.bolt.Begin(false)
	if err != nil {
		d.mu.Unlock()
		return nil, err
	}
	d.meta.mu.Lock()
	meta := d.meta.encode()
	d.meta.mu.Unlock()
	d.mu.Unlock()
	defer func() { _ = tx.Rollback() }()

	info := &SnapshotInfo{Meta: meta}
	info.Encrypted, info.Collections = describeTx(tx)
	n, err := tx.WriteTo(w)
	info.Bytes = n
	if err != nil {
		return nil, err
	}
	return info, nil
}

func describeTx(tx *bolt.Tx) (encrypted bool, cols []CollectionCount) {
	if b := tx.Bucket(sysBucket); b != nil && b.Get([]byte("check")) != nil {
		encrypted = true
	}
	cols = []CollectionCount{}
	_ = tx.ForEach(func(name []byte, b *bolt.Bucket) error {
		if string(name) == string(sysBucket) {
			return nil
		}
		cols = append(cols, CollectionCount{Name: string(name), Documents: b.Stats().KeyN})
		return nil
	})
	sort.Slice(cols, func(i, j int) bool { return cols[i].Name < cols[j].Name })
	return
}

// FileReport is the result of InspectFile.
type FileReport struct {
	Encrypted   bool
	Collections []CollectionCount
	CheckErrors []string // bbolt structural (tx.Check) findings
	// PasswordChecked is true when a password was supplied and the file is
	// encrypted; PasswordOK then says whether it decrypts the check value.
	PasswordChecked bool
	PasswordOK      bool
}

// InspectFile opens a store file read-only (shared lock, waiting at most
// timeout), runs bbolt's structural check, counts documents and, when
// password is non-nil, confirms the password decrypts the "__solara" check
// value. Returns ErrDatabaseInUse when the file is locked by a writer.
func InspectFile(path string, password *string, timeout time.Duration) (*FileReport, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	bdb, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: timeout})
	if err != nil {
		if errors.Is(err, berrors.ErrTimeout) {
			return nil, ErrDatabaseInUse
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer bdb.Close()
	rep := &FileReport{}
	err = bdb.View(func(tx *bolt.Tx) error {
		for cerr := range tx.Check() {
			rep.CheckErrors = append(rep.CheckErrors, cerr.Error())
		}
		rep.Encrypted, rep.Collections = describeTx(tx)
		if password != nil && rep.Encrypted {
			rep.PasswordChecked = true
			b := tx.Bucket(sysBucket)
			rep.PasswordOK = checkPassword(*password, b.Get([]byte("salt")), b.Get([]byte("check")))
		}
		return nil
	})
	return rep, err
}

// checkPassword reproduces initCrypto's key derivation and check-value test.
func checkPassword(password string, salt, check []byte) bool {
	if salt == nil || check == nil {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, 100_000, 32)
	if err != nil {
		return false
	}
	blk, err := aes.NewCipher(key)
	if err != nil {
		return false
	}
	aead, err := cipher.NewGCM(blk)
	if err != nil {
		return false
	}
	n := aead.NonceSize()
	if len(check) < n {
		return false
	}
	p, err := aead.Open(nil, check[:n], check[n:], nil)
	return err == nil && string(p) == "solara"
}

// ProbeUnlocked reports whether no writer holds the store file. It briefly
// opens the file read-only: bbolt's shared lock conflicts with the exclusive
// lock every read-write handle (a running Solara) holds, on both Windows
// (LockFileEx) and Unix (flock), and a read-only open never modifies or
// grows the file. A missing file is reported as os.ErrNotExist.
func ProbeUnlocked(path string, timeout time.Duration) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	bdb, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: timeout})
	if err != nil {
		if errors.Is(err, berrors.ErrTimeout) {
			return ErrDatabaseInUse
		}
		return err
	}
	return bdb.Close()
}

// SnapshotFile is the offline counterpart of WriteSnapshot: it opens a store
// file that no process has open (read-only, shared lock, waiting at most
// timeout), writes a consistent copy to w and returns the sidecar bytes found
// at path+".meta" (nil when there is none). Returns ErrDatabaseInUse when a
// writer holds the file.
func SnapshotFile(path string, w io.Writer, timeout time.Duration) (*SnapshotInfo, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = time.Second
	}
	bdb, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: timeout})
	if err != nil {
		if errors.Is(err, berrors.ErrTimeout) {
			return nil, ErrDatabaseInUse
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer bdb.Close()
	info := &SnapshotInfo{}
	if raw, err := os.ReadFile(path + ".meta"); err == nil {
		info.Meta = raw
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	err = bdb.View(func(tx *bolt.Tx) error {
		info.Encrypted, info.Collections = describeTx(tx)
		n, err := tx.WriteTo(w)
		info.Bytes = n
		return err
	})
	if err != nil {
		return nil, err
	}
	return info, nil
}
