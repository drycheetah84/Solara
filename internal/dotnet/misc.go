package dotnet

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	mrand "math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Exception models a .NET exception that escapes to the hosting layer. Its
// Type and Message reproduce what the original surfaced in error pages.
type Exception struct {
	Type    string
	Message string
}

func (e *Exception) Error() string { return e.Message }

// String mirrors Exception.ToString() without the stack trace (stack traces
// are an implementation artifact; see KNOWN_ISSUES.md).
func (e *Exception) String() string { return e.Type + ": " + e.Message }

// Throw panics with a .NET-style exception.
func Throw(typ, msg string) { panic(&Exception{Type: typ, Message: msg}) }

// NewGuid mirrors Guid.NewGuid().ToString() ("D" format, lower case).
func NewGuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewGuidN mirrors Guid.NewGuid().ToString("N").
func NewGuidN() string { return strings.ReplaceAll(NewGuid(), "-", "") }

// FormatGuid renders 16 bytes in .NET's Guid byte order as "D" format. The
// first three groups of a .NET Guid are little-endian.
func FormatGuid(b [16]byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%x-%x",
		binary.LittleEndian.Uint32(b[0:4]), binary.LittleEndian.Uint16(b[4:6]),
		binary.LittleEndian.Uint16(b[6:8]), b[8:10], b[10:16])
}

// TryParseGuid mirrors Guid.TryParse for the common D/N/B formats.
func TryParseGuid(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimSuffix(s, "}"), "{")
	if len(s) == 36 {
		if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
			return false
		}
		s = strings.ReplaceAll(s, "-", "")
	}
	if len(s) != 32 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// CreateRandomString mirrors StringExtensions.CreateRandomString.
func CreateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[mrand.IntN(len(charset))]
	}
	return string(b)
}

// RandomNext mirrors Random.Shared.Next(min, max) (max exclusive).
func RandomNext(min, max int) int {
	if max <= min {
		return min
	}
	return min + mrand.IntN(max-min)
}

// --- LiteDB ObjectId --------------------------------------------------------

// ObjectId mirrors LiteDB.ObjectId (12 bytes: timestamp, machine, pid, increment).
type ObjectId [12]byte

var (
	oidMachine   = func() [3]byte { h, _ := os.Hostname(); s := md5.Sum([]byte(h)); return [3]byte{s[0], s[1], s[2]} }()
	oidPid       = uint16(os.Getpid())
	oidIncrement = func() uint32 { var b [4]byte; _, _ = rand.Read(b[:]); return binary.BigEndian.Uint32(b[:]) }()
)

// NewObjectId mirrors ObjectId.NewObjectId().
func NewObjectId() ObjectId {
	var id ObjectId
	binary.BigEndian.PutUint32(id[0:4], uint32(time.Now().Unix()))
	copy(id[4:7], oidMachine[:])
	binary.BigEndian.PutUint16(id[7:9], oidPid)
	inc := atomic.AddUint32(&oidIncrement, 1) & 0xffffff
	id[9], id[10], id[11] = byte(inc>>16), byte(inc>>8), byte(inc)
	return id
}

func (o ObjectId) Hex() string    { return hex.EncodeToString(o[:]) }
func (o ObjectId) String() string { return o.Hex() }
func (o ObjectId) IsZero() bool   { return o == ObjectId{} }

// ParseObjectId parses 24 hex characters.
func ParseObjectId(s string) (ObjectId, error) {
	var o ObjectId
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 12 {
		return o, fmt.Errorf("invalid ObjectId %q", s)
	}
	copy(o[:], b)
	return o, nil
}

// --- number formatting -----------------------------------------------------

// FormatDouble renders a float64 the way .NET Core 3+ double.ToString()/"R"
// and System.Text.Json do: shortest round-trip digits, scientific notation
// when the decimal point position exceeds 17 or is below -3, exponent "E+dd".
func FormatDouble(v float64) string { return formatFloat(v, 64, 17) }

// FormatSingle renders a float32 as .NET float.ToString() does.
func FormatSingle(v float32) string { return formatFloat(float64(v), 32, 9) }

func formatFloat(v float64, bits int, maxDigits int) string {
	if math.IsNaN(v) {
		return "NaN"
	}
	if math.IsInf(v, 1) {
		return "∞"
	}
	if math.IsInf(v, -1) {
		return "-∞"
	}
	e := strconv.FormatFloat(v, 'e', -1, bits) // e.g. -1.2345e+17
	neg := strings.HasPrefix(e, "-")
	e = strings.TrimPrefix(e, "-")
	mant, expStr, _ := strings.Cut(e, "e")
	exp, _ := strconv.Atoi(expStr)
	digits := strings.Replace(mant, ".", "", 1)
	digPos := exp + 1
	var out string
	if v == 0 {
		out = "0"
	} else if digPos > maxDigits || digPos < -3 {
		out = digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		es := "+"
		if exp < 0 {
			es = "-"
			exp = -exp
		}
		out += fmt.Sprintf("E%s%02d", es, exp)
	} else if digPos <= 0 {
		out = "0." + strings.Repeat("0", -digPos) + digits
	} else if digPos >= len(digits) {
		out = digits + strings.Repeat("0", digPos-len(digits))
	} else {
		out = digits[:digPos] + "." + digits[digPos:]
	}
	if neg {
		out = "-" + out
	}
	return out
}

// FormatBool mirrors bool.ToString() ("True"/"False").
func FormatBool(b bool) string {
	if b {
		return "True"
	}
	return "False"
}
