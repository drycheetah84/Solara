package models

import "reflect"

// C# `string` members of the stored models are *string: LiteDB's BsonMapper
// (TrimWhitespace, EmptyStringToNull) stores "" as an explicit null which, on
// read, overrides the `= ""` initializer, so a reloaded member can be null.

// P returns a pointer to s (a non-null C# string).
func P(s string) *string { return &s }

// S renders a C# string the way interpolation / concatenation does: null -> "".
func S(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Eq mirrors C# string equality (==): null == null, null != any value.
func Eq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// EqS mirrors `a == "literal"` for a possibly-null a.
func EqS(a *string, b string) bool { return a != nil && *a == b }

// IsNullOrEmpty mirrors string.IsNullOrEmpty.
func IsNullOrEmpty(p *string) bool { return p == nil || *p == "" }

// L converts a List<string> without nulls to its stored representation.
func L(xs ...string) []*string {
	out := make([]*string, len(xs))
	for i := range xs {
		out[i] = P(xs[i])
	}
	return out
}

var strPtrType = reflect.TypeOf((*string)(nil))

// initStrings applies the `= ""` initializers: every nil *string member of the
// struct v points to "" (members with other initializers are set beforehand).
func initStrings(v any) {
	rv := reflect.ValueOf(v).Elem()
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if f.Type() == strPtrType && f.IsNil() && f.CanSet() {
			f.Set(reflect.ValueOf(P("")))
		}
	}
}
