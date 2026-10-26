// Small .NET-semantics helpers used by the AresDashboardServer port
// (DashboardServer.cs / DashboardAuthPreprocessor.cs / the dashboard half of
// DrafterPermissions.cs). Prefixed "dash" to stay clear of helpers that other
// files of package ares may define.
package ares

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"

	"solara/internal/stj"
)

// dashIsNullOrWhiteSpace mirrors string.IsNullOrWhiteSpace for a non-null string.
func dashIsNullOrWhiteSpace(s string) bool { return strings.TrimFunc(s, unicode.IsSpace) == "" }

// dashIsNullOrWhiteSpaceP mirrors string.IsNullOrWhiteSpace(string?).
func dashIsNullOrWhiteSpaceP(s *string) bool { return s == nil || dashIsNullOrWhiteSpace(*s) }

// dashTrim mirrors string.Trim() (char.IsWhiteSpace).
func dashTrim(s string) string { return strings.TrimFunc(s, unicode.IsSpace) }

// dashHasPrefixFoldOrdinal mirrors s.StartsWith(prefix, OrdinalIgnoreCase) for an
// ASCII prefix.
func dashHasPrefixFoldOrdinal(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if dashUpperASCII(s[i]) != dashUpperASCII(prefix[i]) {
			return false
		}
	}
	return true
}

func dashUpperASCII(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

// dashUpperInvariant mirrors the per-char simple upper-casing OrdinalIgnoreCase uses.
func dashUpperInvariant(s string) []uint16 {
	r := []rune(s)
	for i, c := range r {
		r[i] = unicode.ToUpper(c)
	}
	return utf16.Encode(r)
}

// dashCompareOrdinalIgnoreCase mirrors StringComparer.OrdinalIgnoreCase.Compare.
func dashCompareOrdinalIgnoreCase(a, b string) int {
	ua, ub := dashUpperInvariant(a), dashUpperInvariant(b)
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

// dashEqualsOrdinalIgnoreCase mirrors string.Equals(a, b, OrdinalIgnoreCase).
func dashEqualsOrdinalIgnoreCase(a, b string) bool { return dashCompareOrdinalIgnoreCase(a, b) == 0 }

func dashStrPtr(s string) *string { return &s }

// dashDeref renders a string? the way C# interpolation does (null -> "").
func dashDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// dashEqP mirrors the C# `a == b` on two string? values (ordinal; null == null).
func dashEqP(a *string, b string) bool { return a != nil && *a == b }

// dashLiteEq mirrors `x.Prop == value` evaluated by LiteDB inside a
// Find/FindOne predicate: case-insensitive collation; a null (missing) member
// only matches a null value.
func dashLiteEq(a *string, b string) bool { return a != nil && strings.EqualFold(*a, b) }

// dashLiteEqPP is dashLiteEq when the compared value may itself be null.
func dashLiteEqPP(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return strings.EqualFold(*a, *b)
}

// dashListRemove mirrors List<string>.Remove (first ordinal match).
// Stored role lists are []*string (LiteDB reads a stored "" back as null); a
// null element never equals the non-null needle.
func dashListRemove(list []*string, x string) ([]*string, bool) {
	for i, v := range list {
		if v != nil && *v == x {
			return append(list[:i], list[i+1:]...), true
		}
	}
	return list, false
}

// dashListContains mirrors List<string>.Contains (ordinal).
func dashListContains(list []*string, x string) bool {
	for _, v := range list {
		if v != nil && *v == x {
			return true
		}
	}
	return false
}

// dashDeserialize mirrors `try { body = JsonSerializer.Deserialize<T>(bytes); }
// catch { body = null; }`: a literal JSON null yields nil; a fresh instance
// (with the C# property initializers applied by newT) is populated otherwise.
func dashDeserialize[T any](data []byte, newT func() *T) *T {
	el, err := stj.Parse(data)
	if err != nil {
		return nil
	}
	if el.ValueKind() == stj.Null {
		return nil
	}
	v := newT()
	if err := stj.FromElement(el, v); err != nil {
		return nil
	}
	return v
}

// dashDeserializeStringDict mirrors Deserialize<Dictionary<string, string>>:
// an object of string (or null) values, insertion ordered, duplicate keys
// overwrite in place. ok=false where the C# threw (caught -> null).
func dashDeserializeStringDict(data []byte) (keys []string, vals map[string]*string, ok bool) {
	el, err := stj.Parse(data)
	if err != nil {
		return nil, nil, false
	}
	if el.ValueKind() == stj.Null {
		return nil, nil, false
	}
	if el.ValueKind() != stj.Object {
		return nil, nil, false
	}
	vals = map[string]*string{}
	for _, p := range el.EnumerateObject() {
		var v *string
		switch p.Value.ValueKind() {
		case stj.String:
			v = p.Value.GetString()
		case stj.Null:
			v = nil
		default:
			return nil, nil, false
		}
		if _, seen := vals[p.Name]; !seen {
			keys = append(keys, p.Name)
		}
		vals[p.Name] = v
	}
	return keys, vals, true
}

// dashStableSort sorts in place like LINQ OrderBy/OrderByDescending (stable).
func dashStableSort[T any](s []T, less func(a, b T) bool) {
	sort.SliceStable(s, func(i, j int) bool { return less(s[i], s[j]) })
}

// dashEscapeDataString mirrors Uri.EscapeDataString (RFC 3986 unreserved kept).
func dashEscapeDataString(s string) string {
	const hexd = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hexd[c>>4])
		b.WriteByte(hexd[c&15])
	}
	return b.String()
}
