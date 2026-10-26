// .NET semantics helpers local to the HalcyonSocket port (AgentHub.cs,
// HalcyonSocketServer.cs, ServerWatchdog.cs). They reproduce the exact BCL
// behaviour the original relied on (invariant-globalization mode, as deployed:
// install.sh sets DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1).
package halcyonsocket

import (
	"fmt"

	"math"
	"solara/internal/logging"
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// ConcurrentDictionary mirrors System.Collections.Concurrent.ConcurrentDictionary
// for the members the original code (and its callers in Ares) use. Every
// operation is atomic, as in the original.
type ConcurrentDictionary[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]V
}

// NewConcurrentDictionary mirrors new ConcurrentDictionary<K, V>().
func NewConcurrentDictionary[K comparable, V any]() *ConcurrentDictionary[K, V] {
	return &ConcurrentDictionary[K, V]{m: map[K]V{}}
}

// Set mirrors the indexer setter (add or overwrite).
func (d *ConcurrentDictionary[K, V]) Set(k K, v V) {
	d.mu.Lock()
	d.m[k] = v
	d.mu.Unlock()
}

// TryGetValue mirrors ConcurrentDictionary.TryGetValue.
func (d *ConcurrentDictionary[K, V]) TryGetValue(k K) (V, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.m[k]
	return v, ok
}

// TryRemove mirrors ConcurrentDictionary.TryRemove(key, out value).
func (d *ConcurrentDictionary[K, V]) TryRemove(k K) (V, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.m[k]
	if ok {
		delete(d.m, k)
	}
	return v, ok
}

// ContainsKey mirrors ConcurrentDictionary.ContainsKey.
func (d *ConcurrentDictionary[K, V]) ContainsKey(k K) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.m[k]
	return ok
}

// Count mirrors ConcurrentDictionary.Count.
func (d *ConcurrentDictionary[K, V]) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.m)
}

// Clear mirrors ConcurrentDictionary.Clear.
func (d *ConcurrentDictionary[K, V]) Clear() {
	d.mu.Lock()
	d.m = map[K]V{}
	d.mu.Unlock()
}

// isWhiteSpace mirrors char.IsWhiteSpace. Go's unicode.IsSpace covers the same
// set (Zs/Zl/Zp, U+0009..U+000D, U+0085, U+00A0).
func isWhiteSpace(r rune) bool { return unicode.IsSpace(r) }

// isNullOrWhiteSpace mirrors string.IsNullOrWhiteSpace for a non-null string.
func isNullOrWhiteSpace(s string) bool {
	for _, r := range s {
		if !isWhiteSpace(r) {
			return false
		}
	}
	return true
}

// isNullOrWhiteSpaceP mirrors string.IsNullOrWhiteSpace for a nullable string.
func isNullOrWhiteSpaceP(s *string) bool { return s == nil || isNullOrWhiteSpace(*s) }

// isNullOrEmptyP mirrors string.IsNullOrEmpty.
func isNullOrEmptyP(s *string) bool { return s == nil || *s == "" }

// trim mirrors string.Trim() (char.IsWhiteSpace at both ends).
func trim(s string) string { return strings.TrimFunc(s, isWhiteSpace) }

// deref renders a nullable string the way C# string interpolation and `?? ""` do.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func strp(s string) *string { return &s }

// parseInt32 mirrors int.TryParse(s, out v) with NumberStyles.Integer under the
// invariant culture: optional leading/trailing white (U+0009..U+000D, U+0020),
// an optional '+'/'-' sign, ASCII digits, int32 range, and trailing NUL chars
// tolerated (a .NET number-parsing quirk).
func parseInt32(s string) (int, bool) {
	isWhite := func(c byte) bool { return c == 0x20 || (c >= 0x09 && c <= 0x0D) }
	i, n := 0, len(s)
	for i < n && isWhite(s[i]) {
		i++
	}
	neg := false
	if i < n && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	start := i
	var v int64
	for i < n && s[i] >= '0' && s[i] <= '9' {
		v = v*10 + int64(s[i]-'0')
		if v > 1<<31 {
			return 0, false
		}
		i++
	}
	if i == start {
		return 0, false
	}
	for i < n && isWhite(s[i]) {
		i++
	}
	for ; i < n; i++ {
		if s[i] != 0 {
			return 0, false
		}
	}
	if neg {
		v = -v
	}
	if v > math.MaxInt32 || v < math.MinInt32 {
		return 0, false
	}
	return int(v), true
}

