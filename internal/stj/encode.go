package stj

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"solara/internal/dotnet"
)

// Marshaler lets a type control its System.Text.Json representation.
type Marshaler interface {
	MarshalSTJ() ([]byte, error)
}

// Raw is pre-encoded JSON emitted verbatim.
type Raw []byte

// KV is one member of an ordered object.
type KV struct {
	K string
	V any
}

// Obj is an ordered JSON object, the Go stand-in for C# anonymous objects
// (`new { a = 1, b = "x" }`) and insertion-ordered Dictionary<string, T>.
type Obj []KV

// O builds an Obj from alternating key/value arguments.
func O(kv ...any) Obj {
	if len(kv)%2 != 0 {
		panic("stj.O: odd argument count")
	}
	o := make(Obj, 0, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		o = append(o, KV{kv[i].(string), kv[i+1]})
	}
	return o
}

// Set replaces or appends key k (Dictionary indexer semantics: an existing
// key keeps its position).
func (o *Obj) Set(k string, v any) {
	for i := range *o {
		if (*o)[i].K == k {
			(*o)[i].V = v
			return
		}
	}
	*o = append(*o, KV{k, v})
}

// Get returns the value for k.
func (o Obj) Get(k string) (any, bool) {
	for _, kv := range o {
		if kv.K == k {
			return kv.V, true
		}
	}
	return nil, false
}

