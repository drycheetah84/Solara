package pak

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MethodOodle is the compression method name UE writes for Oodle.
const MethodOodle = "Oodle"

// OodleEnvVar names the environment variable with the Oodle DLL path.
const OodleEnvVar = "PAK_EDITOR_OODLE_DLL"

// Decompressor decompresses one compression block.
type Decompressor interface {
	// Decompress decodes comp (compressed with method) into exactly rawLen
	// bytes.
	Decompress(method string, comp []byte, rawLen int) ([]byte, error)
}

// Compressor compresses one compression block.
type Compressor interface {
	Compress(method string, raw []byte) ([]byte, error)
}

// Codec both compresses and decompresses (e.g. *Oodle).
type Codec interface {
	Compressor
	Decompressor
}

// ErrOodleRequired is returned when an Oodle-compressed entry is read
// without an Oodle decompressor configured.
var ErrOodleRequired = errors.New("this entry is Oodle-compressed; the Oodle DLL is required to read it (set it in Settings or via " + OodleEnvVar + ")")

// UnsupportedCompressionError is returned for compression methods this
// package cannot decode.
type UnsupportedCompressionError struct {
	Entry  string
	Method string
}

func (e *UnsupportedCompressionError) Error() string {
	m := e.Method
	if m == "" {
		m = "<unnamed>"
	}
	return fmt.Sprintf("entry %q uses compression method %q, which is not supported (only %s)", e.Entry, m, MethodOodle)
}

// MethodName returns the compression method name of e ("" if uncompressed
// or the slot is empty).
func (p *Pak) MethodName(e *Entry) string {
	i := int(e.CompressionMethod)
	if i == 0 || i > len(p.Footer.CompressionNames) {
		return ""
	}
	return p.Footer.CompressionNames[i-1]
}

// CompressionMethods returns the non-empty compression method names of the
// footer, in slot order.
func (p *Pak) CompressionMethods() []string {
	out := []string{}
	for _, n := range p.Footer.CompressionNames {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// methodIndex returns the 1-based footer slot of method, or 0.
func (p *Pak) methodIndex(method string) uint32 {
	for i, n := range p.Footer.CompressionNames {
		if strings.EqualFold(n, method) {
			return uint32(i + 1)
		}
	}
	return 0
}

// decompressStored turns the stored bytes of a compressed entry (as read
// from offset+HeaderSize) into its uncompressed content.
func (p *Pak) decompressStored(e *Entry, stored []byte, dec Decompressor) ([]byte, error) {
	method := p.MethodName(e)
	if !strings.EqualFold(method, MethodOodle) {
		return nil, &UnsupportedCompressionError{Entry: e.Name, Method: method}
	}
	if dec == nil {
		return nil, fmt.Errorf("entry %q: %w", e.Name, ErrOodleRequired)
	}
	if e.UncompressedSize < 0 || e.UncompressedSize > 1<<30 {
		return nil, fmt.Errorf("entry %q too large (%d bytes)", e.Name, e.UncompressedSize)
	}
	hs := e.HeaderSize()
	cbs := int64(e.CompressionBlockSize)
	if len(e.Blocks) == 1 {
		cbs = e.UncompressedSize
	}
	if cbs <= 0 && len(e.Blocks) > 0 {
		return nil, fmt.Errorf("entry %q: compression block size is 0", e.Name)
	}
	out := make([]byte, 0, e.UncompressedSize)
	remaining := e.UncompressedSize
	for i, b := range e.Blocks {
		s, en := b.Start-hs, b.End-hs
		if s < 0 || en < s || en > int64(len(stored)) {
			return nil, fmt.Errorf("entry %q: compression block %d [%d,%d) outside the stored data", e.Name, i, b.Start, b.End)
		}
		raw := cbs
		if raw > remaining {
			raw = remaining
		}
		d, err := dec.Decompress(method, stored[s:en], int(raw))
		if err != nil {
			return nil, fmt.Errorf("entry %q block %d: %w", e.Name, i, err)
		}
		if int64(len(d)) != raw {
			return nil, fmt.Errorf("entry %q block %d: decompressed %d bytes, want %d", e.Name, i, len(d), raw)
		}
		out = append(out, d...)
		remaining -= raw
	}
	if remaining != 0 {
		return nil, fmt.Errorf("entry %q: blocks cover %d of %d bytes", e.Name, e.UncompressedSize-remaining, e.UncompressedSize)
	}
	return out, nil
}

// DefaultCompressionBlockSize is the block size used when recompressing.
const DefaultCompressionBlockSize = 64 << 10

// compressBlocks splits raw into blocks of cbs bytes, compresses each with
// c and checks it decompresses back (when dec is non-nil). It returns the
// concatenated payload and the per-block compressed sizes.
func compressBlocks(c Compressor, dec Decompressor, method string, raw []byte, cbs int) ([]byte, []int, error) {
	if cbs <= 0 {
		cbs = DefaultCompressionBlockSize
	}
	var payload []byte
	var sizes []int
	for start := 0; start < len(raw) || (start == 0 && len(raw) == 0); start += cbs {
		end := start + cbs
		if end > len(raw) {
			end = len(raw)
		}
		blk := raw[start:end]
		comp, err := c.Compress(method, blk)
		if err != nil {
			return nil, nil, err
		}
		if dec != nil {
			back, err := dec.Decompress(method, comp, len(blk))
			if err != nil {
				return nil, nil, fmt.Errorf("recompressed block does not decompress: %w", err)
			}
			if string(back) != string(blk) {
				return nil, nil, errors.New("recompressed block does not decompress to the original data")
			}
		}
		payload = append(payload, comp...)
		sizes = append(sizes, len(comp))
		if len(raw) == 0 {
			break
		}
	}
	return payload, sizes, nil
}

// OodleDLLNames are the file names searched for next to the executable.
var OodleDLLNames = []string{"oo2core_*_win64.dll", "oodle-data-shared.dll"}

// FindOodleDLL resolves the Oodle DLL path: explicit (if non-empty), then
// the PAK_EDITOR_OODLE_DLL environment variable, then oo2core_*_win64.dll /
// oodle-data-shared.dll next to the executable. source is "explicit",
// "env" or "exe-dir"; path is "" when nothing was found.
func FindOodleDLL(explicit string) (path, source string) {
	if explicit != "" {
		return explicit, "explicit"
	}
	if v := strings.Trim(strings.TrimSpace(os.Getenv(OodleEnvVar)), "\""); v != "" {
		return v, "env"
	}
	exe, err := os.Executable()
	if err != nil {
		return "", ""
	}
	dir := filepath.Dir(exe)
	for _, pat := range OodleDLLNames {
		m, _ := filepath.Glob(filepath.Join(dir, pat))
		sort.Strings(m)
		for _, f := range m {
			if st, err := os.Stat(f); err == nil && st.Mode().IsRegular() {
				return f, "exe-dir"
			}
		}
	}
	return "", ""
}
