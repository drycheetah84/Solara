package pak

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ProgressFunc reports done/total bytes.
type ProgressFunc func(done, total int64)

// RebuildResult describes a rebuilt pak.
type RebuildResult struct {
	Size        int64
	IndexOffset int64
	Changed     []string // replaced entries whose content differs, index order
	// Recompressed lists replaced entries stored Oodle-compressed (v11 with
	// RebuildOptions.Recompress), index order.
	Recompressed []string
}

// RebuildOptions configures RebuildWith.
type RebuildOptions struct {
	Progress ProgressFunc
	// Recompress stores replaced entries that were Oodle-compressed in the
	// source compressed again (Kraken) instead of uncompressed. v11 only;
	// needs Codec. Every compressed block is checked by decompressing it.
	// Entries that were uncompressed stay uncompressed, and an entry whose
	// compressed form would not be smaller is stored uncompressed.
	Recompress bool
	// Codec compresses (Recompress) and checks the recompressed blocks.
	Codec Codec
}

// physicalOrder returns entries sorted by on-disk offset (stable on index
// position) and checks that regions do not overlap.
func (p *Pak) physicalOrder() ([]*Entry, error) {
	ord := append([]*Entry(nil), p.Entries...)
	sort.SliceStable(ord, func(i, j int) bool { return ord[i].Offset < ord[j].Offset })
	var end int64
	for i, e := range ord {
		if i > 0 && e.Offset < end {
			return nil, fmt.Errorf("entries overlap on disk at offset %d (%q); rewriting such paks is not supported", e.Offset, e.Name)
		}
		end = e.Offset + e.RegionSize()
	}
	return ord, nil
}

// checkReplacements validates replacement names against the pak.
func (p *Pak) checkReplacements(repl map[string][]byte) error {
	for name := range repl {
		e, ok := p.byName[name]
		if !ok {
			return fmt.Errorf("cannot replace %q: entry not found in pak", name)
		}
		if p.dupNames[name] {
			return fmt.Errorf("cannot replace %q: the pak contains several entries with this name", name)
		}
		if e.Compressed() && !e.replaceCompressed {
			return fmt.Errorf("cannot replace %q: entry is compressed (only uncompressed entries can be replaced)", name)
		}
		if e.Encrypted() {
			return fmt.Errorf("cannot replace %q: entry is encrypted (only unencrypted entries can be replaced)", name)
		}
	}
	return nil
}

// replacement is a prepared v11 replacement entry.
type replacement struct {
	entry        Entry
	payload      []byte
	recompressed bool
}

// makeReplacement prepares the new index entry and stored bytes for e.
func (p *Pak) makeReplacement(e *Entry, data []byte, opts *RebuildOptions) (*replacement, error) {
	ne := *e
	ne.UncompressedSize = int64(len(data))
	ne.encFresh = true
	if opts.Recompress && e.Compressed() && strings.EqualFold(p.MethodName(e), MethodOodle) && len(data) > 0 {
		cbs := DefaultCompressionBlockSize
		if len(e.Blocks) > 1 && e.CompressionBlockSize > 0 {
			cbs = int(e.CompressionBlockSize)
		}
		payload, sizes, err := compressBlocks(opts.Codec, opts.Codec, MethodOodle, data, cbs)
		if err != nil {
			return nil, fmt.Errorf("recompressing %q: %w", e.Name, err)
		}
		if len(payload) < len(data) {
			ne.CompressionMethod = e.CompressionMethod
			ne.Blocks = make([]Block, len(sizes))
			cur := ne.HeaderSize()
			for i, s := range sizes {
				ne.Blocks[i] = Block{Start: cur, End: cur + int64(s)}
				cur += int64(s)
			}
			ne.Size = int64(len(payload))
			ne.CompressionBlockSize = uint32(cbs)
			if len(sizes) == 1 {
				ne.CompressionBlockSize = uint32(len(data))
			}
			ne.Hash = sha1.Sum(payload)
			return &replacement{entry: ne, payload: payload, recompressed: true}, nil
		}
	}
	ne.CompressionMethod = 0
	ne.Blocks = nil
	ne.CompressionBlockSize = 0
	ne.Size = int64(len(data))
	ne.Hash = sha1.Sum(data)
	return &replacement{entry: ne, payload: data}, nil
}

