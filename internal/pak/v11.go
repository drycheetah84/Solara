package pak

// Pak version 11 (PakFile_Version_Fnv64BugFix, UE 4.26+/UE5) index support.
//
// Layout (all little-endian):
//
//	primary index : FString MountPoint, i32 NumEntries, u64 PathHashSeed,
//	                u32 bHasPathHashIndex [i64 off, i64 size, [20]sha1],
//	                u32 bHasFullDirectoryIndex [i64 off, i64 size, [20]sha1],
//	                i32 EncodedEntriesSize + bytes,
//	                i32 NumNonEncoded + FPakEntry[] (legacy serialisation),
//	                trailer bytes (kept verbatim)
//	path hash idx : i32 N, N x {u64 FNV64(lower(path)), i32 location},
//	                then the pruned directory index (same shape as below)
//	full dir idx  : i32 NumDirs, per dir FString name, i32 NumFiles,
//	                per file FString name, i32 location
//
// A location >= 0 is a byte offset into the encoded-entries blob; a
// negative location -(i+1) refers to the i-th non-encoded entry.
//
// The parser keeps every structure in its stored order so that
// re-serialising an unmodified pak reproduces the original bytes exactly;
// Open refuses paks for which that is not the case.

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf16"
)

// Pak versions with the v10+ index layout.
const (
	VersionPathHashIndex = 10 // PakFile_Version_PathHashIndex
	VersionFnv64BugFix   = 11 // PakFile_Version_Fnv64BugFix
)

// SupportedVersions lists the pak versions this package can parse and write.
var SupportedVersions = []uint32{SupportedVersion, VersionFnv64BugFix}

func versionSupported(v uint32) bool {
	for _, s := range SupportedVersions {
		if v == s {
			return true
		}
	}
	return false
}

type dirFile struct {
	rawName []byte
	name    string
	entry   *Entry
}

type dirRec struct {
	rawName []byte
	name    string
	files   []dirFile
}

type phiRec struct {
	hash  uint64
	entry *Entry
}

// region is a contiguous index structure after the entry data.
type region struct {
	kind      int // regionPrimary, regionPHI, regionFDI
	off, size int64
}

const (
	regionPrimary = iota
	regionPHI
	regionFDI
)

type indexV11 struct {
	rawCount  int32
	seed      uint64
	hasPHIRaw uint32
	phiOff    int64
	phiSize   int64
	phiHash   [20]byte
	hasFDIRaw uint32
	fdiOff    int64
	fdiSize   int64
	fdiHash   [20]byte

	encOrder []*Entry // entries in the encoded blob, blob order
	nonEnc   []*Entry // entries stored as full FPakEntry records
	trailer  []byte   // primary index bytes after the non-encoded entries

	fdi       []dirRec
	phi       []phiRec
	pruned    []dirRec
	hasPruned bool
}

// PathHashSeed returns the v10+ path hash seed (0 for older versions).
func (p *Pak) PathHashSeed() uint64 {
	if p.v11 == nil {
		return 0
	}
	return p.v11.seed
}

// HashPath implements FPakFile::HashPath for version 11+: FNV-1a-64 over the
// UTF-16LE bytes of the lower-cased mount-relative path, with the offset
// basis increased by seed.
func HashPath(path string, seed uint64) uint64 {
	h := uint64(0xcbf29ce484222325) + seed
	for _, c := range utf16.Encode([]rune(strings.ToLower(path))) {
		h ^= uint64(c & 0xff)
		h *= 0x100000001b3
		h ^= uint64(c >> 8)
		h *= 0x100000001b3
	}
	return h
}

// joinPakPath combines a directory-index directory and file name.
func joinPakPath(dir, file string) string {
	if dir == "/" || dir == "" {
		return file
	}
	if strings.HasSuffix(dir, "/") {
		return dir + file
	}
	return dir + "/" + file
}

