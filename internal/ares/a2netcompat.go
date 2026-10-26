// .NET semantics helpers local to the Ares part-1 port (A2StationDbServer.cs,
// NetvarDumpStore.cs, NetvarOverridesFile.cs, SpecLevelsStore.cs,
// StationAcl.cs, DrafterPermissions.cs). Every identifier carries an "a2"
// prefix so it cannot collide with helpers of the DashboardServer port that
// shares package ares. The deployed backend runs with
// DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1, so culture-sensitive BCL calls use
// the invariant culture.
package ares

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
)

// a2Str returns a pointer to s (a non-null C# string).
func a2Str(s string) *string { return &s }

// a2Deref returns *s, or "" for null.
func a2Deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// a2Coalesce mirrors `a ?? b` for strings.
func a2Coalesce(a *string, b *string) *string {
	if a != nil {
		return a
	}
	return b
}

// a2IsNullOrEmpty mirrors string.IsNullOrEmpty.
func a2IsNullOrEmpty(s *string) bool { return s == nil || *s == "" }

// a2IsWhiteSpace mirrors char.IsWhiteSpace.
func a2IsWhiteSpace(r rune) bool {
	switch r {
	case 0x85, 0xA0:
		return true
	}
	return unicode.IsSpace(r) || unicode.Is(unicode.Zs, r) || r == 0x2028 || r == 0x2029
}

// a2IsNullOrWhiteSpace mirrors string.IsNullOrWhiteSpace.
func a2IsNullOrWhiteSpace(s *string) bool {
	if s == nil {
		return true
	}
	for _, r := range *s {
		if !a2IsWhiteSpace(r) {
			return false
		}
	}
	return true
}

// a2IsBlank is string.IsNullOrWhiteSpace for a non-null string.
func a2IsBlank(s string) bool { return a2IsNullOrWhiteSpace(&s) }

// a2Trim mirrors string.Trim() (char.IsWhiteSpace).
func a2Trim(s string) string { return strings.TrimFunc(s, a2IsWhiteSpace) }

// a2EqPtr mirrors C# `a == b` for strings (null equals only null).
func a2EqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// a2EqFoldPtr is a2EqPtr under LiteDB's case-insensitive collation; use it
// inside predicates handed to Find/FindOne/Count/Exists/DeleteMany.
func a2EqFoldPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return docdb.EqFold(*a, *b)
}

// a2EqFoldS compares a stored (nullable) string with a non-null value under
// LiteDB's collation.
func a2EqFoldS(a *string, b string) bool { return a != nil && docdb.EqFold(*a, b) }

// a2EqS compares a nullable string with a non-null value (ordinal, in memory).
func a2EqS(a *string, b string) bool { return a != nil && *a == b }

// Stored string lists / dictionary values are []*string / map[string]*string:
// LiteDB reads a stored "" back as null (see models), so elements and values
// can be null exactly as in the original.

// a2ContainsStr mirrors List<string>.Contains (ordinal, in memory;
// EqualityComparer<string>.Default: null matches a null element).
func a2ContainsStr(list []*string, x *string) bool {
	for _, s := range list {
		if a2EqPtr(s, x) {
			return true
		}
	}
	return false
}

// a2RemoveStr mirrors List<string>.Remove (first occurrence; null matches null).
func a2RemoveStr(list []*string, x *string) ([]*string, bool) {
	for i, s := range list {
		if a2EqPtr(s, x) {
			return append(list[:i:i], list[i+1:]...), true
		}
	}
	return list, false
}

// a2ContainsS / a2RemoveS are List<string>.Contains / Remove for lists that
// never come from LiteDB (e.g. the SpecLevelsStore JSON index) with a
// non-null needle.
func a2ContainsS(list []string, x string) bool {
	for _, s := range list {
		if s == x {
			return true
		}
	}
	return false
}

func a2RemoveS(list []string, x string) ([]string, bool) {
	for i, s := range list {
		if s == x {
			return append(list[:i:i], list[i+1:]...), true
		}
	}
	return list, false
}

