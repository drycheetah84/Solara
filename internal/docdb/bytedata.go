package docdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sync"

	"solara/internal/dotnet"
	"solara/internal/logging"
)

// CollectionMetadata mirrors AstraDatabaseCollectionMetadata.
type CollectionMetadata struct {
	CollectionName string
	CollectionType string
	MaxItems       int32
}

// Bytedata mirrors AstraDatabaseBytedata: the "<db>.meta" sidecar written
// with System.IO.BinaryWriter (7-bit length-prefixed UTF-8 strings,
// little-endian Int32/Int64).
type Bytedata struct {
	mu             sync.Mutex
	CompletePath   string
	Collections    []CollectionMetadata
	CreatedAt      dotnet.DateTime
	LastModifiedAt dotnet.DateTime
	log            *logging.Logger
}

// NewBytedata mirrors the constructor + Initialize(): create the file if
// missing, otherwise read it.
func NewBytedata(path string) (*Bytedata, error) {
	b := &Bytedata{CompletePath: path, CreatedAt: dotnet.UtcNow(), LastModifiedAt: dotnet.UtcNow(), log: logging.New("AstraDatabaseBytedata")}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return b, os.WriteFile(path, b.encode(), 0o600)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := b.decode(raw); err != nil {
		return nil, err
	}
	return b, nil
}

func write7(w *bytes.Buffer, s string) {
	n := uint32(len(s))
	for n >= 0x80 {
		w.WriteByte(byte(n) | 0x80)
		n >>= 7
	}
	w.WriteByte(byte(n))
	w.WriteString(s)
}

func read7(r *bytes.Reader) (string, error) {
	var n uint32
	for shift := 0; shift < 35; shift += 7 {
		c, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		n |= uint32(c&0x7f) << shift
		if c&0x80 == 0 {
			buf := make([]byte, n)
			_, err := io.ReadFull(r, buf)
			return string(buf), err
		}
	}
	return "", errors.New("bad 7-bit length")
}

func (b *Bytedata) encode() []byte {
	var w bytes.Buffer
	write7(&w, b.CompletePath)
	_ = binary.Write(&w, binary.LittleEndian, int32(len(b.Collections)))
	for _, c := range b.Collections {
		write7(&w, c.CollectionName)
		write7(&w, c.CollectionType)
		_ = binary.Write(&w, binary.LittleEndian, c.MaxItems)
	}
	_ = binary.Write(&w, binary.LittleEndian, b.CreatedAt.Ticks())
	_ = binary.Write(&w, binary.LittleEndian, b.LastModifiedAt.Ticks())
	return w.Bytes()
}

func (b *Bytedata) decode(raw []byte) error {
	r := bytes.NewReader(raw)
	var err error
	if b.CompletePath, err = read7(r); err != nil {
		return err
	}
	var count int32
	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		return err
	}
	b.Collections = make([]CollectionMetadata, 0, count)
	for i := int32(0); i < count; i++ {
		var c CollectionMetadata
		if c.CollectionName, err = read7(r); err != nil {
			return err
		}
		if c.CollectionType, err = read7(r); err != nil {
			return err
		}
		if err := binary.Read(r, binary.LittleEndian, &c.MaxItems); err != nil {
			return err
		}
		b.Collections = append(b.Collections, c)
	}
	var ct, mt int64
	if err := binary.Read(r, binary.LittleEndian, &ct); err != nil {
		return err
	}
	if err := binary.Read(r, binary.LittleEndian, &mt); err != nil {
		return err
	}
	b.CreatedAt, b.LastModifiedAt = dotnet.FromTicks(ct, dotnet.Utc), dotnet.FromTicks(mt, dotnet.Utc)
	return nil
}

// Has reports whether a collection is registered.
func (b *Bytedata) Has(name string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.Collections {
		if c.CollectionName == name {
			return true
		}
	}
	return false
}

// Add registers a collection (MaxItems 100000, as the original).
func (b *Bytedata) Add(name, typ string) {
	b.mu.Lock()
	b.Collections = append(b.Collections, CollectionMetadata{name, typ, 100000})
	b.mu.Unlock()
}

// Remove unregisters a collection.
func (b *Bytedata) Remove(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.Collections[:0]
	for _, c := range b.Collections {
		if c.CollectionName != name {
			out = append(out, c)
		}
	}
	b.Collections = out
}

// Save mirrors AstraDatabaseBytedata.Save (no-op with a warning when the
// file has disappeared).
func (b *Bytedata) Save() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, err := os.Stat(b.CompletePath); err != nil {
		b.log.Warning("Cannot save database because the metadata does not exist.")
		return
	}
	_ = os.WriteFile(b.CompletePath, b.encode(), 0o600)
}