// decodeEntry reads one encoded FPakEntry (FPakFile::DecodePakEntry).
func decodeEntry(r *reader, e *Entry) {
	v := r.u32()
	low := v & 0x3f
	cbs := low << 11
	if low == 0x3f {
		e.encExt = r.u32()
		cbs = e.encExt
	}
	e.encLow = low
	e.CompressionMethod = (v >> 23) & 0x3f
	if v&(1<<31) != 0 {
		e.Offset = int64(r.u32())
	} else {
		e.Offset = r.i64()
	}
	if v&(1<<30) != 0 {
		e.UncompressedSize = int64(r.u32())
	} else {
		e.UncompressedSize = r.i64()
	}
	if e.CompressionMethod != 0 {
		if v&(1<<29) != 0 {
			e.Size = int64(r.u32())
		} else {
			e.Size = r.i64()
		}
	} else {
		e.Size = e.UncompressedSize
	}
	if v&(1<<22) != 0 {
		e.Flags |= FlagEncrypted
	}
	nb := int((v >> 6) & 0xffff)
	if r.err != nil {
		return
	}
	if nb > 0 && e.CompressionMethod == 0 {
		r.fail(errors.New("uncompressed encoded entry with compression blocks"))
		return
	}
	e.Blocks = make([]Block, nb)
	if e.CompressionMethod == 0 {
		e.Blocks = nil
	}
	if nb > 0 {
		e.CompressionBlockSize = cbs
		if nb == 1 {
			e.CompressionBlockSize = uint32(e.UncompressedSize)
		}
	}
	hs := e.HeaderSize()
	if nb == 1 && !e.Encrypted() {
		e.Blocks[0] = Block{Start: hs, End: hs + e.Size}
	} else if nb > 0 {
		align := int64(1)
		if e.Encrypted() {
			align = 16
		}
		cur := hs
		for i := range e.Blocks {
			sz := int64(r.u32())
			e.Blocks[i] = Block{Start: cur, End: cur + sz}
			cur += (sz + align - 1) / align * align
		}
	}
}

// encodeEntry writes e in the encoded format with the given offset
// (FPakFile::EncodePakEntry). It fails for entries that cannot be encoded.
func encodeEntry(b *bytes.Buffer, e *Entry, offset int64) error {
	nb := len(e.Blocks)
	if nb > 0xffff {
		return fmt.Errorf("entry %q has %d compression blocks (max 65535 in the encoded index)", e.Name, nb)
	}
	if e.CompressionMethod > 0x3f {
		return fmt.Errorf("entry %q compression method index %d does not fit the encoded index", e.Name, e.CompressionMethod)
	}
	if e.CompressionMethod == 0 && nb > 0 {
		return fmt.Errorf("entry %q is uncompressed but has compression blocks", e.Name)
	}
	if offset < 0 || e.Size < 0 || e.UncompressedSize < 0 {
		return fmt.Errorf("entry %q has negative offset or size", e.Name)
	}
	low, ext := e.encLow, e.encExt
	if e.encFresh {
		low, ext = encodedBlockSize(e)
	}
	off32 := offset <= math.MaxUint32
	usize32 := e.UncompressedSize <= math.MaxUint32
	size32 := e.Size <= math.MaxUint32
	v := low | uint32(nb)<<6 | e.CompressionMethod<<23
	if e.Encrypted() {
		v |= 1 << 22
	}
	if off32 {
		v |= 1 << 31
	}
	if usize32 {
		v |= 1 << 30
	}
	if size32 {
		v |= 1 << 29
	}
	putU32(b, v)
	if low == 0x3f {
		putU32(b, ext)
	}
	put := func(is32 bool, n int64) {
		if is32 {
			putU32(b, uint32(n))
		} else {
			putU64(b, uint64(n))
		}
	}
	put(off32, offset)
	put(usize32, e.UncompressedSize)
	if e.CompressionMethod != 0 {
		put(size32, e.Size)
		if nb > 1 || (e.Encrypted() && nb > 0) {
			for _, bl := range e.Blocks {
				sz := bl.End - bl.Start
				if sz < 0 || sz > math.MaxUint32 {
					return fmt.Errorf("entry %q has an invalid compression block size %d", e.Name, sz)
				}
				putU32(b, uint32(sz))
			}
		}
	}
	return nil
}

// encodedBlockSize returns the 6-bit compression block size field (and the
// escaped full value when the field is 0x3f) as UnrealPak writes it: 0 for
// entries with at most one block, CompressionBlockSize>>11 when exact.
func encodedBlockSize(e *Entry) (low, ext uint32) {
	if len(e.Blocks) <= 1 {
		return 0, 0
	}
	cbs := e.CompressionBlockSize
	if cbs&0x7ff == 0 && cbs>>11 < 0x3f {
		return cbs >> 11, 0
	}
	return 0x3f, cbs
}

