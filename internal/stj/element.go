// Package stj reproduces the observable behaviour of System.Text.Json as the
// original backend used it: default (unconfigured) serializer output, strict
// case-sensitive deserialization, and JsonDocument/JsonElement semantics.
package stj

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"solara/internal/dotnet"
)

// ValueKind mirrors System.Text.Json.JsonValueKind.
type ValueKind int

const (
	Undefined ValueKind = iota
	Object
	Array
	String
	Number
	True
	False
	Null
)

func (k ValueKind) String() string {
	return [...]string{"Undefined", "Object", "Array", "String", "Number", "True", "False", "Null"}[k]
}

// Prop is one property of a JSON object, in document order.
type Prop struct {
	Name  string
	Value Element
}

// Element mirrors JsonElement: an immutable, ordered view of parsed JSON.
type Element struct {
	kind  ValueKind
	raw   string // exact source text
	str   string // decoded string value
	props []Prop
	items []Element
}

func (e Element) ValueKind() ValueKind { return e.kind }
func (e Element) IsUndefined() bool    { return e.kind == Undefined }

// GetRawText mirrors JsonElement.GetRawText (exact original text).
func (e Element) GetRawText() string { return e.raw }

// ToString mirrors JsonElement.ToString().
func (e Element) ToString() string {
	switch e.kind {
	case Null, Undefined:
		return ""
	case True:
		return "True"
	case False:
		return "False"
	case String:
		return e.str
	}
	return e.raw
}

func invalidOp(expected ValueKind, actual ValueKind) {
	dotnet.Throw("System.InvalidOperationException",
		fmt.Sprintf("The requested operation requires an element of type '%s', but the target element has type '%s'.", expected, actual))
}

// GetProperty mirrors JsonElement.GetProperty (case-sensitive; panics with
// KeyNotFoundException when missing, InvalidOperationException when not an object).
func (e Element) GetProperty(name string) Element {
	v, ok := e.TryGetProperty(name)
	if !ok {
		dotnet.Throw("System.Collections.Generic.KeyNotFoundException", "The given key was not present in the dictionary.")
	}
	return v
}

// TryGetProperty mirrors JsonElement.TryGetProperty. When a name repeats,
// the last occurrence wins, as in System.Text.Json.
func (e Element) TryGetProperty(name string) (Element, bool) {
	if e.kind != Object {
		invalidOp(Object, e.kind)
	}
	for i := len(e.props) - 1; i >= 0; i-- {
		if e.props[i].Name == name {
			return e.props[i].Value, true
		}
	}
	return Element{}, false
}

// Prop is a non-throwing convenience: the named property if e is an object
// and has it, otherwise an Undefined element.
func (e Element) Prop(name string) Element {
	if e.kind != Object {
		return Element{}
	}
	v, _ := e.TryGetProperty(name)
	return v
}

// EnumerateObject mirrors JsonElement.EnumerateObject.
func (e Element) EnumerateObject() []Prop {
	if e.kind != Object {
		invalidOp(Object, e.kind)
	}
	return e.props
}

// EnumerateArray mirrors JsonElement.EnumerateArray.
func (e Element) EnumerateArray() []Element {
	if e.kind != Array {
		invalidOp(Array, e.kind)
	}
	return e.items
}

// GetArrayLength mirrors JsonElement.GetArrayLength.
func (e Element) GetArrayLength() int { return len(e.EnumerateArray()) }

// GetString mirrors JsonElement.GetString: string value, nil for Null, panics otherwise.
func (e Element) GetString() *string {
	switch e.kind {
	case String:
		s := e.str
		return &s
	case Null:
		return nil
	}
	invalidOp(String, e.kind)
	return nil
}

// Str returns GetString() with null mapped to "".
func (e Element) Str() string {
	if p := e.GetString(); p != nil {
		return *p
	}
	return ""
}