// a2StrSet mirrors HashSet<string> with the default (ordinal) comparer,
// which can hold null.
type a2StrSet struct {
	m       map[string]bool
	hasNull bool
}

func (s *a2StrSet) Add(x *string) {
	if x == nil {
		s.hasNull = true
		return
	}
	if s.m == nil {
		s.m = map[string]bool{}
	}
	s.m[*x] = true
}

func (s *a2StrSet) Contains(x *string) bool {
	if x == nil {
		return s.hasNull
	}
	return s.m[*x]
}

// a2Strs converts non-null C# string literals to a []*string (List<string>).
func a2Strs(ss ...string) []*string {
	out := make([]*string, len(ss))
	for i := range ss {
		out[i] = a2Str(ss[i])
	}
	return out
}

// a2CopyMap mirrors new Dictionary<string, string>(source) (null values kept).
func a2CopyMap(m map[string]*string) map[string]*string {
	out := make(map[string]*string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// a2TryAdd mirrors Dictionary.TryAdd: a key holding a null value is still
// present, so it is not replaced.
func a2TryAdd(m map[string]*string, k string, v *string) {
	if _, ok := m[k]; !ok {
		m[k] = v
	}
}

// a2Query mirrors IHttpRequest.GetQueryParameter(key) (null when absent).
func a2Query(c *hosting.Context, key string) *string {
	if v, ok := c.Request.Query(key); ok {
		return &v
	}
	return nil
}

// a2QueryOr mirrors IHttpRequest.GetQueryParameter(key, defaultValue).
func a2QueryOr(c *hosting.Context, key, def string) string {
	return c.Request.GetQueryParameter(key, def)
}

// a2Header mirrors IHttpRequest.GetHeaderValue(key) (null when absent).
func a2Header(c *hosting.Context, key string) *string {
	if v, ok := c.Request.Header(key); ok {
		return &v
	}
	return nil
}

// a2IsNumberWhite is the whitespace set of .NET number/bool parsing.
func a2IsNumberWhite(r rune) bool { return r == 0x20 || (r >= 0x09 && r <= 0x0D) }

// a2ParseInt mirrors int.Parse(s) (NumberStyles.Integer, invariant culture):
// panics with FormatException / OverflowException like the original.
func a2ParseInt(s string) int {
	t := strings.TrimFunc(s, a2IsNumberWhite)
	t = strings.TrimRight(t, "\x00")
	t = strings.TrimFunc(t, a2IsNumberWhite)
	neg := false
	if strings.HasPrefix(t, "-") {
		neg, t = true, t[1:]
	} else if strings.HasPrefix(t, "+") {
		t = t[1:]
	}
	if t == "" {
		dotnet.Throw("System.FormatException", "Input string was not in a correct format.")
	}
	for i := 0; i < len(t); i++ {
		if t[i] < '0' || t[i] > '9' {
			dotnet.Throw("System.FormatException", "Input string was not in a correct format.")
		}
	}
	t = strings.TrimLeft(t, "0")
	if len(t) > 10 {
		dotnet.Throw("System.OverflowException", "Value was either too large or too small for an Int32.")
	}
	n, _ := strconv.ParseInt("0"+t, 10, 64)
	if neg {
		n = -n
	}
	if n > math.MaxInt32 || n < math.MinInt32 {
		dotnet.Throw("System.OverflowException", "Value was either too large or too small for an Int32.")
	}
	return int(n)
}

// a2TryParseInt mirrors int.TryParse(s, out v).
func a2TryParseInt(s *string) (n int, ok bool) {
	if s == nil {
		return 0, false
	}
	defer func() {
		if recover() != nil {
			n, ok = 0, false
		}
	}()
	return a2ParseInt(*s), true
}

// a2TrimBool trims what bool.Parse ignores (whitespace and NUL).
func a2TrimBool(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return a2IsWhiteSpace(r) || r == 0 })
}

// a2TryParseBool mirrors bool.TryParse.
func a2TryParseBool(s string) (bool, bool) {
	t := a2TrimBool(s)
	switch {
	case strings.EqualFold(t, "True"):
		return true, true
	case strings.EqualFold(t, "False"):
		return false, true
	}
	return false, false
}