// readDirIndex parses a (full or pruned) directory index.
func readDirIndex(r *reader, loc map[int32]*Entry) []dirRec {
	n := r.i32()
	if r.err == nil && (n < 0 || int(n) > len(r.b)/4) {
		r.fail(fmt.Errorf("bad directory count %d", n))
	}
	var dirs []dirRec
	for i := 0; i < int(n) && r.err == nil; i++ {
		var d dirRec
		d.name, d.rawName = r.fstring()
		nf := r.i32()
		if r.err == nil && (nf < 0 || int(nf) > len(r.b)/8) {
			r.fail(fmt.Errorf("bad file count %d in directory %q", nf, d.name))
		}
		for j := 0; j < int(nf) && r.err == nil; j++ {
			var f dirFile
			f.name, f.rawName = r.fstring()
			l := r.i32()
			if r.err != nil {
				break
			}
			e, ok := loc[l]
			if !ok {
				r.fail(fmt.Errorf("directory index entry %q%s points to unknown location %d", d.name, f.name, l))
				break
			}
			f.entry = e
			d.files = append(d.files, f)
		}
		dirs = append(dirs, d)
	}
	return dirs
}

func writeDirIndex(b *bytes.Buffer, dirs []dirRec, loc func(*Entry) int32) {
	putU32(b, uint32(len(dirs)))
	for _, d := range dirs {
		b.Write(d.rawName)
		putU32(b, uint32(len(d.files)))
		for _, f := range d.files {
			b.Write(f.rawName)
			putU32(b, uint32(loc(f.entry)))
		}
	}
}

func readRegion(f io.ReaderAt, off, size, limit int64, what string) ([]byte, error) {
	if off < 0 || size < 0 || off+size > limit {
		return nil, fmt.Errorf("corrupt index: %s [%d,+%d) outside the file", what, off, size)
	}
	if size > 1<<30 {
		return nil, fmt.Errorf("%s too large (%d bytes)", what, size)
	}
	b := make([]byte, size)
	if _, err := f.ReadAt(b, off); err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	return b, nil
}