// Rebuild streams a copy of the pak at src to dst, replacing the data of the
// entries named in replacements. Everything else (version, mount point,
// entry order, physical order, flags, compression block size, gap bytes,
// footer fields other than the index location/hash) is preserved. The
// output is fsynced before Rebuild returns.
func Rebuild(src, dst string, replacements map[string][]byte, progress ProgressFunc) (*RebuildResult, error) {
	return RebuildWith(src, dst, replacements, &RebuildOptions{Progress: progress})
}

// RebuildWith is Rebuild with options. For v11 paks, replaced entries are
// written uncompressed (or recompressed, see RebuildOptions.Recompress);
// all untouched entries keep their stored bytes, and the encoded entries,
// path hash index, full directory index, index hashes and footer are
// regenerated.
func RebuildWith(src, dst string, replacements map[string][]byte, opts *RebuildOptions) (*RebuildResult, error) {
	if opts == nil {
		opts = &RebuildOptions{}
	}
	progress := opts.Progress
	p, err := Open(src)
	if err != nil {
		return nil, err
	}
	if err := p.checkReplacements(replacements); err != nil {
		return nil, err
	}
	if opts.Recompress {
		if p.v11 == nil {
			return nil, fmt.Errorf("recompressing is only supported for pak version %d", VersionFnv64BugFix)
		}
		if opts.Codec == nil {
			return nil, errors.New("recompressing requires the Oodle DLL")
		}
	}
	ord, err := p.physicalOrder()
	if err != nil {
		return nil, err
	}
	in, err := os.Open(src)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	if st, err := in.Stat(); err != nil {
		return nil, err
	} else if st.Size() != p.FileSize {
		return nil, fmt.Errorf("%s changed while being read", src)
	}

	var prepared map[string]*replacement
	total := p.FileSize
	if p.v11 != nil {
		prepared = map[string]*replacement{}
		for name, data := range replacements {
			e := p.byName[name]
			r, err := p.makeReplacement(e, data, opts)
			if err != nil {
				return nil, err
			}
			prepared[name] = r
			total += r.entry.RegionSize() - e.RegionSize()
		}
	} else {
		for name, data := range replacements {
			total += int64(len(data)) - p.byName[name].Size
		}
	}

	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			out.Close()
			os.Remove(dst)
		}
	}()

	w := &countingWriter{w: bufio.NewWriterSize(out, 1<<20), total: total, progress: progress}
	copyRange := func(from, to int64) error {
		if to <= from {
			return nil
		}
		_, err := io.Copy(w, io.NewSectionReader(in, from, to-from))
		return err
	}

	newOffsets := make([]int64, len(p.Entries))
	var cursor int64
	res := &RebuildResult{}
	changed := map[string]bool{}
	recompressed := map[string]bool{}
	for _, e := range ord {
		if err := copyRange(cursor, e.Offset); err != nil { // gap bytes
			return nil, err
		}
		newOffsets[e.IndexPos] = w.n
		origEnd := e.Offset + e.RegionSize()
		data, replace := replacements[e.Name]
		if replace && !p.dupNames[e.Name] && prepared != nil {
			r := prepared[e.Name]
			ne := r.entry
			if ne.Hash != e.Hash || ne.Size != e.Size || ne.CompressionMethod != e.CompressionMethod {
				changed[e.Name] = true
			}
			if r.recompressed {
				recompressed[e.Name] = true
			}
			var hb bytes.Buffer
			writeEntry(&hb, &ne, 0)
			if _, err := w.Write(hb.Bytes()); err != nil {
				return nil, err
			}
			if _, err := w.Write(r.payload); err != nil {
				return nil, err
			}
			*e = ne
		} else if replace && !p.dupNames[e.Name] {
			ne := *e
			ne.Size = int64(len(data))
			ne.UncompressedSize = int64(len(data))
			ne.Hash = sha1.Sum(data)
			if ne.Hash != e.Hash || ne.Size != e.Size {
				changed[e.Name] = true
			}
			var hb bytes.Buffer
			writeEntry(&hb, &ne, 0)
			if _, err := w.Write(hb.Bytes()); err != nil {
				return nil, err
			}
			if _, err := w.Write(data); err != nil {
				return nil, err
			}
			*e = ne // index entry now describes the new data
		} else if err := copyRange(e.Offset, origEnd); err != nil {
			return nil, err
		}
		cursor = origEnd
	}
	if err := copyRange(cursor, p.dataEnd()); err != nil { // tail gap
		return nil, err
	}
	indexOffset := w.n
	var index []byte
	footerStart := p.FileSize - int64(p.Footer.Size)
	if p.v11 != nil {
		index, err = p.writeV11Index(w, copyRange, newOffsets)
		if err != nil {
			return nil, err
		}
	} else {
		index = p.serializeIndex(newOffsets)
		if _, err := w.Write(index); err != nil {
			return nil, err
		}
		if err := copyRange(p.Footer.IndexOffset+p.Footer.IndexSize, footerStart); err != nil {
			return nil, err
		}
	}
	footer := append([]byte(nil), p.Footer.raw...)
	m := p.Footer.magicPos
	binary.LittleEndian.PutUint64(footer[m+8:], uint64(indexOffset))
	binary.LittleEndian.PutUint64(footer[m+16:], uint64(len(index)))
	h := sha1.Sum(index)
	copy(footer[m+24:m+44], h[:])
	if _, err := w.Write(footer); err != nil {
		return nil, err
	}
	if err := w.w.(*bufio.Writer).Flush(); err != nil {
		return nil, err
	}
	if err := out.Sync(); err != nil {
		return nil, fmt.Errorf("fsync %s: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		return nil, err
	}
	ok = true
	res.Size = w.n
	res.IndexOffset = indexOffset
	for _, e := range p.Entries {
		if changed[e.Name] {
			res.Changed = append(res.Changed, e.Name)
		}
		if recompressed[e.Name] {
			res.Recompressed = append(res.Recompressed, e.Name)
		}
	}
	if progress != nil {
		progress(w.n, w.n)
	}
	return res, nil
}

