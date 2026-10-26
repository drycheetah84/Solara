package docdb

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// The document codec reproduces LiteDB's BsonMapper as configured by the
// original (defaults):
//   - members are named after the C# property (Go field name, or `bson:"Name"`);
//   - a member named Id is stored as "_id";
//   - null members are not stored (SerializeNullValues = false); on read a
//     missing member keeps the value the constructor gave it;
//   - DateTime is stored as UTC milliseconds and read back as Local kind
//     (DateTime.MinValue/MaxValue round-trip unchanged, Unspecified);
//   - [JsonIgnore] does not affect storage (separate tags).
//
// Documents are persisted as JSON using LiteDB's extended-JSON conventions
// ({"$oid":..}, {"$date":..}), so a LiteDB export can be imported verbatim.

type dfield struct {
	name  string
	index []int
}

var dfieldCache sync.Map

func dfields(t reflect.Type) []dfield {
	if f, ok := dfieldCache.Load(t); ok {
		return f.([]dfield)
	}
	var own, inherited []dfield
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		tag := sf.Tag.Get("bson")
		if sf.Anonymous && tag == "" && sf.Type.Kind() == reflect.Struct {
			for _, f := range dfields(sf.Type) {
				f.index = append([]int{i}, f.index...)
				inherited = append(inherited, f)
			}
			continue
		}
		if !sf.IsExported() || tag == "-" {
			continue
		}
		name := tag
		if name == "" {
			name = sf.Name
		}
		if name == "Id" {
			name = "_id"
		}
		own = append(own, dfield{name, []int{i}})
	}
	// LiteDB writes _id first, then the remaining members.
	all := append(inherited, own...)
	dfieldCache.Store(t, all)
	return all
}

var (
	dateType  = reflect.TypeOf(dotnet.DateTime{})
	oidType   = reflect.TypeOf(dotnet.ObjectId{})
	elemType  = reflect.TypeOf(stj.Element{})
	objType   = reflect.TypeOf(stj.Obj(nil))
	minTicks  = dotnet.MinValue.Ticks()
	maxTicksV = dotnet.MaxValue.Ticks()
)