// parseV11 parses the v10+/v11 index structures of p (primary index idx).
func parseV11(p *Pak, f io.ReaderAt, idx []byte) error {
	ft := p.Footer
	footerStart := p.FileSize - int64(ft.Size)
	x := &indexV11{}
	r := &reader{b: idx}
	p.MountPoint, p.rawMount = r.fstring()
	x.rawCount = r.i32()
	x.seed = uint64(r.i64())
	x.hasPHIRaw = r.u32()
	if x.hasPHIRaw != 0 {
		x.phiOff, x.phiSize = r.i64(), r.i64()
		r.bytes(x.phiHash[:])
	}
	x.hasFDIRaw = r.u32()
	if x.hasFDIRaw != 0 {
		x.fdiOff, x.fdiSize = r.i64(), r.i64()
		r.bytes(x.fdiHash[:])
	}
	encSize := r.i32()
	if r.err == nil && (encSize < 0 || int(encSize) > len(idx)) {
		return fmt.Errorf("corrupt index: encoded entries size %d", encSize)
	}
	blob := r.take(int(encSize))
	nonEnc := r.i32()
	if r.err == nil && (nonEnc < 0 || int64(nonEnc) > int64(len(idx))/50) {
		return fmt.Errorf("corrupt index: non-encoded entry count %d", nonEnc)
	}
	loc := map[int32]*Entry{}
	for i := 0; i < int(nonEnc) && r.err == nil; i++ {
		e := &Entry{v11: true}
		readEntry(r, e)
		x.nonEnc = append(x.nonEnc, e)
		loc[int32(-(i + 1))] = e
	}
	if r.err != nil {
		return fmt.Errorf("corrupt index: %w", r.err)
	}
	x.trailer = append([]byte(nil), idx[r.p:]...)

	br := &reader{b: blob}
	for br.p < len(blob) && br.err == nil {
		start := br.p
		e := &Entry{v11: true}
		decodeEntry(br, e)
		if br.err != nil {
			break
		}
		loc[int32(start)] = e
		x.encOrder = append(x.encOrder, e)
	}
	if br.err != nil {
		return fmt.Errorf("corrupt encoded entries: %w", br.err)
	}

	if x.hasFDIRaw == 0 {
		return &UnsupportedVersionError{Version: ft.Version, Reason: "pak has no full directory index (entry names unavailable)"}
	}
	fdiB, err := readRegion(f, x.fdiOff, x.fdiSize, footerStart, "full directory index")
	if err != nil {
		return err
	}
	if sha1.Sum(fdiB) != x.fdiHash {
		return errors.New("full directory index hash mismatch (index is corrupt)")
	}
	fr := &reader{b: fdiB}
	x.fdi = readDirIndex(fr, loc)
	if fr.err == nil && fr.p != len(fdiB) {
		fr.fail(fmt.Errorf("%d unexpected bytes after the directory index", len(fdiB)-fr.p))
	}
	if fr.err != nil {
		return fmt.Errorf("corrupt full directory index: %w", fr.err)
	}
	listed := map[*Entry]bool{}
	for _, d := range x.fdi {
		for _, fl := range d.files {
			e := fl.entry
			if listed[e] {
				return fmt.Errorf("pak entry is listed twice in the directory index (%q); not supported", joinPakPath(d.name, fl.name))
			}
			listed[e] = true
			e.Name = joinPakPath(d.name, fl.name)
			e.rawName = append(append([]byte(nil), d.rawName...), fl.rawName...)
			e.IndexPos = len(p.Entries)
			p.Entries = append(p.Entries, e)
		}
	}
	if len(p.Entries) != len(x.encOrder)+len(x.nonEnc) {
		return fmt.Errorf("directory index lists %d files but the index has %d entries; not supported", len(p.Entries), len(x.encOrder)+len(x.nonEnc))
	}
	if int(x.rawCount) != len(p.Entries) {
		return fmt.Errorf("corrupt index: entry count %d differs from %d listed files", x.rawCount, len(p.Entries))
	}

	if x.hasPHIRaw != 0 {
		phiB, err := readRegion(f, x.phiOff, x.phiSize, footerStart, "path hash index")
		if err != nil {
			return err
		}
		if sha1.Sum(phiB) != x.phiHash {
			return errors.New("path hash index hash mismatch (index is corrupt)")
		}
		pr := &reader{b: phiB}
		n := pr.i32()
		if pr.err == nil && (n < 0 || int(n) > len(phiB)/12) {
			return fmt.Errorf("corrupt path hash index: count %d", n)
		}
		for i := 0; i < int(n) && pr.err == nil; i++ {
			h := uint64(pr.i64())
			l := pr.i32()
			if pr.err != nil {
				break
			}
			e, ok := loc[l]
			if !ok {
				return fmt.Errorf("corrupt path hash index: unknown location %d", l)
			}
			x.phi = append(x.phi, phiRec{hash: h, entry: e})
		}
		if pr.err == nil && pr.p < len(phiB) {
			x.hasPruned = true
			x.pruned = readDirIndex(pr, loc)
			if pr.err == nil && pr.p != len(phiB) {
				pr.fail(fmt.Errorf("%d unexpected bytes after the pruned directory index", len(phiB)-pr.p))
			}
		}
		if pr.err != nil {
			return fmt.Errorf("corrupt path hash index: %w", pr.err)
		}
	}
	p.v11 = x
	for _, e := range p.Entries {
		if e.Compressed() {
			e.replaceCompressed = true
		}
	}

	// Inline headers carry the SHA-1 (the encoded index does not).
	nonEncSet := map[*Entry]bool{}
	for _, e := range x.nonEnc {
		nonEncSet[e] = true
	}
	for _, e := range p.Entries {
		if e.Offset < 0 || e.Offset+e.HeaderSize() > footerStart {
			return fmt.Errorf("entry %q offset %d outside the file", e.Name, e.Offset)
		}
		h, err := readInlineHeader(f, e)
		if err != nil {
			return fmt.Errorf("entry %q: read inline header: %w", e.Name, err)
		}
		if nonEncSet[e] && e.Hash != h.Hash {
			return fmt.Errorf("entry %q: index hash differs from inline header", e.Name)
		}
		e.Hash = h.Hash
	}

	// Lossless check: re-serialising must reproduce the stored bytes.
	enc, locOf, err := p.encodeEntries(nil)
	if err != nil {
		return fmt.Errorf("re-encoding entries: %w", err)
	}
	if !bytes.Equal(enc, blob) {
		return errors.New("encoded entry re-serialisation is not lossless; refusing to handle this pak")
	}
	if !bytes.Equal(p.serializeFDI(locOf), fdiB) {
		return errors.New("directory index re-serialisation is not lossless; refusing to handle this pak")
	}
	if x.hasPHIRaw != 0 {
		phiB, _ := readRegion(f, x.phiOff, x.phiSize, footerStart, "path hash index")
		if !bytes.Equal(p.serializePHI(locOf), phiB) {
			return errors.New("path hash index re-serialisation is not lossless; refusing to handle this pak")
		}
	}
	prim := p.serializePrimary(enc, nil, x.phiOff, x.phiSize, x.phiHash, x.fdiOff, x.fdiSize, x.fdiHash)
	if !bytes.Equal(prim, idx) {
		return errors.New("index re-serialisation is not lossless; refusing to handle this pak")
	}
	return nil
}