// a2ParseBool mirrors bool.Parse (FormatException on anything else).
func a2ParseBool(s string) bool {
	b, ok := a2TryParseBool(s)
	if !ok {
		dotnet.Throw("System.FormatException", "String '"+s+"' was not recognized as a valid Boolean.")
	}
	return b
}

// a2TryParseDouble mirrors double.TryParse(s, NumberStyles.Any,
// CultureInfo.InvariantCulture, out d) on .NET 6: leading/trailing white,
// leading or trailing sign, parentheses, '.' decimal point, ',' group
// separators after the first digit, exponent, the invariant currency symbol
// U+00A4, trailing NULs, case-insensitive "NaN"/"Infinity" symbols, and
// overflow to +/-Infinity.
func a2TryParseDouble(s string) (float64, bool) {
	if v, ok := a2ParseNumberAny(s); ok {
		return v, true
	}
	// .NET Core 3.0+: special symbols (whitespace allowed per styles).
	t := strings.TrimRight(s, "\x00")
	t = strings.TrimFunc(t, a2IsNumberWhite)
	switch {
	case strings.EqualFold(t, "Infinity"), strings.EqualFold(t, "+Infinity"):
		return math.Inf(1), true
	case strings.EqualFold(t, "-Infinity"):
		return math.Inf(-1), true
	case strings.EqualFold(t, "NaN"), strings.EqualFold(t, "+NaN"), strings.EqualFold(t, "-NaN"):
		return math.NaN(), true
	}
	return 0, false
}