// Marshal mirrors JsonSerializer.Serialize(value) with default options.
func Marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	if err := encode(&b, reflect.ValueOf(v)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// MustMarshal panics (as a .NET exception would) on failure.
func MustMarshal(v any) []byte {
	b, err := Marshal(v)
	if err != nil {
		dotnet.Throw("System.NotSupportedException", err.Error())
	}
	return b
}

// MarshalString is MustMarshal as a string.
func MarshalString(v any) string { return string(MustMarshal(v)) }

var (
	marshalerType = reflect.TypeOf((*Marshaler)(nil)).Elem()
	jsonRawType   = reflect.TypeOf(json.RawMessage(nil))
	rawType       = reflect.TypeOf(Raw(nil))
)

func encode(b *bytes.Buffer, v reflect.Value) error {
	if !v.IsValid() {
		b.WriteString("null")
		return nil
	}
	t := v.Type()
	if t == rawType || t == jsonRawType {
		if v.IsNil() {
			b.WriteString("null")
		} else {
			b.Write(v.Bytes())
		}
		return nil
	}
	if t.Implements(marshalerType) {
		if (t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface) && v.IsNil() {
			b.WriteString("null")
			return nil
		}
		out, err := v.Interface().(Marshaler).MarshalSTJ()
		if err != nil {
			return err
		}
		b.Write(out)
		return nil
	}
	if t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(marshalerType) && v.CanAddr() {
		return encode(b, v.Addr())
	}
	switch t.Kind() {
	case reflect.Bool:
		if v.Bool() {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32:
		f := v.Float()
		if f != f || f > 3.4028234663852886e38 || f < -3.4028234663852886e38 {
			return fmt.Errorf(".NET number values such as positive and negative infinity cannot be written as valid JSON")
		}
		b.WriteString(dotnet.FormatSingle(float32(f)))
	case reflect.Float64:
		f := v.Float()
		if f != f || f > 1.7976931348623157e308 || f < -1.7976931348623157e308 {
			return fmt.Errorf(".NET number values such as positive and negative infinity cannot be written as valid JSON")
		}
		b.WriteString(dotnet.FormatDouble(f))
	case reflect.String:
		WriteString(b, v.String())
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		return encode(b, v.Elem())
	case reflect.Slice:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		if t == reflect.TypeOf(Obj(nil)) {
			return encodeObj(b, v.Interface().(Obj))
		}
		if t.Elem().Kind() == reflect.Uint8 {
			b.WriteByte('"')
			b.WriteString(base64.StdEncoding.EncodeToString(v.Bytes()))
			b.WriteByte('"')
			return nil
		}
		fallthrough
	case reflect.Array:
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := encode(b, v.Index(i)); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case reflect.Map:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		keys := v.MapKeys()
		ks := make([]string, len(keys))
		for i, k := range keys {
			ks[i] = mapKey(k)
		}
		idx := make([]int, len(keys))
		for i := range idx {
			idx[i] = i
		}
		// Go maps are unordered; emit keys sorted for determinism. Callers that
		// need .NET insertion order use stj.Obj.
		sort.Slice(idx, func(a, c int) bool { return ks[idx[a]] < ks[idx[c]] })
		b.WriteByte('{')
		for n, i := range idx {
			if n > 0 {
				b.WriteByte(',')
			}
			WriteString(b, ks[i])
			b.WriteByte(':')
			if err := encode(b, v.MapIndex(keys[i])); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case reflect.Struct:
		return encodeStruct(b, v)
	default:
		return fmt.Errorf("stj: unsupported type %s", t)
	}
	return nil
}

func mapKey(k reflect.Value) string {
	switch k.Kind() {
	case reflect.String:
		return k.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(k.Uint(), 10)
	}
	return fmt.Sprint(k.Interface())
}

func encodeObj(b *bytes.Buffer, o Obj) error {
	b.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		WriteString(b, kv.K)
		b.WriteByte(':')
		if err := encode(b, reflect.ValueOf(kv.V)); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

// field describes one serialized struct member.
type field struct {
	name     string
	index    []int
	omitNull bool // [JsonIgnore(Condition = WhenWritingNull)]
	omitDef  bool // [JsonIgnore(Condition = WhenWritingDefault)]
	readOnly bool // getter-only on deserialize
}

var fieldCache sync.Map

// fields returns the serialized members of t. As with .NET reflection, a
// type's own members come first and members inherited from embedded "base"
// structs follow.
func fields(t reflect.Type) []field {
	if f, ok := fieldCache.Load(t); ok {
		return f.([]field)
	}
	var own, inherited []field
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		tag := sf.Tag.Get("json")
		if sf.Anonymous && tag == "" {
			ft := sf.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				for _, f := range fields(ft) {
					f.index = append([]int{i}, f.index...)
					inherited = append(inherited, f)
				}
				continue
			}
		}
		if !sf.IsExported() || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if name == "" {
			name = sf.Name
		}
		f := field{name: name, index: []int{i}}
		for _, o := range strings.Split(opts, ",") {
			switch o {
			case "omitnull":
				f.omitNull = true
			case "omitdefault", "omitempty":
				f.omitDef = true
			case "readonly":
				f.readOnly = true
			}
		}
		own = append(own, f)
	}
	all := append(own, inherited...)
	fieldCache.Store(t, all)
	return all
}

func encodeStruct(b *bytes.Buffer, v reflect.Value) error {
	b.WriteByte('{')
	first := true
	for _, f := range fields(v.Type()) {
		fv, ok := fieldByIndex(v, f.index)
		if !ok {
			continue
		}
		if f.omitNull && isNullish(fv) {
			continue
		}
		if f.omitDef && fv.IsZero() {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		WriteString(b, f.name)
		b.WriteByte(':')
		if err := encode(b, fv); err != nil {
			return err
		}
	}
	b.WriteByte('}')
	return nil
}

func fieldByIndex(v reflect.Value, idx []int) (reflect.Value, bool) {
	for i, x := range idx {
		if i > 0 && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		}
		v = v.Field(x)
	}
	return v, true
}

func isNullish(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// WriteString writes s as a JSON string using System.Text.Json's default
// JavaScriptEncoder: only printable ASCII outside of "&'+<>`\ is emitted
// literally; everything else is \uXXXX (upper-case hex, UTF-16 surrogate
// pairs), except the short escapes \b \t \n \f \r \\.
func WriteString(b *bytes.Buffer, s string) {
	const hex = "0123456789ABCDEF"
	u := func(r uint16) {
		b.WriteString(`\u`)
		b.WriteByte(hex[r>>12&0xf])
		b.WriteByte(hex[r>>8&0xf])
		b.WriteByte(hex[r>>4&0xf])
		b.WriteByte(hex[r&0xf])
	}
	b.WriteByte('"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch c {
			case '\b':
				b.WriteString(`\b`)
			case '\t':
				b.WriteString(`\t`)
			case '\n':
				b.WriteString(`\n`)
			case '\f':
				b.WriteString(`\f`)
			case '\r':
				b.WriteString(`\r`)
			case '\\':
				b.WriteString(`\\`)
			case '"', '&', '\'', '+', '<', '>', '`':
				u(uint16(c))
			default:
				if c < 0x20 || c == 0x7f {
					u(uint16(c))
				} else {
					b.WriteByte(c)
				}
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			u(0xFFFD)
		} else if r >= 0x10000 {
			r1, r2 := utf16.EncodeRune(r)
			u(uint16(r1))
			u(uint16(r2))
		} else {
			u(uint16(r))
		}
		i += size
	}
	b.WriteByte('"')
}