// encodeEntries builds the encoded-entries blob with the given offsets
// (nil = current offsets, indexed by IndexPos) and returns each entry's
// location.
func (p *Pak) encodeEntries(offsets []int64) ([]byte, func(*Entry) int32, error) {
	x := p.v11
	locs := make(map[*Entry]int32, len(p.Entries))
	var b bytes.Buffer
	off := func(e *Entry) int64 {
		if offsets != nil {
			return offsets[e.IndexPos]
		}
		return e.Offset
	}
	for _, e := range x.encOrder {
		locs[e] = int32(b.Len())
		if err := encodeEntry(&b, e, off(e)); err != nil {
			return nil, nil, err
		}
		if b.Len() > math.MaxInt32 {
			return nil, nil, errors.New("encoded index too large")
		}
	}
	for i, e := range x.nonEnc {
		locs[e] = int32(-(i + 1))
	}
	return b.Bytes(), func(e *Entry) int32 { return locs[e] }, nil
}

func (p *Pak) serializeFDI(loc func(*Entry) int32) []byte {
	var b bytes.Buffer
	writeDirIndex(&b, p.v11.fdi, loc)
	return b.Bytes()
}

func (p *Pak) serializePHI(loc func(*Entry) int32) []byte {
	x := p.v11
	var b bytes.Buffer
	putU32(&b, uint32(len(x.phi)))
	for _, r := range x.phi {
		putU64(&b, r.hash)
		putU32(&b, uint32(loc(r.entry)))
	}
	if x.hasPruned {
		writeDirIndex(&b, x.pruned, loc)
	}
	return b.Bytes()
}

// serializePrimary writes the primary index. offsets (by IndexPos) override
// the non-encoded entries' offsets when non-nil.
func (p *Pak) serializePrimary(enc []byte, offsets []int64, phiOff, phiSize int64, phiHash [20]byte, fdiOff, fdiSize int64, fdiHash [20]byte) []byte {
	x := p.v11
	var b bytes.Buffer
	b.Write(p.rawMount)
	putU32(&b, uint32(x.rawCount))
	putU64(&b, x.seed)
	putU32(&b, x.hasPHIRaw)
	if x.hasPHIRaw != 0 {
		putU64(&b, uint64(phiOff))
		putU64(&b, uint64(phiSize))
		b.Write(phiHash[:])
	}
	putU32(&b, x.hasFDIRaw)
	if x.hasFDIRaw != 0 {
		putU64(&b, uint64(fdiOff))
		putU64(&b, uint64(fdiSize))
		b.Write(fdiHash[:])
	}
	putU32(&b, uint32(len(enc)))
	b.Write(enc)
	putU32(&b, uint32(len(x.nonEnc)))
	for _, e := range x.nonEnc {
		off := e.Offset
		if offsets != nil {
			off = offsets[e.IndexPos]
		}
		writeEntry(&b, e, off)
	}
	b.Write(x.trailer)
	return b.Bytes()
}