func encodeDoc(v reflect.Value) ([]byte, error) {
	var b bytes.Buffer
	if err := encVal(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func isNil(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// DateToStore converts a DateTime to the stored UTC millisecond value.
func DateToStore(d dotnet.DateTime) int64 {
	switch d.Ticks() {
	case minTicks:
		return -62135596800000
	case maxTicksV:
		return 253402300800000
	}
	u := d.ToUniversalTime()
	return (u.Ticks() - 621355968000000000) / dotnet.TicksPerMillisecond
}

// DateFromStore converts stored UTC milliseconds back as LiteDB does.
func DateFromStore(ms int64) dotnet.DateTime {
	if ms <= -62135596800000 {
		return dotnet.MinValue
	}
	if ms >= 253402300800000 {
		return dotnet.MaxValue
	}
	return dotnet.FromUnixMilliseconds(ms).ToLocalTime()
}

func encVal(b *bytes.Buffer, v reflect.Value) error {
	if !v.IsValid() {
		b.WriteString("null")
		return nil
	}
	t := v.Type()
	switch t {
	case dateType:
		fmt.Fprintf(b, `{"$date":%d}`, DateToStore(v.Interface().(dotnet.DateTime)))
		return nil
	case oidType:
		fmt.Fprintf(b, `{"$oid":"%s"}`, v.Interface().(dotnet.ObjectId).Hex())
		return nil
	case elemType:
		el := v.Interface().(stj.Element)
		if el.IsUndefined() {
			b.WriteString("null")
		} else {
			b.WriteString(el.GetRawText())
		}
		return nil
	case objType:
		o := v.Interface().(stj.Obj)
		b.WriteByte('{')
		for i, kv := range o {
			if i > 0 {
				b.WriteByte(',')
			}
			stj.WriteString(b, kv.K)
			b.WriteByte(':')
			if err := encVal(b, reflect.ValueOf(kv.V)); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	}
	if !v.IsValid() {
		b.WriteString("null")
		return nil
	}
	switch t.Kind() {
	case reflect.Bool:
		b.WriteString(strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		b.WriteString(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		b.WriteString(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		b.WriteString(strconv.FormatFloat(v.Float(), 'g', -1, 64))
	case reflect.String:
		// BsonMapper defaults: TrimWhitespace = true, EmptyStringToNull = true.
		// An empty (after trimming) string is stored as an explicit null, which
		// on read overrides the member initializer (verified with LiteDB 5.0.21).
		s := strings.TrimSpace(v.String())
		if s == "" {
			b.WriteString("null")
		} else {
			stj.WriteString(b, s)
		}
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		return encVal(b, v.Elem())
	case reflect.Slice:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		if t.Elem().Kind() == reflect.Uint8 {
			fmt.Fprintf(b, `{"$binary":"%s"}`, base64.StdEncoding.EncodeToString(v.Bytes()))
			return nil
		}
		fallthrough
	case reflect.Array:
		b.WriteByte('[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := encVal(b, v.Index(i)); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	case reflect.Map:
		if v.IsNil() {
			b.WriteString("null")
			return nil
		}
		b.WriteByte('{')
		keys := v.MapKeys()
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			stj.WriteString(b, fmt.Sprint(k.Interface()))
			b.WriteByte(':')
			if err := encVal(b, v.MapIndex(k)); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case reflect.Struct:
		b.WriteByte('{')
		first := true
		for _, f := range dfields(t) {
			fv := v.FieldByIndex(f.index)
			if isNil(fv) {
				continue // SerializeNullValues = false
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			stj.WriteString(b, f.name)
			b.WriteByte(':')
			if err := encVal(b, fv); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	default:
		return fmt.Errorf("docdb: unsupported type %s", t)
	}
	return nil
}

func decodeDoc(data []byte, v reflect.Value) error {
	el, err := stj.Parse(data)
	if err != nil {
		return err
	}
	return decVal(el, v)
}

func wrapped(el stj.Element, key string) (stj.Element, bool) {
	if el.ValueKind() != stj.Object {
		return stj.Element{}, false
	}
	props := el.EnumerateObject()
	if len(props) == 1 && props[0].Name == key {
		return props[0].Value, true
	}
	return stj.Element{}, false
}

func decVal(el stj.Element, v reflect.Value) error {
	t := v.Type()
	if el.ValueKind() == stj.Null {
		v.Set(reflect.Zero(t))
		return nil
	}
	switch t {
	case dateType:
		inner, ok := wrapped(el, "$date")
		if !ok {
			return fmt.Errorf("docdb: expected $date")
		}
		var ms int64
		if inner.ValueKind() == stj.String { // LiteDB export form: ISO string
			d, err := dotnet.ParseSTJ(inner.Str())
			if err != nil {
				return err
			}
			// LiteDB stores DateTime.MinValue/MaxValue as sentinels but its JSON
			// export shifts them by the exporting machine's UTC offset
			// (e.g. "0001-01-01T08:00:00Z"); map the edge days back.
			switch {
			case d.Ticks() < dotnet.TicksPerDay:
				d = dotnet.MinValue
			case d.Ticks() > dotnet.MaxValue.Ticks()-dotnet.TicksPerDay:
				d = dotnet.MaxValue
			}
			ms = DateToStore(d)
		} else {
			ms = inner.GetInt64()
		}
		v.Set(reflect.ValueOf(DateFromStore(ms)))
		return nil
	case oidType:
		inner, ok := wrapped(el, "$oid")
		if !ok {
			return fmt.Errorf("docdb: expected $oid")
		}
		o, err := dotnet.ParseObjectId(inner.Str())
		if err != nil {
			return err
		}
		v.Set(reflect.ValueOf(o))
		return nil
	case elemType:
		v.Set(reflect.ValueOf(el))
		return nil
	case objType:
		o := stj.Obj{}
		for _, p := range el.EnumerateObject() {
			o.Set(p.Name, p.Value)
		}
		v.Set(reflect.ValueOf(o))
		return nil
	}
	switch t.Kind() {
	case reflect.Bool:
		v.SetBool(el.ValueKind() == stj.True)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if in, ok := wrapped(el, "$numberLong"); ok {
			n, _ := strconv.ParseInt(in.Str(), 10, 64)
			v.SetInt(n)
			return nil
		}
		if n, ok := el.TryGetInt64(); ok {
			v.SetInt(n)
		} else {
			v.SetInt(int64(el.GetDouble()))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(el.GetInt64()))
	case reflect.Float32, reflect.Float64:
		if in, ok := wrapped(el, "$numberDecimal"); ok {
			f, _ := strconv.ParseFloat(in.Str(), 64)
			v.SetFloat(f)
			return nil
		}
		if in, ok := wrapped(el, "$numberLong"); ok {
			f, _ := strconv.ParseFloat(in.Str(), 64)
			v.SetFloat(f)
			return nil
		}
		v.SetFloat(el.GetDouble())
	case reflect.String:
		if in, ok := wrapped(el, "$guid"); ok {
			v.SetString(in.Str())
			return nil
		}
		if in, ok := wrapped(el, "$numberDecimal"); ok {
			v.SetString(in.Str())
			return nil
		}
		v.SetString(el.Str())
	case reflect.Pointer:
		nv := reflect.New(t.Elem())
		if err := decVal(el, nv.Elem()); err != nil {
			return err
		}
		v.Set(nv)
	case reflect.Interface:
		v.Set(reflect.ValueOf(el))
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			inner, ok := wrapped(el, "$binary")
			if !ok {
				return fmt.Errorf("docdb: expected $binary")
			}
			raw, err := base64.StdEncoding.DecodeString(inner.Str())
			if err != nil {
				return err
			}
			v.SetBytes(raw)
			return nil
		}
		items := el.EnumerateArray()
		s := reflect.MakeSlice(t, len(items), len(items))
		for i, it := range items {
			if err := decVal(it, s.Index(i)); err != nil {
				return err
			}
		}
		v.Set(s)
	case reflect.Map:
		m := reflect.MakeMap(t)
		for _, p := range el.EnumerateObject() {
			kv := reflect.New(t.Key()).Elem()
			switch t.Key().Kind() {
			case reflect.String:
				kv.SetString(p.Name)
			case reflect.Int, reflect.Int32, reflect.Int64:
				n, _ := strconv.ParseInt(p.Name, 10, 64)
				kv.SetInt(n)
			}
			ev := reflect.New(t.Elem()).Elem()
			if err := decVal(p.Value, ev); err != nil {
				return err
			}
			m.SetMapIndex(kv, ev)
		}
		v.Set(m)
	case reflect.Struct:
		fs := dfields(t)
		for _, p := range el.EnumerateObject() {
			for _, f := range fs {
				if f.name == p.Name {
					if err := decVal(p.Value, v.FieldByIndex(f.index)); err != nil {
						return fmt.Errorf("%s: %w", f.name, err)
					}
					break
				}
			}
		}
	default:
		return fmt.Errorf("docdb: unsupported type %s", t)
	}
	return nil
}

// EqFold mirrors LiteDB's default collation (IgnoreCase) for string equality
// inside query predicates (Find/FindOne/Count/Exists/DeleteMany). Use plain
// == only for predicates applied in memory after FindAll() (LINQ semantics).
func EqFold(a, b string) bool { return strings.EqualFold(a, b) }

// ContainsFold mirrors `list.Contains(x)` inside a LiteDB predicate.
func ContainsFold(list []string, x string) bool {
	for _, s := range list {
		if strings.EqualFold(s, x) {
			return true
		}
	}
	return false
}

// HasPrefixFold mirrors `s.StartsWith(p)` inside a LiteDB predicate.
func HasPrefixFold(s, p string) bool {
	return len(s) >= len(p) && strings.EqualFold(s[:len(p)], p)
}
