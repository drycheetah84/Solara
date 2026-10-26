package stj

import (
	"encoding/base64"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"solara/internal/dotnet"
)

// Unmarshaler lets a type control how it is read from JSON.
type Unmarshaler interface {
	UnmarshalSTJ(Element) error
}

// Options mirrors the subset of JsonSerializerOptions the original used.
type Options struct {
	CaseInsensitive     bool // PropertyNameCaseInsensitive
	AllowTrailingCommas bool // (accepted but treated as default)
	NumbersFromStrings  bool // JsonNumberHandling.AllowReadingFromString
}

// JsonError mirrors System.Text.Json.JsonException.
type JsonError struct{ Msg string }

func (e *JsonError) Error() string { return e.Msg }

// Unmarshal mirrors JsonSerializer.Deserialize<T>(data[, options]).
// Property matching is case-sensitive unless CaseInsensitive is set; unknown
// properties are ignored; null into a non-nullable value type is an error.
func Unmarshal(data []byte, v any, opts ...Options) error {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	el, err := Parse(data)
	if err != nil {
		return &JsonError{err.Error()}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("stj: Unmarshal needs a non-nil pointer")
	}
	return assign(rv.Elem(), el, o, "$")
}

// FromElement deserializes an already-parsed element into v.
func FromElement(el Element, v any, opts ...Options) error {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	return assign(reflect.ValueOf(v).Elem(), el, o, "$")
}

// MustUnmarshal panics with a JsonException on failure.
func MustUnmarshal(data []byte, v any, opts ...Options) {
	if err := Unmarshal(data, v, opts...); err != nil {
		dotnet.Throw("System.Text.Json.JsonException", err.Error())
	}
}

var (
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
	elementType     = reflect.TypeOf(Element{})
)

func convErr(t reflect.Type, path string) error {
	return &JsonError{fmt.Sprintf("The JSON value could not be converted to %s. Path: %s", dotnetName(t), path)}
}

func dotnetName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "System.String"
	case reflect.Bool:
		return "System.Boolean"
	case reflect.Int, reflect.Int32:
		return "System.Int32"
	case reflect.Int64:
		return "System.Int64"
	case reflect.Float64:
		return "System.Double"
	case reflect.Float32:
		return "System.Single"
	}
	return t.String()
}