// writeV11Index writes the primary index, path hash index and full
// directory index at the writer's current position, in their original
// relative order and with the original gap bytes between them and up to
// the footer. It returns the primary index bytes.
func (p *Pak) writeV11Index(w *countingWriter, copyRange func(from, to int64) error, newOffsets []int64) ([]byte, error) {
	x := p.v11
	enc, loc, err := p.encodeEntries(newOffsets)
	if err != nil {
		return nil, err
	}
	content := map[int][]byte{}
	if x.hasPHIRaw != 0 {
		content[regionPHI] = p.serializePHI(loc)
	}
	content[regionFDI] = p.serializeFDI(loc)
	var zero [20]byte
	primaryLen := int64(len(p.serializePrimary(enc, newOffsets, 0, 0, zero, 0, 0, zero)))

	rs := p.regions()
	newOff := map[int]int64{}
	pos := w.n
	for i, r := range rs {
		if i > 0 {
			pos += r.off - (rs[i-1].off + rs[i-1].size)
		}
		newOff[r.kind] = pos
		if r.kind == regionPrimary {
			pos += primaryLen
		} else {
			pos += int64(len(content[r.kind]))
		}
	}
	phiB, fdiB := content[regionPHI], content[regionFDI]
	primary := p.serializePrimary(enc, newOffsets,
		newOff[regionPHI], int64(len(phiB)), sha1.Sum(phiB),
		newOff[regionFDI], int64(len(fdiB)), sha1.Sum(fdiB))
	if int64(len(primary)) != primaryLen {
		return nil, errors.New("internal error: primary index size changed")
	}
	content[regionPrimary] = primary
	for i, r := range rs {
		if i > 0 {
			if err := copyRange(rs[i-1].off+rs[i-1].size, r.off); err != nil {
				return nil, err
			}
		}
		if w.n != newOff[r.kind] {
			return nil, errors.New("internal error: index layout mismatch")
		}
		if _, err := w.Write(content[r.kind]); err != nil {
			return nil, err
		}
	}
	last := rs[len(rs)-1]
	if err := copyRange(last.off+last.size, p.FileSize-int64(p.Footer.Size)); err != nil {
		return nil, err
	}
	return primary, nil
}

type countingWriter struct {
	w        io.Writer
	n        int64
	total    int64
	last     int64
	progress ProgressFunc
}

func (c *countingWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	if c.progress != nil && c.n-c.last >= 4<<20 {
		c.last = c.n
		c.progress(c.n, c.total)
	}
	return n, err
}