func a2ParseNumberAny(s string) (float64, bool) {
	const currency = "¤" // U+00A4, the invariant currency symbol
	r := []rune(s)
	i, n := 0, len(r)
	signSeen, neg, paren, cur := false, false, false, false
	// leading part
	for i < n {
		c := r[i]
		switch {
		case a2IsNumberWhite(c):
			i++
		case !signSeen && (c == '+' || c == '-'):
			signSeen, neg = true, c == '-'
			i++
		case !signSeen && !paren && c == '(':
			signSeen, paren, neg = true, true, true
			i++
		case !cur && string(c) == currency:
			cur = true
			i++
		default:
			goto digits
		}
	}
digits:
	var mant strings.Builder
	exp := 0
	digitsSeen, decSeen := false, false
	for i < n {
		c := r[i]
		switch {
		case c >= '0' && c <= '9':
			digitsSeen = true
			mant.WriteRune(c)
			if decSeen {
				exp--
			}
			i++
		case c == '.' && !decSeen:
			decSeen = true
			i++
		case c == ',' && digitsSeen && !decSeen:
			i++
		default:
			goto expo
		}
	}
expo:
	if !digitsSeen {
		return 0, false
	}
	if i < n && (r[i] == 'e' || r[i] == 'E') {
		j := i + 1
		eneg := false
		if j < n && (r[j] == '+' || r[j] == '-') {
			eneg = r[j] == '-'
			j++
		}
		if j < n && r[j] >= '0' && r[j] <= '9' {
			e := 0
			for j < n && r[j] >= '0' && r[j] <= '9' {
				if e < 100000 {
					e = e*10 + int(r[j]-'0')
				}
				j++
			}
			if eneg {
				e = -e
			}
			exp += e
			i = j
		}
	}
	// trailing part
	for i < n {
		c := r[i]
		switch {
		case a2IsNumberWhite(c):
			i++
		case !signSeen && (c == '+' || c == '-'):
			signSeen, neg = true, c == '-'
			i++
		case paren && c == ')':
			paren = false
			i++
		case !cur && string(c) == currency:
			cur = true
			i++
		case c == 0:
			// trailing NULs are ignored
			for i < n && r[i] == 0 {
				i++
			}
			if i != n {
				return 0, false
			}
		default:
			return 0, false
		}
	}
	if paren {
		return 0, false
	}
	m := strings.TrimLeft(mant.String(), "0")
	if m == "" {
		if neg {
			return math.Copysign(0, -1), true
		}
		return 0, true
	}
	v, err := strconv.ParseFloat(m+"e"+strconv.Itoa(exp), 64)
	if err != nil && !math.IsInf(v, 0) {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

// a2ParseDateTime mirrors DateTime.Parse(s) under the invariant culture for
// the formats the original's callers send (ISO-8601 with optional fraction
// and Z/offset, "yyyy-MM-dd HH:mm:ss", "MM/dd/yyyy[ HH:mm[:ss]]",
// "yyyy/MM/dd"). A Z or offset designator yields a Local-kind value
// converted to local time, as DateTime.Parse does; otherwise Unspecified.
func a2ParseDateTime(s string) dotnet.DateTime {
	t := a2Trim(s)
	zoned := []string{
		"2006-01-02T15:04:05.9999999Z07:00", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04Z07:00",
		"2006-01-02 15:04:05.9999999Z07:00", "2006-01-02 15:04:05Z07:00", "2006-01-02T15:04:05.9999999Z0700",
		"2006-01-02T15:04:05Z0700",
	}
	for _, l := range zoned {
		if tm, err := time.Parse(l, t); err == nil {
			return dotnet.FromTime(tm, dotnet.Local)
		}
	}
	plain := []string{
		"2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05", "2006-01-02T15:04",
		"2006-01-02 15:04:05.9999999", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02",
		"1/2/2006 15:04:05", "1/2/2006 15:04", "1/2/2006", "1/2/2006 3:04:05 PM", "1/2/2006 3:04 PM",
		"2006/1/2 15:04:05", "2006/1/2", "2006-1-2",
	}
	for _, l := range plain {
		if tm, err := time.Parse(l, t); err == nil {
			return dotnet.FromTime(tm, dotnet.Unspecified)
		}
	}
	dotnet.Throw("System.FormatException", "String '"+s+"' was not recognized as a valid DateTime.")
	return dotnet.DateTime{}
}

// a2RoundEven mirrors Math.Round(double) (banker's rounding).
func a2RoundEven(v float64) float64 { return math.RoundToEven(v) }

// a2UTF16Len mirrors string.Length.
func a2UTF16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// a2CurrentDir mirrors Environment.CurrentDirectory.
func a2CurrentDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// a2BaseDirectory mirrors AppContext.BaseDirectory (the executable's folder).
func a2BaseDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// a2InvalidFileNameChar mirrors Path.GetInvalidFileNameChars(): on Windows
// NUL..US plus " < > | : * ? \ /; on Unix only NUL and /.
func a2InvalidFileNameChar(c rune) bool {
	if runtime.GOOS != "windows" {
		return c == 0 || c == '/'
	}
	if c < 32 {
		return true
	}
	switch c {
	case '"', '<', '>', '|', ':', '*', '?', '\\', '/':
		return true
	}
	return false
}

// a2ExceptionMessage renders a recovered panic / error as Exception.Message.
func a2ExceptionMessage(p any) string { return hosting.ExceptionMessage(p) }

// a2CaseInsensitiveFS reports whether file names compare case-insensitively
// (Windows), as Directory.GetFiles pattern matching does there.
var a2CaseInsensitiveFS = runtime.GOOS == "windows"

// a2PanicError carries a recovered panic as an error (a caught .NET exception).
type a2PanicError struct{ p any }

func (e a2PanicError) Error() string { return hosting.ExceptionMessage(e.p) }

// a2ErrMessage renders Exception.Message for a caught exception.
func a2ErrMessage(err error) string {
	if pe, ok := err.(a2PanicError); ok {
		return hosting.ExceptionMessage(pe.p)
	}
	return a2IOMessage(err)
}

// a2NullReference builds the NullReferenceException the original would throw.
func a2NullReference() *dotnet.Exception {
	return &dotnet.Exception{Type: "System.NullReferenceException", Message: "Object reference not set to an instance of an object."}
}

// a2IsLetterOrDigit mirrors char.IsLetterOrDigit (categories L* and Nd).
func a2IsLetterOrDigit(c rune) bool { return unicode.IsLetter(c) || unicode.Is(unicode.Nd, c) }

// a2Itoa mirrors int.ToString() (invariant).
func a2Itoa(n int) string { return strconv.Itoa(n) }
