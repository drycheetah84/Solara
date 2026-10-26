// Package docdb is the Go port of AUnrealFeatures.Hosting.Database
// (AstraDatabase / AstraDatabaseManager / AstraDatabaseBuilder /
// AstraDatabaseBytedata) together with the subset of LiteDB's
// ILiteCollection<T> the original backend used.
//
// Storage engine: LiteDB has no Go implementation, so documents are kept in a
// bbolt file (one bucket per collection, keyed by the 12-byte ObjectId so
// iteration order equals LiteDB's _id index order). When the original
// configured a password, document payloads are encrypted (AES-256-GCM,
// PBKDF2-SHA256 key). Existing LiteDB data is migrated with
// `solara import-litedb` (see docs/DATA_MIGRATION.md).
package docdb

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	bolt "go.etcd.io/bbolt"

	"solara/internal/dotnet"
	"solara/internal/logging"
)

// Descriptor mirrors SimpleDatabaseDescriptor.
type Descriptor struct {
	DatabaseId int
	Name       string
	Path       string
	Password   *string
}

// Options mirrors DatabaseOptions.
type Options struct {
	DatabaseId   int
	FriendlyName string
	UsePassword  bool
	Password     string
}

// FileSuffix distinguishes Solara's store from a LiteDB file of the same name
// so an original "production.db" is never opened (or clobbered) by Solara.
const FileSuffix = ".solara.db"

// Builder mirrors AstraDatabaseBuilder.
type Builder struct {
	descriptors []*Descriptor
	Dir         string // defaults to the working directory (Environment.CurrentDirectory)
	log         *logging.Logger
}

// NewBuilder mirrors AstraDatabaseBuilder.Shared (a fresh builder each time).
func NewBuilder() *Builder { return &Builder{log: logging.New("AstraDatabaseBuilder")} }

// WithDatabase mirrors IDatabaseBuilder.WithDatabase.
func (b *Builder) WithDatabase(name string, o Options) *Builder {
	for _, d := range b.descriptors {
		if d.Name == name {
			b.log.Warning("Duplicate entry found for '{Name}', cannot add another database.", name)
			return b
		}
	}
	dir := b.Dir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	d := &Descriptor{DatabaseId: o.DatabaseId, Name: name, Path: filepath.Join(dir, name+FileSuffix)}
	if o.UsePassword {
		p := o.Password
		d.Password = &p
	}
	b.descriptors = append(b.descriptors, d)
	return b
}

// Build mirrors IDatabaseBuilder.Build.
func (b *Builder) Build() *Manager { return NewManager(b.descriptors) }

// Manager mirrors AstraDatabaseManager.
type Manager struct {
	descriptors []*Descriptor
	mu          sync.Mutex
	cached      []*Database
	log         *logging.Logger
}

func NewManager(ds []*Descriptor) *Manager {
	m := &Manager{descriptors: ds, log: logging.New("AstraDatabaseManager")}
	if len(ds) == 0 {
		m.log.Warning("The 'descriptors' field has 0 elements! This means no databases are currently being managed. Is this supposed to happen?")
	}
	return m
}

// GetDatabase mirrors IDatabaseManager.GetDatabase(name, id): the first
// descriptor whose id OR name matches.
func (m *Manager) GetDatabase(name string, id int) (*Database, error) {
	if name == "" && id == 0 {
		return nil, errors.New("You cannot get a database when 'name' or 'id' are not set.")
	}
	var desc *Descriptor
	for _, d := range m.descriptors {
		if d.DatabaseId == id || d.Name == name {
			desc = d
			break
		}
	}
	if desc == nil {
		m.log.Warning("Tried to get a database that didnt exist inside the descriptor list.")
		return nil, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.cached {
		if c.desc.DatabaseId == id || c.desc.Name == name {
			return c, nil
		}
	}
	db, err := Open(desc)
	if err != nil {
		return nil, err
	}
	m.cached = append(m.cached, db)
	return db, nil
}

// Close closes every opened database.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range m.cached {
		_ = d.Close()
	}
	m.cached = nil
}

// Database mirrors AstraDatabase.
type Database struct {
	guid string
	desc *Descriptor
	bolt *bolt.DB
	aead cipher.AEAD
	meta *Bytedata
	log  *logging.Logger
	mu   sync.Mutex // guards meta
}

var sysBucket = []byte("__solara")