// GetBoolean mirrors JsonElement.GetBoolean.
func (e Element) GetBoolean() bool {
	switch e.kind {
	case True:
		return true
	case False:
		return false
	}
	invalidOp(True, e.kind)
	return false
}

func (e Element) number() string {
	if e.kind != Number {
		invalidOp(Number, e.kind)
	}
	return e.raw
}

// TryGetInt64 mirrors JsonElement.TryGetInt64 (integral literal within range).
func (e Element) TryGetInt64() (int64, bool) {
	v, err := strconv.ParseInt(e.number(), 10, 64)
	return v, err == nil
}

// TryGetInt32 mirrors JsonElement.TryGetInt32.
func (e Element) TryGetInt32() (int32, bool) {
	v, err := strconv.ParseInt(e.number(), 10, 32)
	return int32(v), err == nil
}

// TryGetDouble mirrors JsonElement.TryGetDouble.
func (e Element) TryGetDouble() (float64, bool) {
	v, err := strconv.ParseFloat(e.number(), 64)
	return v, err == nil && !math.IsInf(v, 0)
}

func formatEx() {
	dotnet.Throw("System.FormatException", "One of the identified items was in an invalid format.")
}

// GetInt32 mirrors JsonElement.GetInt32.
func (e Element) GetInt32() int32 {
	v, ok := e.TryGetInt32()
	if !ok {
		formatEx()
	}
	return v
}

// GetInt64 mirrors JsonElement.GetInt64.
func (e Element) GetInt64() int64 {
	v, ok := e.TryGetInt64()
	if !ok {
		formatEx()
	}
	return v
}

// GetDouble mirrors JsonElement.GetDouble.
func (e Element) GetDouble() float64 {
	v, ok := e.TryGetDouble()
	if !ok {
		formatEx()
	}
	return v
}

// MarshalSTJ mirrors JsonElement.WriteTo: compact output, strings and property
// names re-escaped with the default encoder, numbers written as their original
// literal text.
func (e Element) MarshalSTJ() ([]byte, error) {
	if e.kind == Undefined {
		return nil, fmt.Errorf("cannot serialize undefined JsonElement")
	}
	var b bytes.Buffer
	e.writeCompact(&b)
	return b.Bytes(), nil
}

func (e Element) writeCompact(b *bytes.Buffer) {
	switch e.kind {
	case Object:
		b.WriteByte('{')
		for i, p := range e.props {
			if i > 0 {
				b.WriteByte(',')
			}
			WriteString(b, p.Name)
			b.WriteByte(':')
			p.Value.writeCompact(b)
		}
		b.WriteByte('}')
	case Array:
		b.WriteByte('[')
		for i, it := range e.items {
			if i > 0 {
				b.WriteByte(',')
			}
			it.writeCompact(b)
		}
		b.WriteByte(']')
	case String:
		WriteString(b, e.str)
	default:
		b.WriteString(e.raw)
	}
}

// ---------------------------------------------------------------------------
// Parser (RFC 8259, System.Text.Json defaults: no comments, no trailing
// commas, max depth 64).

type parser struct {
	s   string
	pos int
	dep int
}

// SyntaxError mirrors JsonException raised by the reader.
type SyntaxError struct{ Msg string }

func (e *SyntaxError) Error() string { return e.Msg }

// Parse mirrors JsonDocument.Parse(...).RootElement.
func Parse(data []byte) (Element, error) {
	s := string(data)
	// UTF-8 BOM is skipped by JsonDocument.
	s = strings.TrimPrefix(s, "\xef\xbb\xbf")
	p := &parser{s: s}
	p.ws()
	if p.pos >= len(p.s) {
		return Element{}, &SyntaxError{"The input does not contain any JSON tokens. Expected the input to start with a valid JSON token, while isFinalBlock is true. LineNumber: 0 | BytePositionInLine: 0."}
	}
	el, err := p.value()
	if err != nil {
		return Element{}, err
	}
	p.ws()
	if p.pos != len(p.s) {
		return Element{}, p.err("'%c' is invalid after a single JSON value. Expected end of data.", p.s[p.pos])
	}
	return el, nil
}

