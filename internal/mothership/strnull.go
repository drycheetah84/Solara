package mothership

import (
	"strings"

	"solara/internal/mothership/models"
)

// Helpers for the C# `string` members of the stored models, which are *string
// in Go because a reloaded "" is null (LiteDB EmptyStringToNull).

// sv renders a possibly-null C# string as interpolation/concatenation does (null -> "").
func sv(p *string) string { return models.S(p) }

// eqP mirrors C# string == on two possibly-null strings (ordinal; LINQ to objects).
func eqP(a, b *string) bool { return models.Eq(a, b) }

// eqS mirrors `a == "value"` (ordinal) for a possibly-null a and a non-null value.
func eqS(a *string, b string) bool { return models.EqS(a, b) }

// isNullOrEmpty mirrors string.IsNullOrEmpty.
func isNullOrEmpty(p *string) bool { return models.IsNullOrEmpty(p) }

// dbEq mirrors `x.Member == value` inside a LiteDB predicate (Find/FindOne/
// Count/Exists/DeleteMany): IgnoreCase collation, and the captured value is
// compared verbatim (not trimmed, "" not turned into null), so a stored null
// never equals a non-null value (verified with LiteDB 5.0.21, see
// tools/oracle-mothership-db).
func dbEq(member *string, value string) bool {
	return member != nil && strings.EqualFold(*member, value)
}

// dbEqP is dbEq for a possibly-null captured value: null equals only null.
func dbEqP(member, value *string) bool {
	if member == nil || value == nil {
		return member == nil && value == nil
	}
	return strings.EqualFold(*member, *value)
}

// strPtrs converts a List<string> without nulls to its stored representation.
func strPtrs(xs []string) []*string { return models.L(xs...) }

// orStr mirrors `p ?? def`.
func orStr(p *string, def string) string {
	if p == nil {
		return def
	}
	return *p
}
