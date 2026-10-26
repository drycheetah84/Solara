// Package pak reads, verifies and rewrites Unreal Engine .pak files.
//
// Version 8 (PakFile_Version_FNameBasedCompressionMethod) with the legacy
// index layout and version 11 (PakFile_Version_Fnv64BugFix) with the
// encoded-entry / path-hash / full-directory index layout are supported.
// Other versions are recognised by their footer so the version can be
// reported, but are refused with *UnsupportedVersionError. Oodle-compressed
// v11 entries are read through a pluggable Decompressor (see Oodle).
//
// The writer (Rebuild) is lossless: rebuilding a pak with no replacements
// produces a byte-identical file. Entries keep their index order, their
// physical (on-disk) order — which in real paks differs from index order —
// and any gap bytes between entry regions, between the last entry and the
// index, and between the index and the footer are copied verbatim.
package pak

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf16"
)

// Magic is the pak footer magic number.
const Magic uint32 = 0x5A6F12E1

// SupportedVersion is the legacy-index pak version (see also
// SupportedVersions).
const SupportedVersion = 8

// Entry flag bits (FPakEntry::Flags).
const (
	FlagEncrypted uint8 = 0x01
	FlagDeleted   uint8 = 0x02
)

// ErrNotPak is returned when no valid pak footer is found.
var ErrNotPak = errors.New("not an Unreal pak file (footer magic not found)")

// UnsupportedVersionError is returned by Open for recognised paks whose
// version (or layout) this package cannot parse.
type UnsupportedVersionError struct {
	Version uint32
	Reason  string
}

func (e *UnsupportedVersionError) Error() string {
	if e.Reason != "" {
		return fmt.Sprintf("unsupported pak version %d: %s (supported versions: 8, 11)", e.Version, e.Reason)
	}
	return fmt.Sprintf("unsupported pak version %d (supported versions: 8, 11)", e.Version)
}

// Footer is the parsed pak trailer (FPakInfo).
type Footer struct {
	Version           uint32
	Size              int // total footer size in bytes
	EncryptionKeyGUID [16]byte
	EncryptedIndex    bool
	IndexOffset       int64
	IndexSize         int64
	IndexHash         [20]byte
	CompressionNames  []string

	magicPos int    // offset of the magic within the footer
	raw      []byte // raw footer bytes
}

// footerLayouts lists the known footer sizes with the offset of the magic.
//
//	44  : v1-v3  Magic Version Offset Size Hash
//	45  : v4-v6  bEncryptedIndex + above
//	61  : v7     EncryptionKeyGuid + above
//	189 : v8 (UE 4.22) + 4 compression names
//	221 : v8 (4.23+), v10, v11 + 5 compression names
//	222 : v9 + bIndexIsFrozen
var footerLayouts = []struct{ size, magicPos, names int }{
	{221, 17, 5},
	{222, 17, 5},
	{189, 17, 4},
	{61, 17, 0},
	{45, 1, 0},
	{44, 0, 0},
}