// MustParse parses or panics with a JsonException.
func MustParse(data []byte) Element {
	el, err := Parse(data)
	if err != nil {
		dotnet.Throw("System.Text.Json.JsonException", err.Error())
	}
	return el
}

func (p *parser) err(f string, a ...any) error {
	line, col := 0, 0
	for i := 0; i < p.pos && i < len(p.s); i++ {
		if p.s[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return &SyntaxError{fmt.Sprintf(f, a...) + fmt.Sprintf(" LineNumber: %d | BytePositionInLine: %d.", line, col)}
}

func (p *parser) ws() {
	for p.pos < len(p.s) {
		switch p.s[p.pos] {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *parser) value() (Element, error) {
	if p.pos >= len(p.s) {
		return Element{}, p.err("Expected a value but reached end of data.")
	}
	start := p.pos
	switch c := p.s[p.pos]; {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"':
		str, err := p.str()
		if err != nil {
			return Element{}, err
		}
		return Element{kind: String, raw: p.s[start:p.pos], str: str}, nil
	case c == 't':
		return p.lit("true", True)
	case c == 'f':
		return p.lit("false", False)
	case c == 'n':
		return p.lit("null", Null)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.num()
	default:
		return Element{}, p.err("'%c' is an invalid start of a value.", c)
	}
}

func (p *parser) lit(word string, k ValueKind) (Element, error) {
	if !strings.HasPrefix(p.s[p.pos:], word) {
		return Element{}, p.err("'%s' is an invalid JSON literal. Expected the literal '%s'.", p.s[p.pos:min(len(p.s), p.pos+len(word))], word)
	}
	p.pos += len(word)
	return Element{kind: k, raw: word}, nil
}

func (p *parser) num() (Element, error) {
	start := p.pos
	if p.s[p.pos] == '-' {
		p.pos++
	}
	digits := func() int {
		n := 0
		for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
			p.pos++
			n++
		}
		return n
	}
	if p.pos < len(p.s) && p.s[p.pos] == '0' {
		p.pos++
	} else if digits() == 0 {
		return Element{}, p.err("'-' is an invalid end of a number. Expected a digit ('0'-'9').")
	}
	if p.pos < len(p.s) && p.s[p.pos] == '.' {
		p.pos++
		if digits() == 0 {
			return Element{}, p.err("Expected a digit ('0'-'9').")
		}
	}
	if p.pos < len(p.s) && (p.s[p.pos] == 'e' || p.s[p.pos] == 'E') {
		p.pos++
		if p.pos < len(p.s) && (p.s[p.pos] == '+' || p.s[p.pos] == '-') {
			p.pos++
		}
		if digits() == 0 {
			return Element{}, p.err("Expected a digit ('0'-'9').")
		}
	}
	return Element{kind: Number, raw: p.s[start:p.pos]}, nil
}

func (p *parser) str() (string, error) {
	p.pos++ // opening quote
	var b strings.Builder
	for {
		if p.pos >= len(p.s) {
			return "", p.err("Expected end of string, but instead reached end of data.")
		}
		c := p.s[p.pos]
		switch {
		case c == '"':
			p.pos++
			return b.String(), nil
		case c == '\\':
			p.pos++
			if p.pos >= len(p.s) {
				return "", p.err("Expected end of string, but instead reached end of data.")
			}
			esc := p.s[p.pos]
			p.pos++
			switch esc {
			case '"', '\\', '/':
				b.WriteByte(esc)
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				r, ok := p.hex4()
				if !ok {
					return "", p.err("'%c' is not a valid hex character.", p.s[min(p.pos, len(p.s)-1)])
				}
				if utf16.IsSurrogate(r) && strings.HasPrefix(p.s[p.pos:], "\\u") {
					save := p.pos
					p.pos += 2
					if r2, ok2 := p.hex4(); ok2 && utf16.DecodeRune(r, r2) != utf8.RuneError {
						r = utf16.DecodeRune(r, r2)
					} else {
						p.pos = save
					}
				}
				b.WriteRune(r)
			default:
				return "", p.err("'%c' is not a valid escapable character within a JSON string. The string should be correctly escaped.", esc)
			}
		case c < 0x20:
			return "", p.err("'0x%02X' is invalid within a JSON string. The string should be correctly escaped.", c)
		default:
			b.WriteByte(c)
			p.pos++
		}
	}
}

func (p *parser) hex4() (rune, bool) {
	if p.pos+4 > len(p.s) {
		return 0, false
	}
	v, err := strconv.ParseUint(p.s[p.pos:p.pos+4], 16, 32)
	if err != nil {
		return 0, false
	}
	p.pos += 4
	return rune(v), true
}

func (p *parser) enter() error {
	p.dep++
	if p.dep > 64 {
		return p.err("The maximum configured depth of 64 has been exceeded. Cannot read next JSON object.")
	}
	return nil
}

func (p *parser) object() (Element, error) {
	start := p.pos
	if err := p.enter(); err != nil {
		return Element{}, err
	}
	p.pos++
	el := Element{kind: Object}
	p.ws()
	if p.pos < len(p.s) && p.s[p.pos] == '}' {
		p.pos++
		p.dep--
		el.raw = p.s[start:p.pos]
		return el, nil
	}
	for {
		p.ws()
		if p.pos >= len(p.s) || p.s[p.pos] != '"' {
			if p.pos >= len(p.s) {
				return Element{}, p.err("Expected depth to be zero at the end of the JSON payload. There is an open JSON object or array that should be closed.")
			}
			return Element{}, p.err("'%c' is an invalid start of a property name. Expected a '\"'.", p.s[p.pos])
		}
		name, err := p.str()
		if err != nil {
			return Element{}, err
		}
		p.ws()
		if p.pos >= len(p.s) || p.s[p.pos] != ':' {
			return Element{}, p.err("Expected ':' after a property name.")
		}
		p.pos++
		p.ws()
		v, err := p.value()
		if err != nil {
			return Element{}, err
		}
		el.props = append(el.props, Prop{name, v})
		p.ws()
		if p.pos >= len(p.s) {
			return Element{}, p.err("Expected depth to be zero at the end of the JSON payload. There is an open JSON object or array that should be closed.")
		}
		if p.s[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.s[p.pos] == '}' {
			p.pos++
			p.dep--
			el.raw = p.s[start:p.pos]
			return el, nil
		}
		return Element{}, p.err("'%c' is invalid after a value. Expected either ',', '}', or ']'.", p.s[p.pos])
	}
}

func (p *parser) array() (Element, error) {
	start := p.pos
	if err := p.enter(); err != nil {
		return Element{}, err
	}
	p.pos++
	el := Element{kind: Array}
	p.ws()
	if p.pos < len(p.s) && p.s[p.pos] == ']' {
		p.pos++
		p.dep--
		el.raw = p.s[start:p.pos]
		el.items = []Element{}
		return el, nil
	}
	for {
		p.ws()
		v, err := p.value()
		if err != nil {
			return Element{}, err
		}
		el.items = append(el.items, v)
		p.ws()
		if p.pos >= len(p.s) {
			return Element{}, p.err("Expected depth to be zero at the end of the JSON payload. There is an open JSON object or array that should be closed.")
		}
		if p.s[p.pos] == ',' {
			p.pos++
			continue
		}
		if p.s[p.pos] == ']' {
			p.pos++
			p.dep--
			el.raw = p.s[start:p.pos]
			return el, nil
		}
		return Element{}, p.err("'%c' is invalid after a value. Expected either ',', '}', or ']'.", p.s[p.pos])
	}
}
