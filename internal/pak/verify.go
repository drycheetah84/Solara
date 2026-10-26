package pak

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// VerifyOptions configures Verify.
type VerifyOptions struct {
	// Source, when set, is the pak the verified file was rebuilt from. Every
	// entry not listed in Expect must then be byte-identical to the source,
	// and version, mount point, entry list and footer must match.
	Source string
	// Expect maps replaced entry names to their expected new data.
	Expect map[string][]byte
	// Progress reports verified bytes.
	Progress ProgressFunc
	// Decompressor is used to check replaced entries that are stored
	// compressed (v11 recompress).
	Decompressor Decompressor
}

// VerifyReport summarises a successful verification.
type VerifyReport struct {
	Version       uint32
	Entries       int
	HashedEntries int // entries whose SHA-1 was checked
	SkippedHash   int // encrypted entries (hash not checkable without key)
}

// VerifyError lists all problems found by Verify.
type VerifyError struct {
	Path     string
	Problems []string
}

func (e *VerifyError) Error() string {
	const max = 10
	ps := e.Problems
	extra := ""
	if len(ps) > max {
		extra = fmt.Sprintf(" (and %d more)", len(ps)-max)
		ps = ps[:max]
	}
	return fmt.Sprintf("verification of %s failed: %s%s", e.Path, strings.Join(ps, "; "), extra)
}

