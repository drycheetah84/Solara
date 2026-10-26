// Helpers for the AUnrealFeatures.EOSSDK.Models port: an insertion-ordered
// Dictionary<string, V> and a JsonElement writer matching JsonElement.WriteTo.
package models

import (
	"bytes"
	"fmt"

	"solara/internal/stj"
)

// Dict mirrors Dictionary<string, V> as the models use it: lookups are
// ordinal (case-sensitive) and enumeration/serialization follows insertion
// order (no model dictionary ever has entries removed and then re-added in a
// way that is serialized, so slot reuse after Remove is not modelled).
type Dict[V any] struct {
	keys []string
	vals map[string]V
}

// NewDict returns an empty dictionary (C# `new()`).
func NewDict[V any]() Dict[V] { return Dict[V]{vals: map[string]V{}} }

// Set mirrors the indexer setter: an existing key keeps its position.
func (d *Dict[V]) Set(k string, v V) {
	if d.vals == nil {
		d.vals = map[string]V{}
	}
	if _, ok := d.vals[k]; !ok {
		d.keys = append(d.keys, k)
	}
	d.vals[k] = v
}

// TryGetValue mirrors Dictionary.TryGetValue.
func (d Dict[V]) TryGetValue(k string) (V, bool) {
	v, ok := d.vals[k]
	return v, ok
}

// ContainsKey mirrors Dictionary.ContainsKey.
func (d Dict[V]) ContainsKey(k string) bool { _, ok := d.vals[k]; return ok }

// Remove mirrors Dictionary.Remove.
func (d *Dict[V]) Remove(k string) bool {
	if _, ok := d.vals[k]; !ok {
		return false
	}
	delete(d.vals, k)
	for i, x := range d.keys {
		if x == k {
			d.keys = append(d.keys[:i:i], d.keys[i+1:]...)
			break
		}
	}
	return true
}

// Count mirrors Dictionary.Count.
func (d Dict[V]) Count() int { return len(d.keys) }

// Keys returns the keys in enumeration order.
func (d Dict[V]) Keys() []string { return append([]string(nil), d.keys...) }

// Clone mirrors `new Dictionary<string, V>(source)` (same enumeration order).
func (d Dict[V]) Clone() Dict[V] {
	c := NewDict[V]()
	for _, k := range d.keys {
		c.Set(k, d.vals[k])
	}
	return c
}

// MarshalSTJ writes the dictionary as a JSON object in enumeration order.
// JsonElement values (object-typed entries read from JSON) are re-written the
// way JsonElement.WriteTo does.
func (d Dict[V]) MarshalSTJ() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range d.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		stj.WriteString(&b, k)
		b.WriteByte(':')
		var v any = d.vals[k]
		if el, ok := v.(stj.Element); ok {
			if err := WriteElement(&b, el); err != nil {
				return nil, err
			}
			continue
		}
		out, err := stj.Marshal(v)
		if err != nil {
			return nil, err
		}
		b.Write(out)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// UnmarshalSTJ mirrors the STJ dictionary converter (duplicate keys: last
// value wins, first position kept).
func (d *Dict[V]) UnmarshalSTJ(el stj.Element) error {
	if el.ValueKind() != stj.Object {
		return &stj.JsonError{Msg: "The JSON value could not be converted to System.Collections.Generic.Dictionary. Path: $"}
	}
	*d = NewDict[V]()
	for _, p := range el.EnumerateObject() {
		var v V
		if err := stj.FromElement(p.Value, &v); err != nil {
			return err
		}
		d.Set(p.Name, v)
	}
	return nil
}

// WriteElement mirrors JsonElement.WriteTo with the default writer: the value
// is re-emitted compactly, with string values and property names unescaped and
// re-escaped by the default encoder and numbers copied verbatim.
func WriteElement(b *bytes.Buffer, el stj.Element) error {
	switch el.ValueKind() {
	case stj.Object:
		b.WriteByte('{')
		for i, p := range el.EnumerateObject() {
			if i > 0 {
				b.WriteByte(',')
			}
			stj.WriteString(b, p.Name)
			b.WriteByte(':')
			if err := WriteElement(b, p.Value); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case stj.Array:
		b.WriteByte('[')
		for i, it := range el.EnumerateArray() {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := WriteElement(b, it); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case stj.String:
		stj.WriteString(b, *el.GetString())
	case stj.Number:
		b.WriteString(el.GetRawText())
	case stj.True:
		b.WriteString("true")
	case stj.False:
		b.WriteString("false")
	case stj.Null:
		b.WriteString("null")
	default:
		return fmt.Errorf("Operation is not valid due to the current state of the object.")
	}
	return nil
}

// CompactElement returns WriteElement's output as bytes.
func CompactElement(el stj.Element) ([]byte, error) {
	var b bytes.Buffer
	if err := WriteElement(&b, el); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