// ReadFooter locates and parses the pak footer of f (whose size is size).
func ReadFooter(f io.ReaderAt, size int64) (*Footer, error) {
	const maxFooter = 222
	n := int64(maxFooter)
	if size < n {
		n = size
	}
	tail := make([]byte, n)
	if _, err := f.ReadAt(tail, size-n); err != nil && err != io.EOF {
		return nil, fmt.Errorf("read footer: %w", err)
	}
	var fallback *Footer
	for _, l := range footerLayouts {
		if int64(l.size) > size {
			continue
		}
		raw := tail[len(tail)-l.size:]
		if binary.LittleEndian.Uint32(raw[l.magicPos:]) != Magic {
			continue
		}
		ft := &Footer{Size: l.size, magicPos: l.magicPos, raw: append([]byte(nil), raw...)}
		m := l.magicPos
		if m == 17 {
			copy(ft.EncryptionKeyGUID[:], raw[0:16])
		}
		if m >= 1 {
			ft.EncryptedIndex = raw[m-1] != 0
		}
		ft.Version = binary.LittleEndian.Uint32(raw[m+4:])
		ft.IndexOffset = int64(binary.LittleEndian.Uint64(raw[m+8:]))
		ft.IndexSize = int64(binary.LittleEndian.Uint64(raw[m+16:]))
		copy(ft.IndexHash[:], raw[m+24:m+44])
		p := m + 44
		if l.size == 222 {
			p++ // bIndexIsFrozen
		}
		for i := 0; i < l.names; i++ {
			name := raw[p : p+32]
			if j := bytes.IndexByte(name, 0); j >= 0 {
				name = name[:j]
			}
			ft.CompressionNames = append(ft.CompressionNames, string(name))
			p += 32
		}
		if footerPlausible(ft, l.size) {
			return ft, nil
		}
		if fallback == nil {
			fallback = ft
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, ErrNotPak
}

// footerPlausible checks that the version matches the footer size.
func footerPlausible(ft *Footer, size int) bool {
	v := ft.Version
	switch size {
	case 44:
		return v >= 1 && v <= 3
	case 45:
		return v >= 4 && v <= 6
	case 61:
		return v == 7
	case 189:
		return v == 8
	case 221:
		return v == 8 || v >= 10
	case 222:
		return v == 9
	}
	return false
}

// Block is a compression block, offsets relative to the entry offset (v5+).
type Block struct {
	Start, End int64
}

// Entry is one FPakEntry from the index.
type Entry struct {
	Name                 string
	Offset               int64
	Size                 int64
	UncompressedSize     int64
	CompressionMethod    uint32
	Hash                 [20]byte
	Blocks               []Block
	Flags                uint8
	CompressionBlockSize uint32

	// IndexPos is the position of the entry in the index (0-based). For v11
	// paks this is the order of the full directory index.
	IndexPos int
	rawName  []byte // serialized FString, preserved byte-for-byte

	v11               bool   // entry of a v10+ pak
	replaceCompressed bool   // compressed, but may be replaced (v11)
	encLow            uint32 // encoded block-size bits as stored
	encExt            uint32 // escaped block size (encLow == 0x3f)
	encFresh          bool   // recompute encLow/encExt when encoding
}

// Compressed reports whether the entry uses a compression method.
func (e *Entry) Compressed() bool { return e.CompressionMethod != 0 }

// Encrypted reports whether the entry data is encrypted.
func (e *Entry) Encrypted() bool { return e.Flags&FlagEncrypted != 0 }

// Editable reports whether the entry can be replaced by Rebuild: v8
// entries must be uncompressed and unencrypted; v11 entries must be
// unencrypted (compressed ones are replaced uncompressed or recompressed).
func (e *Entry) Editable() bool {
	return !e.Encrypted() && (!e.Compressed() || e.replaceCompressed)
}

// HeaderSize is the size of the serialized FPakEntry (inline header).
func (e *Entry) HeaderSize() int64 {
	n := int64(8 + 8 + 8 + 4 + 20 + 1 + 4)
	if e.Compressed() {
		n += 4 + int64(len(e.Blocks))*16
	}
	return n
}

// dataLen is the number of payload bytes stored on disk after the header.
func (e *Entry) dataLen() int64 {
	if e.Compressed() && len(e.Blocks) > 0 {
		var end int64
		for _, b := range e.Blocks {
			if b.End > end {
				end = b.End
			}
		}
		if d := end - e.HeaderSize(); d >= e.Size {
			return d
		}
	}
	if e.Encrypted() {
		return align16(e.Size)
	}
	return e.Size
}

// RegionSize is the total on-disk size of the entry (header + data).
func (e *Entry) RegionSize() int64 { return e.HeaderSize() + e.dataLen() }

func align16(n int64) int64 { return (n + 15) &^ 15 }

// Pak is a parsed pak file. It holds no open file handle.
type Pak struct {
	Path       string
	FileSize   int64
	Footer     *Footer
	MountPoint string
	Entries    []*Entry // index order
	// Decompressor decodes compressed entries in ReadEntry (nil: reading a
	// compressed entry fails with ErrOodleRequired).
	Decompressor Decompressor

	rawMount     []byte
	indexTrailer []byte // unparsed bytes after the entries (normally none)
	byName       map[string]*Entry
	dupNames     map[string]bool
	v11          *indexV11 // v10+ index structures (nil for v8)
}

// Open parses the pak at path.
func Open(path string) (*Pak, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return parse(f, st.Size(), path)
}

func parse(f io.ReaderAt, size int64, path string) (*Pak, error) {
	ft, err := ReadFooter(f, size)
	if err != nil {
		return nil, err
	}
	if !versionSupported(ft.Version) {
		return nil, &UnsupportedVersionError{Version: ft.Version}
	}
	if ft.EncryptedIndex {
		return nil, &UnsupportedVersionError{Version: ft.Version, Reason: "encrypted index"}
	}
	footerStart := size - int64(ft.Size)
	if ft.IndexOffset < 0 || ft.IndexSize <= 0 || ft.IndexOffset+ft.IndexSize > footerStart {
		return nil, fmt.Errorf("corrupt footer: index [%d,+%d) outside file data (footer at %d)", ft.IndexOffset, ft.IndexSize, footerStart)
	}
	if ft.IndexSize > 1<<30 {
		return nil, fmt.Errorf("index too large (%d bytes)", ft.IndexSize)
	}
	idx := make([]byte, ft.IndexSize)
	if _, err := f.ReadAt(idx, ft.IndexOffset); err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	if sha1.Sum(idx) != ft.IndexHash {
		return nil, errors.New("index hash mismatch (index is corrupt)")
	}
	p := &Pak{Path: path, FileSize: size, Footer: ft, byName: map[string]*Entry{}, dupNames: map[string]bool{}}
	if ft.Version >= VersionPathHashIndex {
		return parseV11Pak(p, f, idx)
	}
	r := &reader{b: idx}
	p.MountPoint, p.rawMount = r.fstring()
	count := r.i32()
	if r.err == nil && (count < 0 || int64(count) > ft.IndexSize/50) {
		return nil, fmt.Errorf("corrupt index: entry count %d", count)
	}
	for i := 0; i < int(count) && r.err == nil; i++ {
		e := &Entry{IndexPos: i}
		e.Name, e.rawName = r.fstring()
		readEntry(r, e)
		if r.err != nil {
			break
		}
		p.Entries = append(p.Entries, e)
		if _, ok := p.byName[e.Name]; ok {
			p.dupNames[e.Name] = true
		} else {
			p.byName[e.Name] = e
		}
	}
	if r.err != nil {
		return nil, fmt.Errorf("corrupt index: %w", r.err)
	}
	p.indexTrailer = append([]byte(nil), idx[r.p:]...)
	// The writer re-serialises the index; refuse paks where that would not
	// reproduce the original bytes.
	if !bytes.Equal(p.serializeIndex(nil), idx) {
		return nil, errors.New("index re-serialisation is not lossless; refusing to handle this pak")
	}
	for _, e := range p.Entries {
		if e.Offset < 0 || e.Offset+e.RegionSize() > ft.IndexOffset {
			return nil, fmt.Errorf("entry %q region [%d,+%d) lies outside the data area", e.Name, e.Offset, e.RegionSize())
		}
	}
	return p, nil
}

func readEntry(r *reader, e *Entry) {
	e.Offset = r.i64()
	e.Size = r.i64()
	e.UncompressedSize = r.i64()
	e.CompressionMethod = r.u32()
	r.bytes(e.Hash[:])
	if e.CompressionMethod != 0 {
		n := r.i32()
		if n < 0 || n > 1<<24 {
			r.fail(fmt.Errorf("bad block count %d", n))
			return
		}
		e.Blocks = make([]Block, n)
		for i := range e.Blocks {
			e.Blocks[i].Start = r.i64()
			e.Blocks[i].End = r.i64()
		}
	}
	e.Flags = r.u8()
	e.CompressionBlockSize = r.u32()
}

// Entry returns the entry with the given name.
func (p *Pak) Entry(name string) (*Entry, bool) {
	e, ok := p.byName[name]
	return e, ok
}

// serializeIndex writes the index; offsets overrides entry offsets when non-nil.
func (p *Pak) serializeIndex(offsets []int64) []byte {
	var b bytes.Buffer
	b.Write(p.rawMount)
	putU32(&b, uint32(len(p.Entries)))
	for i, e := range p.Entries {
		b.Write(e.rawName)
		off := e.Offset
		if offsets != nil {
			off = offsets[i]
		}
		writeEntry(&b, e, off)
	}
	b.Write(p.indexTrailer)
	return b.Bytes()
}

// writeEntry serialises an FPakEntry with the given offset.
func writeEntry(b *bytes.Buffer, e *Entry, offset int64) {
	putU64(b, uint64(offset))
	putU64(b, uint64(e.Size))
	putU64(b, uint64(e.UncompressedSize))
	putU32(b, e.CompressionMethod)
	b.Write(e.Hash[:])
	if e.CompressionMethod != 0 {
		putU32(b, uint32(len(e.Blocks)))
		for _, bl := range e.Blocks {
			putU64(b, uint64(bl.Start))
			putU64(b, uint64(bl.End))
		}
	}
	b.WriteByte(e.Flags)
	putU32(b, e.CompressionBlockSize)
}

func putU32(b *bytes.Buffer, v uint32) {
	var t [4]byte
	binary.LittleEndian.PutUint32(t[:], v)
	b.Write(t[:])
}

func putU64(b *bytes.Buffer, v uint64) {
	var t [8]byte
	binary.LittleEndian.PutUint64(t[:], v)
	b.Write(t[:])
}

// encodeFString encodes s as an Unreal FString (ANSI if possible, else UTF-16).
func encodeFString(s string) []byte {
	var b bytes.Buffer
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		putU32(&b, uint32(len(s)+1))
		b.WriteString(s)
		b.WriteByte(0)
		return b.Bytes()
	}
	u := utf16.Encode([]rune(s))
	putU32(&b, uint32(-int32(len(u)+1)))
	for _, c := range u {
		b.WriteByte(byte(c))
		b.WriteByte(byte(c >> 8))
	}
	b.Write([]byte{0, 0})
	return b.Bytes()
}

// reader is a bounds-checked little-endian cursor over a byte slice.
type reader struct {
	b   []byte
	p   int
	err error
}

func (r *reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
}

func (r *reader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || r.p+n > len(r.b) {
		r.fail(fmt.Errorf("unexpected end of data at %d (need %d bytes)", r.p, n))
		return nil
	}
	s := r.b[r.p : r.p+n]
	r.p += n
	return s
}

func (r *reader) u8() uint8 {
	if s := r.take(1); s != nil {
		return s[0]
	}
	return 0
}

func (r *reader) u32() uint32 {
	if s := r.take(4); s != nil {
		return binary.LittleEndian.Uint32(s)
	}
	return 0
}

func (r *reader) i32() int32 { return int32(r.u32()) }

func (r *reader) i64() int64 {
	if s := r.take(8); s != nil {
		return int64(binary.LittleEndian.Uint64(s))
	}
	return 0
}

func (r *reader) bytes(dst []byte) {
	if s := r.take(len(dst)); s != nil {
		copy(dst, s)
	}
}

// fstring reads an FString, returning the decoded value and its raw bytes.
func (r *reader) fstring() (string, []byte) {
	start := r.p
	n := r.i32()
	if r.err != nil {
		return "", nil
	}
	var s string
	switch {
	case n == 0:
	case n > 0:
		if n > 1<<20 {
			r.fail(fmt.Errorf("string too long (%d)", n))
			return "", nil
		}
		d := r.take(int(n))
		if d == nil {
			return "", nil
		}
		s = string(bytes.TrimRight(d, "\x00"))
	default:
		if n < -(1 << 20) {
			r.fail(fmt.Errorf("string too long (%d)", n))
			return "", nil
		}
		d := r.take(int(-n) * 2)
		if d == nil {
			return "", nil
		}
		u := make([]uint16, 0, -n)
		for i := 0; i+1 < len(d); i += 2 {
			c := uint16(d[i]) | uint16(d[i+1])<<8
			if c == 0 {
				break
			}
			u = append(u, c)
		}
		s = string(utf16.Decode(u))
	}
	return s, append([]byte(nil), r.b[start:r.p]...)
}

// readInlineHeader parses the inline FPakEntry stored at e.Offset.
func readInlineHeader(f io.ReaderAt, e *Entry) (*Entry, error) {
	buf := make([]byte, e.HeaderSize())
	if _, err := f.ReadAt(buf, e.Offset); err != nil {
		return nil, err
	}
	r := &reader{b: buf}
	h := &Entry{Name: e.Name}
	readEntry(r, h)
	if r.err != nil {
		return nil, r.err
	}
	return h, nil
}

// headerMatches compares an inline header against an index entry.
func headerMatches(h, e *Entry) error {
	switch {
	case h.Offset != 0:
		return fmt.Errorf("inline header offset is %d, expected 0", h.Offset)
	case h.Size != e.Size || h.UncompressedSize != e.UncompressedSize:
		return fmt.Errorf("inline header sizes %d/%d differ from index %d/%d", h.Size, h.UncompressedSize, e.Size, e.UncompressedSize)
	case h.CompressionMethod != e.CompressionMethod:
		return errors.New("inline header compression method differs from index")
	case h.Hash != e.Hash:
		return errors.New("inline header hash differs from index")
	case h.Flags != e.Flags:
		return errors.New("inline header flags differ from index")
	case h.CompressionBlockSize != e.CompressionBlockSize:
		return errors.New("inline header compression block size differs from index")
	case len(h.Blocks) != len(e.Blocks):
		return errors.New("inline header block count differs from index")
	}
	for i := range h.Blocks {
		if h.Blocks[i] != e.Blocks[i] {
			return errors.New("inline header blocks differ from index")
		}
	}
	return nil
}

// ReadEntry returns the data of an unencrypted entry after verifying its
// SHA-1 against the index. Compressed entries (v11 only) are decompressed
// with p.Decompressor.
func (p *Pak) ReadEntry(name string) ([]byte, error) {
	e, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("entry %q not found in pak", name)
	}
	if e.Compressed() && !e.v11 {
		return nil, fmt.Errorf("entry %q is compressed; reading compressed entries is not supported", name)
	}
	if e.Encrypted() {
		return nil, fmt.Errorf("entry %q is encrypted; reading encrypted entries is not supported", name)
	}
	if e.Size > 1<<30 {
		return nil, fmt.Errorf("entry %q too large (%d bytes)", name, e.Size)
	}
	f, err := os.Open(p.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h, err := readInlineHeader(f, e)
	if err != nil {
		return nil, fmt.Errorf("entry %q: read inline header: %w", name, err)
	}
	if err := headerMatches(h, e); err != nil {
		return nil, fmt.Errorf("entry %q: %w", name, err)
	}
	data := make([]byte, e.Size)
	if _, err := f.ReadAt(data, e.Offset+e.HeaderSize()); err != nil {
		return nil, fmt.Errorf("entry %q: read data: %w", name, err)
	}
	if sha1.Sum(data) != e.Hash {
		return nil, fmt.Errorf("entry %q: SHA-1 mismatch (data is corrupt)", name)
	}
	if e.Compressed() {
		return p.decompressStored(e, data, p.Decompressor)
	}
	return data, nil
}