// Verify fully checks the pak at path: footer sanity, index hash, every
// entry's inline header against the index, every entry's data SHA-1, and
// (with opts.Source) equality with the source pak.
func Verify(path string, opts *VerifyOptions) (*VerifyReport, error) {
	if opts == nil {
		opts = &VerifyOptions{}
	}
	p, err := Open(path) // checks magic, version, index bounds, index hash
	if err != nil {
		return nil, &VerifyError{Path: path, Problems: []string{err.Error()}}
	}
	ve := &VerifyError{Path: path}
	add := func(format string, a ...any) { ve.Problems = append(ve.Problems, fmt.Sprintf(format, a...)) }

	if _, err := p.physicalOrder(); err != nil {
		add("%v", err)
	}
	p.checkV11Hashes(add)

	var src *Pak
	var srcF *os.File
	if opts.Source != "" {
		src, err = Open(opts.Source)
		if err != nil {
			return nil, fmt.Errorf("verify: open source: %w", err)
		}
		srcF, err = os.Open(opts.Source)
		if err != nil {
			return nil, err
		}
		defer srcF.Close()
		compareStructure(p, src, opts.Expect, add)
	}
	for name := range opts.Expect {
		if _, ok := p.byName[name]; !ok {
			add("expected replaced entry %q is missing", name)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	rep := &VerifyReport{Version: p.Footer.Version, Entries: len(p.Entries)}
	var total, done int64
	for _, e := range p.Entries {
		total += e.RegionSize()
	}
	buf := make([]byte, 1<<20)
	buf2 := make([]byte, 1<<20)
	for _, e := range p.Entries {
		h, err := readInlineHeader(f, e)
		if err != nil {
			add("entry %q: cannot read inline header: %v", e.Name, err)
			continue
		}
		if err := headerMatches(h, e); err != nil {
			add("entry %q: %v", e.Name, err)
		}
		dataOff := e.Offset + e.HeaderSize()
		if !e.Encrypted() {
			hs := sha1.New()
			if _, err := io.CopyBuffer(hs, io.NewSectionReader(f, dataOff, e.Size), buf); err != nil {
				add("entry %q: read data: %v", e.Name, err)
			} else if !bytes.Equal(hs.Sum(nil), e.Hash[:]) {
				add("entry %q: SHA-1 mismatch", e.Name)
			}
			rep.HashedEntries++
		} else {
			rep.SkippedHash++
		}
		if want, ok := opts.Expect[e.Name]; ok && e.Compressed() {
			checkCompressedExpect(p, f, e, want, opts.Decompressor, add)
		} else if ok {
			if int64(len(want)) != e.Size || sha1.Sum(want) != e.Hash {
				add("entry %q: content differs from the expected replacement", e.Name)
			} else if !e.Encrypted() {
				got := make([]byte, e.Size)
				if _, err := f.ReadAt(got, dataOff); err != nil || !bytes.Equal(got, want) {
					add("entry %q: content differs from the expected replacement", e.Name)
				}
			}
		} else if src != nil {
			se, ok := src.byName[e.Name]
			if ok && !src.dupNames[e.Name] {
				if se.RegionSize() != e.RegionSize() {
					add("entry %q: size differs from source", e.Name)
				} else if !sameBytes(f, e.Offset, srcF, se.Offset, e.RegionSize(), buf, buf2) {
					add("entry %q: bytes differ from source", e.Name)
				}
			}
		}
		done += e.RegionSize()
		if opts.Progress != nil {
			opts.Progress(done, total)
		}
		if len(ve.Problems) > 200 {
			break
		}
	}
	if len(ve.Problems) > 0 {
		return nil, ve
	}
	return rep, nil
}

// compareStructure checks that p has the same structure as its source.
func compareStructure(p, src *Pak, expect map[string][]byte, add func(string, ...any)) {
	if p.Footer.Version != src.Footer.Version {
		add("version %d differs from source %d", p.Footer.Version, src.Footer.Version)
	}
	if p.Footer.Size != src.Footer.Size || p.Footer.EncryptionKeyGUID != src.Footer.EncryptionKeyGUID ||
		p.Footer.EncryptedIndex != src.Footer.EncryptedIndex || strings.Join(p.Footer.CompressionNames, ",") != strings.Join(src.Footer.CompressionNames, ",") {
		add("footer fields differ from source")
	}
	if !bytes.Equal(p.rawMount, src.rawMount) {
		add("mount point differs from source")
	}
	if len(p.Entries) != len(src.Entries) {
		add("entry count %d differs from source %d", len(p.Entries), len(src.Entries))
		return
	}
	for i, e := range p.Entries {
		s := src.Entries[i]
		if !bytes.Equal(e.rawName, s.rawName) {
			add("entry %d name %q differs from source %q", i, e.Name, s.Name)
			continue
		}
		_, replaced := expect[e.Name]
		if e.Flags != s.Flags || (!replaced && (e.CompressionMethod != s.CompressionMethod || e.CompressionBlockSize != s.CompressionBlockSize)) {
			add("entry %q: flags/compression differ from source", e.Name)
		}
		if replaced && e.CompressionMethod != 0 && e.CompressionMethod != s.CompressionMethod {
			add("entry %q: replaced entry uses a different compression method than the source", e.Name)
		}
		if !replaced {
			if e.Size != s.Size || e.UncompressedSize != s.UncompressedSize || e.Hash != s.Hash {
				add("entry %q: index entry differs from source", e.Name)
			}
		}
	}
	// physical order must be preserved
	po, err1 := p.physicalOrder()
	so, err2 := src.physicalOrder()
	if err1 == nil && err2 == nil {
		for i := range po {
			if po[i].IndexPos != so[i].IndexPos {
				add("physical entry order differs from source at position %d", i)
				break
			}
		}
	}
	if !bytes.Equal(p.indexTrailer, src.indexTrailer) {
		add("index trailer bytes differ from source")
	}
	if (p.v11 == nil) != (src.v11 == nil) {
		add("index layout differs from source")
	} else if p.v11 != nil && !bytes.Equal(p.canonicalV11(), src.canonicalV11()) {
		add("v11 index structure (directory index, path hash index, entry order) differs from source")
	}
}

func sameBytes(a io.ReaderAt, aoff int64, b io.ReaderAt, boff int64, n int64, b1, b2 []byte) bool {
	for n > 0 {
		c := int64(len(b1))
		if n < c {
			c = n
		}
		if _, err := a.ReadAt(b1[:c], aoff); err != nil {
			return false
		}
		if _, err := b.ReadAt(b2[:c], boff); err != nil {
			return false
		}
		if !bytes.Equal(b1[:c], b2[:c]) {
			return false
		}
		aoff += c
		boff += c
		n -= c
	}
	return true
}

// IsVerifyError reports whether err is a *VerifyError.
func IsVerifyError(err error) bool {
	var ve *VerifyError
	return errors.As(err, &ve)
}

// checkCompressedExpect decompresses a replaced, compressed entry and
// compares it with the intended content.
func checkCompressedExpect(p *Pak, f io.ReaderAt, e *Entry, want []byte, dec Decompressor, add func(string, ...any)) {
	if int64(len(want)) != e.UncompressedSize {
		add("entry %q: uncompressed size %d differs from the expected replacement (%d)", e.Name, e.UncompressedSize, len(want))
		return
	}
	if dec == nil {
		add("entry %q: is stored compressed but no decompressor is available to check it", e.Name)
		return
	}
	if e.Size > 1<<30 {
		add("entry %q: too large to check", e.Name)
		return
	}
	stored := make([]byte, e.Size)
	if _, err := f.ReadAt(stored, e.Offset+e.HeaderSize()); err != nil {
		add("entry %q: read data: %v", e.Name, err)
		return
	}
	got, err := p.decompressStored(e, stored, dec)
	if err != nil {
		add("entry %q: decompress check failed: %v", e.Name, err)
		return
	}
	if !bytes.Equal(got, want) {
		add("entry %q: decompressed content differs from the expected replacement", e.Name)
	}
}
