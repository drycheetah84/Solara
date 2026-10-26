//go:build windows

package pak

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// Oodle enum values (oodle2.h).
const (
	oodleCompressorKraken = 8 // OodleLZ_Compressor_Kraken
	oodleLevelNormal      = 4 // OodleLZ_CompressionLevel_Normal
	oodleFuzzSafeYes      = 1 // OodleLZ_FuzzSafe_Yes
	oodleCheckCRCNo       = 0 // OodleLZ_CheckCRC_No
	oodleVerbosityNone    = 0 // OodleLZ_Verbosity_None
	oodleDecodeUnthreaded = 3 // OodleLZ_Decode_Unthreaded
)

// Oodle calls an Oodle DLL (oo2core_*_win64.dll or UE's
// oodle-data-shared.dll) through syscall, without cgo. It implements Codec
// for the "Oodle" method; Compress produces Kraken at level Normal.
type Oodle struct {
	path       string
	decompress *syscall.LazyProc
	compress   *syscall.LazyProc // nil if not exported
}

// OodleSupported reports whether this platform can load the Oodle DLL.
const OodleSupported = true

// LoadOodle loads the DLL at path and checks that it exports
// OodleLZ_Decompress (and, if present, that OodleLZ_Compress round-trips).
func LoadOodle(path string) (*Oodle, error) {
	if path == "" {
		return nil, errors.New("no Oodle DLL path given")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("Oodle DLL not found: %s", abs)
	}
	if !st.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(abs), ".dll") {
		return nil, fmt.Errorf("not a DLL file: %s", abs)
	}
	dll := syscall.NewLazyDLL(abs)
	if err := dll.Load(); err != nil {
		return nil, fmt.Errorf("cannot load %s (must be a 64-bit Oodle DLL): %v", abs, err)
	}
	o := &Oodle{path: abs, decompress: dll.NewProc("OodleLZ_Decompress")}
	if err := o.decompress.Find(); err != nil {
		return nil, fmt.Errorf("%s does not export OodleLZ_Decompress (not an Oodle DLL)", abs)
	}
	if c := dll.NewProc("OodleLZ_Compress"); c.Find() == nil {
		o.compress = c
		// self-test
		sample := bytes.Repeat([]byte("[/Script/Engine.Engine]\r\nbSmoothFrameRate=True\r\n"), 40)
		comp, err := o.Compress(MethodOodle, sample)
		if err != nil {
			return nil, fmt.Errorf("%s: Oodle self-test failed: %v", abs, err)
		}
		back, err := o.Decompress(MethodOodle, comp, len(sample))
		if err != nil || !bytes.Equal(back, sample) {
			return nil, fmt.Errorf("%s: Oodle self-test round trip failed: %v", abs, err)
		}
	}
	return o, nil
}

// Path returns the DLL path.
func (o *Oodle) Path() string { return o.path }

// CanCompress reports whether the DLL exports OodleLZ_Compress.
func (o *Oodle) CanCompress() bool { return o.compress != nil }

// Decompress implements Decompressor.
func (o *Oodle) Decompress(method string, comp []byte, rawLen int) ([]byte, error) {
	if !strings.EqualFold(method, MethodOodle) {
		return nil, fmt.Errorf("Oodle cannot decompress method %q", method)
	}
	if rawLen < 0 {
		return nil, errors.New("negative raw length")
	}
	raw := make([]byte, rawLen)
	if rawLen == 0 {
		return raw, nil
	}
	if len(comp) == 0 {
		return nil, errors.New("empty compressed block")
	}
	r, _, _ := o.decompress.Call(
		uintptr(unsafe.Pointer(&comp[0])), uintptr(len(comp)),
		uintptr(unsafe.Pointer(&raw[0])), uintptr(rawLen),
		oodleFuzzSafeYes, oodleCheckCRCNo, oodleVerbosityNone,
		0, 0, 0, 0, 0, 0, oodleDecodeUnthreaded)
	if int64(r) != int64(rawLen) {
		return nil, fmt.Errorf("OodleLZ_Decompress returned %d, want %d (corrupt data?)", int64(r), rawLen)
	}
	return raw, nil
}

// Compress implements Compressor (Kraken, level Normal, default options).
func (o *Oodle) Compress(method string, raw []byte) ([]byte, error) {
	if !strings.EqualFold(method, MethodOodle) {
		return nil, fmt.Errorf("Oodle cannot compress method %q", method)
	}
	if o.compress == nil {
		return nil, fmt.Errorf("%s does not export OodleLZ_Compress", o.path)
	}
	if len(raw) == 0 {
		return nil, errors.New("cannot compress an empty block")
	}
	// Generous worst-case bound (Oodle needs rawLen + a few hundred bytes
	// per 256 KiB chunk).
	buf := make([]byte, len(raw)+len(raw)/2+65536)
	r, _, _ := o.compress.Call(oodleCompressorKraken,
		uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)),
		uintptr(unsafe.Pointer(&buf[0])), oodleLevelNormal,
		0, 0, 0, 0, 0)
	n := int64(r)
	if n <= 0 || n > int64(len(buf)) {
		return nil, fmt.Errorf("OodleLZ_Compress failed (returned %d)", n)
	}
	return append([]byte(nil), buf[:n]...), nil
}
