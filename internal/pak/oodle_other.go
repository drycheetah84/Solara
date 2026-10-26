//go:build !windows

package pak

import "errors"

// OodleSupported reports whether this platform can load the Oodle DLL.
const OodleSupported = false

var errOodlePlatform = errors.New("Oodle unavailable: loading the Oodle DLL is only supported on Windows")

// Oodle is unavailable on this platform; LoadOodle always fails.
type Oodle struct{ path string }

// LoadOodle always fails on non-Windows platforms.
func LoadOodle(path string) (*Oodle, error) { return nil, errOodlePlatform }

// Path returns the DLL path.
func (o *Oodle) Path() string { return o.path }

// CanCompress always returns false.
func (o *Oodle) CanCompress() bool { return false }

// Decompress always fails.
func (o *Oodle) Decompress(string, []byte, int) ([]byte, error) { return nil, errOodlePlatform }

// Compress always fails.
func (o *Oodle) Compress(string, []byte) ([]byte, error) { return nil, errOodlePlatform }