// regions returns the index structures stored after the entry data, in
// on-disk order.
func (p *Pak) regions() []region {
	rs := []region{{regionPrimary, p.Footer.IndexOffset, p.Footer.IndexSize}}
	if x := p.v11; x != nil {
		if x.hasPHIRaw != 0 {
			rs = append(rs, region{regionPHI, x.phiOff, x.phiSize})
		}
		if x.hasFDIRaw != 0 {
			rs = append(rs, region{regionFDI, x.fdiOff, x.fdiSize})
		}
	}
	for i := 1; i < len(rs); i++ {
		for j := i; j > 0 && rs[j].off < rs[j-1].off; j-- {
			rs[j], rs[j-1] = rs[j-1], rs[j]
		}
	}
	return rs
}

// dataEnd is the start of the first index structure.
func (p *Pak) dataEnd() int64 { return p.regions()[0].off }

// checkRegions validates that the index structures do not overlap.
func (p *Pak) checkRegions() error {
	rs := p.regions()
	footerStart := p.FileSize - int64(p.Footer.Size)
	for i, r := range rs {
		if r.off < 0 || r.size < 0 || r.off+r.size > footerStart {
			return fmt.Errorf("corrupt footer: index structure [%d,+%d) outside file data", r.off, r.size)
		}
		if i > 0 && r.off < rs[i-1].off+rs[i-1].size {
			return errors.New("index structures overlap; not supported")
		}
	}
	return nil
}

// canonicalV11 serialises the v11 structures with locations replaced by
// entry index positions, for structural comparison between two paks.
func (p *Pak) canonicalV11() []byte {
	x := p.v11
	pos := func(e *Entry) int32 { return int32(e.IndexPos) }
	var b bytes.Buffer
	b.Write(p.rawMount)
	putU32(&b, uint32(x.rawCount))
	putU64(&b, x.seed)
	putU32(&b, x.hasPHIRaw)
	putU32(&b, x.hasFDIRaw)
	b.Write(x.trailer)
	putU32(&b, uint32(len(x.encOrder)))
	for _, e := range x.encOrder {
		putU32(&b, uint32(e.IndexPos))
	}
	putU32(&b, uint32(len(x.nonEnc)))
	for _, e := range x.nonEnc {
		putU32(&b, uint32(e.IndexPos))
	}
	writeDirIndex(&b, x.fdi, pos)
	if x.hasPHIRaw != 0 {
		b.Write(p.serializePHI(pos))
	}
	return b.Bytes()
}

// checkV11Hashes verifies the path hash index against the entry names and
// the pruned directory index against the full one.
func (p *Pak) checkV11Hashes(add func(string, ...any)) {
	x := p.v11
	if x == nil {
		return
	}
	bad := 0
	for _, r := range x.phi {
		if HashPath(r.entry.Name, x.seed) != r.hash {
			if bad < 5 {
				add("path hash index: hash of %q does not match", r.entry.Name)
			}
			bad++
		}
	}
	if bad >= 5 {
		add("path hash index: %d hashes do not match in total", bad)
	}
	if x.hasPHIRaw != 0 && len(x.phi) != len(p.Entries) {
		add("path hash index has %d entries, index has %d", len(x.phi), len(p.Entries))
	}
	for _, d := range x.pruned {
		for _, f := range d.files {
			if joinPakPath(d.name, f.name) != f.entry.Name {
				add("pruned directory index entry %q points to %q", joinPakPath(d.name, f.name), f.entry.Name)
			}
		}
	}
}

// parseV11Pak completes parse for a v10+ pak.
func parseV11Pak(p *Pak, f io.ReaderAt, idx []byte) (*Pak, error) {
	if err := parseV11(p, f, idx); err != nil {
		return nil, err
	}
	if err := p.checkRegions(); err != nil {
		return nil, err
	}
	dataEnd := p.dataEnd()
	for _, e := range p.Entries {
		if _, ok := p.byName[e.Name]; ok {
			p.dupNames[e.Name] = true
		} else {
			p.byName[e.Name] = e
		}
		if e.Offset < 0 || e.Offset+e.RegionSize() > dataEnd {
			return nil, fmt.Errorf("entry %q region [%d,+%d) lies outside the data area", e.Name, e.Offset, e.RegionSize())
		}
	}
	return p, nil
}