// Open opens (creating if needed) the store for a descriptor.
func Open(desc *Descriptor) (*Database, error) {
	bdb, err := bolt.Open(desc.Path, 0o600, nil)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", desc.Path, err)
	}
	d := &Database{guid: dotnet.NewGuid(), desc: desc, bolt: bdb, log: logging.New("AstraDatabase")}
	if err := d.initCrypto(); err != nil {
		_ = bdb.Close()
		return nil, err
	}
	meta, err := NewBytedata(desc.Path + ".meta")
	if err != nil {
		_ = bdb.Close()
		return nil, err
	}
	d.meta = meta
	return d, nil
}

func (d *Database) initCrypto() error {
	return d.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(sysBucket)
		if err != nil {
			return err
		}
		salt := b.Get([]byte("salt"))
		check := b.Get([]byte("check"))
		if d.desc.Password == nil {
			if check != nil {
				return errors.New("database is encrypted but no password was configured")
			}
			return nil
		}
		if salt == nil {
			salt = make([]byte, 16)
			if _, err := rand.Read(salt); err != nil {
				return err
			}
			if err := b.Put([]byte("salt"), salt); err != nil {
				return err
			}
		}
		key, err := pbkdf2.Key(sha256.New, *d.desc.Password, salt, 100_000, 32)
		if err != nil {
			return err
		}
		blk, err := aes.NewCipher(key)
		if err != nil {
			return err
		}
		d.aead, err = cipher.NewGCM(blk)
		if err != nil {
			return err
		}
		if check == nil {
			return b.Put([]byte("check"), d.seal([]byte("solara")))
		}
		if p, err := d.open(check); err != nil || string(p) != "solara" {
			// LiteDB: "Invalid password"
			return errors.New("Invalid password")
		}
		return nil
	})
}

func (d *Database) seal(p []byte) []byte {
	if d.aead == nil {
		return p
	}
	nonce := make([]byte, d.aead.NonceSize())
	_, _ = rand.Read(nonce)
	return d.aead.Seal(nonce, nonce, p, nil)
}

func (d *Database) open(c []byte) ([]byte, error) {
	if d.aead == nil {
		return c, nil
	}
	n := d.aead.NonceSize()
	if len(c) < n {
		return nil, errors.New("corrupt document")
	}
	return d.aead.Open(nil, c[:n], c[n:], nil)
}

// Close releases the file.
func (d *Database) Close() error { return d.bolt.Close() }

// GetGuid mirrors IDatabase.GetGuid.
func (d *Database) GetGuid() string { return d.guid }

// Descriptor mirrors IDatabase.Descriptor.
func (d *Database) Descriptor() *Descriptor { return d.desc }

// Meta exposes the collection metadata.
func (d *Database) Meta() *Bytedata { return d.meta }

// CollectionName mirrors AstraDatabase.CreateCollectionName<T>():
// "_" + first 7 hex chars of MD5(type name) + "_" + lower(type name).
func CollectionName(typeName string) string {
	h := md5.Sum([]byte(typeName))
	return "_" + fmt.Sprintf("%x", h)[:7] + "_" + strings.ToLower(typeName)
}

// Named lets a model declare its original CLR type name and full name when
// they differ from the Go struct name.
type Named interface {
	DotnetTypeName() string
	DotnetFullName() string
}

var (
	fullNamesMu sync.RWMutex
	fullNames   = map[reflect.Type]string{}
)

// SetFullName records the original CLR full name of a model type (written to
// the .meta sidecar as CollectionType, like typeof(T).FullName).
func SetFullName[T any](full string) {
	fullNamesMu.Lock()
	defer fullNamesMu.Unlock()
	fullNames[reflect.TypeOf((*T)(nil)).Elem()] = full
}

func typeNames(t reflect.Type) (string, string) {
	if n, ok := reflect.New(t).Interface().(Named); ok {
		return n.DotnetTypeName(), n.DotnetFullName()
	}
	fullNamesMu.RLock()
	defer fullNamesMu.RUnlock()
	if f, ok := fullNames[t]; ok {
		return t.Name(), f
	}
	return t.Name(), t.Name()
}

// DropCollection mirrors IDatabase.DropCollection<T>.
func DropCollection[T any](d *Database) {
	name, _ := typeNames(reflect.TypeOf((*T)(nil)).Elem())
	cname := CollectionName(name)
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.meta.Has(cname) {
		d.log.Warning("Failed to drop collection -- the database did not contain any collections for type '{Type}'", name)
		return
	}
	_ = d.bolt.Update(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte(cname)) != nil {
			return tx.DeleteBucket([]byte(cname))
		}
		return nil
	})
	d.meta.Remove(cname)
	d.meta.Save()
}