func assign(v reflect.Value, el Element, o Options, path string) error {
	t := v.Type()
	if t == elementType {
		v.Set(reflect.ValueOf(el))
		return nil
	}
	if reflect.PointerTo(t).Implements(unmarshalerType) {
		if el.kind == Null && t.Kind() != reflect.Pointer {
			// value-type with custom reader: null is not convertible
			return convErr(t, path)
		}
		return v.Addr().Interface().(Unmarshaler).UnmarshalSTJ(el)
	}
	if t == reflect.TypeOf(dotnet.DateTime{}) {
		if el.kind != String {
			return convErr(t, path)
		}
		d, err := dotnet.ParseSTJ(el.str)
		if err != nil {
			return convErr(t, path)
		}
		v.Set(reflect.ValueOf(d))
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		if el.kind == Null {
			v.Set(reflect.Zero(t))
			return nil
		}
		nv := reflect.New(t.Elem())
		if err := assign(nv.Elem(), el, o, path); err != nil {
			return err
		}
		v.Set(nv)
		return nil
	case reflect.Interface:
		if el.kind == Null {
			v.Set(reflect.Zero(t))
			return nil
		}
		// object-typed members materialise as JsonElement, as in STJ.
		v.Set(reflect.ValueOf(el))
		return nil
	case reflect.String:
		switch el.kind {
		case Null:
			v.SetString("")
		case String:
			v.SetString(el.str)
		default:
			return convErr(t, path)
		}
		return nil
	case reflect.Bool:
		switch el.kind {
		case True:
			v.SetBool(true)
		case False:
			v.SetBool(false)
		default:
			return convErr(t, path)
		}
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		raw, ok := numText(el, o)
		if !ok {
			return convErr(t, path)
		}
		n, err := strconv.ParseInt(raw, 10, t.Bits())
		if err != nil {
			return convErr(t, path)
		}
		v.SetInt(n)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		raw, ok := numText(el, o)
		if !ok {
			return convErr(t, path)
		}
		n, err := strconv.ParseUint(raw, 10, t.Bits())
		if err != nil {
			return convErr(t, path)
		}
		v.SetUint(n)
		return nil
	case reflect.Float32, reflect.Float64:
		raw, ok := numText(el, o)
		if !ok {
			return convErr(t, path)
		}
		f, err := strconv.ParseFloat(raw, t.Bits())
		if err != nil || math.IsInf(f, 0) {
			return convErr(t, path)
		}
		v.SetFloat(f)
		return nil
	case reflect.Slice:
		if el.kind == Null {
			v.Set(reflect.Zero(t))
			return nil
		}
		if t.Elem().Kind() == reflect.Uint8 && el.kind == String {
			b, err := base64.StdEncoding.DecodeString(el.str)
			if err != nil {
				return convErr(t, path)
			}
			v.SetBytes(b)
			return nil
		}
		if t == reflect.TypeOf(Obj(nil)) {
			if el.kind != Object {
				return convErr(t, path)
			}
			obj := Obj{}
			for _, p := range el.props {
				obj.Set(p.Name, p.Value)
			}
			v.Set(reflect.ValueOf(obj))
			return nil
		}
		if el.kind != Array {
			return convErr(t, path)
		}
		s := reflect.MakeSlice(t, len(el.items), len(el.items))
		for i, it := range el.items {
			if err := assign(s.Index(i), it, o, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		v.Set(s)
		return nil
	case reflect.Map:
		if el.kind == Null {
			v.Set(reflect.Zero(t))
			return nil
		}
		if el.kind != Object {
			return convErr(t, path)
		}
		m := reflect.MakeMapWithSize(t, len(el.props))
		for _, p := range el.props {
			kv := reflect.New(t.Key()).Elem()
			switch t.Key().Kind() {
			case reflect.String:
				kv.SetString(p.Name)
			case reflect.Int, reflect.Int32, reflect.Int64:
				n, err := strconv.ParseInt(p.Name, 10, 64)
				if err != nil {
					return convErr(t.Key(), path)
				}
				kv.SetInt(n)
			default:
				return fmt.Errorf("stj: unsupported map key %s", t.Key())
			}
			ev := reflect.New(t.Elem()).Elem()
			if err := assign(ev, p.Value, o, path+"."+p.Name); err != nil {
				return err
			}
			m.SetMapIndex(kv, ev)
		}
		v.Set(m)
		return nil
	case reflect.Struct:
		if el.kind != Object {
			return convErr(t, path)
		}
		fs := fields(t)
		for _, p := range el.props {
			f := matchField(fs, p.Name, o.CaseInsensitive)
			if f == nil || f.readOnly {
				continue
			}
			fv := v
			for i, x := range f.index {
				if i > 0 && fv.Kind() == reflect.Pointer {
					if fv.IsNil() {
						fv.Set(reflect.New(fv.Type().Elem()))
					}
					fv = fv.Elem()
				}
				fv = fv.Field(x)
			}
			if err := assign(fv, p.Value, o, path+"."+p.Name); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("stj: unsupported type %s", t)
}

func numText(el Element, o Options) (string, bool) {
	if el.kind == Number {
		return el.raw, true
	}
	if el.kind == String && o.NumbersFromStrings {
		return el.str, true
	}
	return "", false
}

func matchField(fs []field, name string, ci bool) *field {
	for i := range fs {
		if fs[i].name == name {
			return &fs[i]
		}
	}
	if ci {
		for i := range fs {
			if strings.EqualFold(fs[i].name, name) {
				return &fs[i]
			}
		}
	}
	return nil
}