// decodeUTF8 mirrors Encoding.UTF8.GetString(buf, 0, n): invalid input is
// replaced by U+FFFD, one per maximal subpart of an ill-formed sequence. The
// original decodes every 4096-byte read on its own (no Decoder state), so a
// multi-byte character split across two reads becomes replacement characters;
// callers reproduce that by decoding each chunk separately.
func decodeUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r != utf8.RuneError || size > 1 {
			sb.Write(b[i : i+size])
			i += size
			continue
		}
		sb.WriteRune(utf8.RuneError)
		i += maximalSubpart(b[i:])
	}
	return sb.String()
}

// maximalSubpart returns the length of the maximal subpart of the ill-formed
// sequence starting at b[0] (Unicode 3.9, "U+FFFD substitution of maximal subparts").
func maximalSubpart(b []byte) int {
	b0 := b[0]
	var need int
	lo, hi := byte(0x80), byte(0xBF)
	switch {
	case b0 >= 0xC2 && b0 <= 0xDF:
		need = 1
	case b0 == 0xE0:
		need, lo = 2, 0xA0
	case b0 >= 0xE1 && b0 <= 0xEC, b0 == 0xEE, b0 == 0xEF:
		need = 2
	case b0 == 0xED:
		need, hi = 2, 0x9F
	case b0 == 0xF0:
		need, lo = 3, 0x90
	case b0 >= 0xF1 && b0 <= 0xF3:
		need = 3
	case b0 == 0xF4:
		need, hi = 3, 0x8F
	default:
		return 1
	}
	n := 1
	for k := 0; k < need && n < len(b); k++ {
		c := b[n]
		if c < lo || c > hi {
			break
		}
		n++
		lo, hi = 0x80, 0xBF
	}
	return n
}

// compareOrdinal mirrors string.CompareOrdinal (UTF-16 code-unit order), which
// is what the default string comparer does in invariant-globalization mode.
func compareOrdinal(a, b string) int {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			if ua[i] < ub[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ua) < len(ub):
		return -1
	case len(ua) > len(ub):
		return 1
	}
	return 0
}

// containsASCIIFold mirrors s.Contains(sub, StringComparison.OrdinalIgnoreCase)
// for an ASCII needle in invariant mode (.NET 6 invariant mode folds ASCII only).
func containsASCIIFold(s, sub string) bool {
	lower := func(c byte) byte {
		if c >= 'A' && c <= 'Z' {
			return c + 32
		}
		return c
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			if lower(s[i+j]) != lower(sub[j]) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// formatF0 mirrors double.ToString("F0"). Verified against .NET 6 (oracle):
// correctly rounded, midpoint to even (2.5 -> "2", 1.5 -> "2"), and a
// negative value rounding to zero keeps its sign ("-0") -- exactly Go's %.0f.
func formatF0(v float64) string { return fmt.Sprintf("%.0f", v) }

// consoleWriteLine is Console.WriteLine (swappable by tests to capture output).
var consoleWriteLine = func(s string) { logging.Console(s) }

// utf16Prefix mirrors s[..n] on a .NET (UTF-16) string; a split surrogate pair
// is emitted as U+FFFD, as the UTF-8 console encoder does.
func utf16Prefix(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= n {
		return s
	}
	return string(utf16.Decode(u[:n]))
}

// utf16Len mirrors string.Length.
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// msBetween mirrors (a - b).TotalMilliseconds on two DateTimes.
func msBetween(a, b int64) float64 { return float64(a-b) / 10000.0 }
