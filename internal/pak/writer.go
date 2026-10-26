package pak

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"os"
)

// NewFile describes an entry for WriteNew.
type NewFile struct {
	Name string
	Data []byte
	// CompressionMethod != 0 marks the entry as compressed; Data is then
	// stored as-is as a single "compressed" block (used to build test paks).
	CompressionMethod uint32
	// Encrypted sets the encrypted flag; Data is stored padded to 16 bytes
	// as-is (no real encryption is performed).
	Encrypted bool
	// CompressionBlockSize is written verbatim.
	CompressionBlockSize uint32
}

// WriteOptions tunes WriteNew's physical layout (useful for tests).
type WriteOptions struct {
	// PhysicalOrder lists index positions in on-disk order; nil = index order.
	PhysicalOrder []int
	// GapBefore inserts filler bytes before the entry at the given index
	// position.
	GapBefore map[int]int
	// GapBeforeIndex inserts filler bytes between the last entry and the index.
	GapBeforeIndex int
	// CompressionNames fills the footer's compression-method name slots.
	CompressionNames []string
	// Version selects the pak version: 0 or 8 (legacy index) or 11.
	Version uint32
	// Compressor (v11) really compresses entries with CompressionMethod != 0
	// using the footer method name; without it the data is stored as-is as
	// one fake "compressed" block.
	Compressor Compressor
	// EncodedOrder (v11) lists index positions in encoded-blob order; nil =
	// index order.
	EncodedOrder []int
	// NonEncoded (v11) stores these index positions as full FPakEntry
	// records in the primary index (negative locations).
	NonEncoded map[int]bool
	// PathHashSeed (v11) is the path hash seed.
	PathHashSeed uint64
}

// WriteNew creates a version 8 pak (legacy index, 5 compression slots), or
// a version 11 pak when opts.Version is 11.
func WriteNew(path, mountPoint string, files []NewFile, opts *WriteOptions) error {
	if opts == nil {
		opts = &WriteOptions{}
	}
	if opts.Version == VersionFnv64BugFix {
		return writeNewV11(path, mountPoint, files, opts)
	}
	if opts.Version != 0 && opts.Version != SupportedVersion {
		return fmt.Errorf("cannot write pak version %d", opts.Version)
	}
	order := opts.PhysicalOrder
	if order == nil {
		for i := range files {
			order = append(order, i)
		}
	}
	if len(order) != len(files) {
		return fmt.Errorf("physical order has %d items for %d files", len(order), len(files))
	}
	entries := make([]*Entry, len(files))
	var body bytes.Buffer
	for _, i := range order {
		nf := files[i]
		for g := 0; g < opts.GapBefore[i]; g++ {
			body.WriteByte(byte(0xA0 + g%16))
		}
		e := &Entry{
			Name:                 nf.Name,
			Offset:               int64(body.Len()),
			Size:                 int64(len(nf.Data)),
			UncompressedSize:     int64(len(nf.Data)),
			CompressionMethod:    nf.CompressionMethod,
			Hash:                 sha1.Sum(nf.Data),
			CompressionBlockSize: nf.CompressionBlockSize,
			IndexPos:             i,
			rawName:              encodeFString(nf.Name),
		}
		data := nf.Data
		if nf.Encrypted {
			e.Flags |= FlagEncrypted
			data = append(append([]byte(nil), data...), make([]byte, align16(int64(len(data)))-int64(len(data)))...)
		}
		if e.Compressed() {
			e.UncompressedSize = int64(len(nf.Data)) * 2
			e.Blocks = []Block{{}}
			hs := e.HeaderSize()
			e.Blocks[0] = Block{Start: hs, End: hs + int64(len(data))}
		}
		writeEntry(&body, e, 0)
		body.Write(data)
		entries[i] = e
	}
	for g := 0; g < opts.GapBeforeIndex; g++ {
		body.WriteByte(0xEE)
	}
	p := &Pak{rawMount: encodeFString(mountPoint), Entries: entries}
	index := p.serializeIndex(nil)
	indexOffset := int64(body.Len())
	body.Write(index)

	footer := make([]byte, 221)
	binary.LittleEndian.PutUint32(footer[17:], Magic)
	binary.LittleEndian.PutUint32(footer[21:], SupportedVersion)
	binary.LittleEndian.PutUint64(footer[25:], uint64(indexOffset))
	binary.LittleEndian.PutUint64(footer[33:], uint64(len(index)))
	h := sha1.Sum(index)
	copy(footer[41:61], h[:])
	for i, n := range opts.CompressionNames {
		if i < 5 {
			copy(footer[61+32*i:61+32*i+31], n)
		}
	}
	body.Write(footer)
	return os.WriteFile(path, body.Bytes(), 0o644)
}
