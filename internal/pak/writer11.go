package pak

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
)

// splitPakPath splits an entry name into its directory-index directory
// ("A2/Config/", or "/" for the root) and file name.
func splitPakPath(name string) (dir, file string) {
	k := strings.LastIndex(name, "/")
	if k < 0 {
		return "/", name
	}
	return name[:k+1], name[k+1:]
}

// writeNewV11 creates a version 11 pak: entry data, primary index, path
// hash index (with a pruned directory index of the .ini files), full
// directory index, 221-byte footer. Entries should be grouped by directory
// so that the directory index order equals the files order.
func writeNewV11(path, mountPoint string, files []NewFile, opts *WriteOptions) error {
	order := opts.PhysicalOrder
	if order == nil {
		for i := range files {
			order = append(order, i)
		}
	}
	if len(order) != len(files) {
		return fmt.Errorf("physical order has %d items for %d files", len(order), len(files))
	}
	names := opts.CompressionNames
	entries := make([]*Entry, len(files))
	var body bytes.Buffer
	for _, i := range order {
		nf := files[i]
		for g := 0; g < opts.GapBefore[i]; g++ {
			body.WriteByte(byte(0xA0 + g%16))
		}
		e := &Entry{
			Name:              nf.Name,
			Offset:            int64(body.Len()),
			CompressionMethod: nf.CompressionMethod,
			IndexPos:          i,
			v11:               true,
			encFresh:          true,
		}
		payload := nf.Data
		switch {
		case nf.CompressionMethod != 0 && opts.Compressor != nil:
			if int(nf.CompressionMethod) > len(names) {
				return fmt.Errorf("%s: compression method %d has no footer name", nf.Name, nf.CompressionMethod)
			}
			cbs := int(nf.CompressionBlockSize)
			if cbs == 0 {
				cbs = DefaultCompressionBlockSize
			}
			var sizes []int
			var err error
			payload, sizes, err = compressBlocks(opts.Compressor, nil, names[nf.CompressionMethod-1], nf.Data, cbs)
			if err != nil {
				return fmt.Errorf("%s: %w", nf.Name, err)
			}
			e.Blocks = make([]Block, len(sizes))
			cur := e.HeaderSize()
			for k, s := range sizes {
				e.Blocks[k] = Block{Start: cur, End: cur + int64(s)}
				cur += int64(s)
			}
			e.Size = int64(len(payload))
			e.UncompressedSize = int64(len(nf.Data))
			e.CompressionBlockSize = uint32(cbs)
			if len(sizes) == 1 {
				e.CompressionBlockSize = uint32(len(nf.Data))
			}
		case nf.CompressionMethod != 0:
			// fake single block: data stored as-is
			e.Size = int64(len(nf.Data))
			e.UncompressedSize = int64(len(nf.Data)) * 2
			e.Blocks = []Block{{}}
			hs := e.HeaderSize()
			e.Blocks[0] = Block{Start: hs, End: hs + e.Size}
			e.CompressionBlockSize = uint32(e.UncompressedSize)
		default:
			e.Size = int64(len(nf.Data))
			e.UncompressedSize = e.Size
		}
		e.Hash = sha1.Sum(payload)
		if nf.Encrypted {
			if e.Compressed() {
				return fmt.Errorf("%s: encrypted compressed test entries are not supported", nf.Name)
			}
			e.Flags |= FlagEncrypted
			payload = append(append([]byte(nil), payload...), make([]byte, align16(int64(len(payload)))-int64(len(payload)))...)
		}
		writeEntry(&body, e, 0)
		body.Write(payload)
		entries[i] = e
	}
	for g := 0; g < opts.GapBeforeIndex; g++ {
		body.WriteByte(0xEE)
	}

	x := &indexV11{rawCount: int32(len(files)), seed: opts.PathHashSeed, hasPHIRaw: 1, hasFDIRaw: 1, hasPruned: true}
	encOrd := opts.EncodedOrder
	if encOrd == nil {
		for i := range files {
			encOrd = append(encOrd, i)
		}
	}
	for _, i := range encOrd {
		if !opts.NonEncoded[i] {
			x.encOrder = append(x.encOrder, entries[i])
		}
	}
	for i := range files {
		if opts.NonEncoded[i] {
			x.nonEnc = append(x.nonEnc, entries[i])
		}
	}
	if len(x.encOrder)+len(x.nonEnc) != len(files) {
		return fmt.Errorf("encoded order covers %d of %d files", len(x.encOrder)+len(x.nonEnc), len(files))
	}
	x.fdi = []dirRec{{rawName: encodeFString("/"), name: "/"}}
	dirPos := map[string]int{"/": 0}
	for _, e := range entries {
		dir, file := splitPakPath(e.Name)
		k, ok := dirPos[dir]
		if !ok {
			k = len(x.fdi)
			dirPos[dir] = k
			x.fdi = append(x.fdi, dirRec{rawName: encodeFString(dir), name: dir})
		}
		x.fdi[k].files = append(x.fdi[k].files, dirFile{rawName: encodeFString(file), name: file, entry: e})
	}
	for _, d := range x.fdi {
		var pd dirRec
		for _, f := range d.files {
			if strings.HasSuffix(strings.ToLower(f.name), ".ini") {
				pd.files = append(pd.files, f)
			}
		}
		if len(pd.files) > 0 {
			pd.rawName, pd.name = d.rawName, d.name
			x.pruned = append(x.pruned, pd)
		}
	}
	for _, e := range append(append([]*Entry(nil), x.encOrder...), x.nonEnc...) {
		x.phi = append(x.phi, phiRec{hash: HashPath(e.Name, x.seed), entry: e})
	}

	p := &Pak{rawMount: encodeFString(mountPoint), Entries: entries, v11: x}
	enc, loc, err := p.encodeEntries(nil)
	if err != nil {
		return err
	}
	phi := p.serializePHI(loc)
	fdi := p.serializeFDI(loc)
	var zero [20]byte
	indexOffset := int64(body.Len())
	primLen := int64(len(p.serializePrimary(enc, nil, 0, 0, zero, 0, 0, zero)))
	phiOff := indexOffset + primLen
	fdiOff := phiOff + int64(len(phi))
	index := p.serializePrimary(enc, nil, phiOff, int64(len(phi)), sha1.Sum(phi), fdiOff, int64(len(fdi)), sha1.Sum(fdi))
	body.Write(index)
	body.Write(phi)
	body.Write(fdi)

	footer := make([]byte, 221)
	binary.LittleEndian.PutUint32(footer[17:], Magic)
	binary.LittleEndian.PutUint32(footer[21:], VersionFnv64BugFix)
	binary.LittleEndian.PutUint64(footer[25:], uint64(indexOffset))
	binary.LittleEndian.PutUint64(footer[33:], uint64(len(index)))
	h := sha1.Sum(index)
	copy(footer[41:61], h[:])
	for i, n := range names {
		if i < 5 {
			copy(footer[61+32*i:61+32*i+31], n)
		}
	}
	body.Write(footer)
	return os.WriteFile(path, body.Bytes(), 0o644)
}
