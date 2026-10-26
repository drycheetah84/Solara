// Helpers for System.Text.Json behaviour MothershipServer.cs relies on that the
// shared stj package does not model: JsonElement.WriteTo (elements are
// re-written, not copied verbatim) and the mutable System.Text.Json.Nodes tree
// (JsonNode.Parse / JsonObject indexers / ToJsonString(WriteIndented = true))
// used by ApplyAutoCompletedQuests.
package mothership

import (
	"bytes"
	"runtime"
	"strconv"
	"strings"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// newLine mirrors Environment.NewLine, which Utf8JsonWriter (.NET 6) uses for
// indented output. The production backend ran on Windows.
var newLine = func() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}()

func writeIndent(b *bytes.Buffer, depth int) {
	b.WriteString(newLine)
	b.WriteString(strings.Repeat("  ", depth))
}

// writeElement mirrors JsonElement.WriteTo(Utf8JsonWriter) with default
// options: strings/property names re-escaped by the default encoder, numbers
// copied as written, structure re-emitted (minified or indented).
func writeElement(b *bytes.Buffer, el stj.Element, indented bool, depth int) {
	switch el.ValueKind() {
	case stj.Object:
		props := el.EnumerateObject()
		if len(props) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, p := range props {
			if i > 0 {
				b.WriteByte(',')
			}
			if indented {
				writeIndent(b, depth+1)
			}
			stj.WriteString(b, p.Name)
			b.WriteByte(':')
			if indented {
				b.WriteByte(' ')
			}
			writeElement(b, p.Value, indented, depth+1)
		}
		if indented {
			writeIndent(b, depth)
		}
		b.WriteByte('}')
	case stj.Array:
		items := el.EnumerateArray()
		if len(items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, it := range items {
			if i > 0 {
				b.WriteByte(',')
			}
			if indented {
				writeIndent(b, depth+1)
			}
			writeElement(b, it, indented, depth+1)
		}
		if indented {
			writeIndent(b, depth)
		}
		b.WriteByte(']')
	case stj.String:
		stj.WriteString(b, el.Str())
	case stj.Number:
		b.WriteString(el.GetRawText())
	case stj.True:
		b.WriteString("true")
	case stj.False:
		b.WriteString("false")
	default:
		b.WriteString("null")
	}
}

// rewrittenElement serializes like a JsonElement value (re-written).
type rewrittenElement struct{ el stj.Element }

func (r rewrittenElement) MarshalSTJ() ([]byte, error) {
	var b bytes.Buffer
	writeElement(&b, r.el, false, 0)
	return b.Bytes(), nil
}

// ---- System.Text.Json.Nodes subset -----------------------------------------

// jnode mirrors a non-null JsonNode. A nil *jnode is a JSON null.
type jnode struct {
	kind  stj.ValueKind
	props []jprop  // JsonObject
	items []*jnode // JsonArray
	str   string   // JsonValue string
	num   string   // JsonValue number (raw text)
	// materialized: a JsonObject created from a JsonElement builds its
	// dictionary lazily on first access, throwing on duplicate names then.
	materialized bool
}

type jprop struct {
	name string
	val  *jnode
}

func nodeFromElement(el stj.Element) *jnode {
	switch el.ValueKind() {
	case stj.Object:
		n := &jnode{kind: stj.Object}
		for _, p := range el.EnumerateObject() {
			n.props = append(n.props, jprop{p.Name, nodeFromElement(p.Value)})
		}
		return n
	case stj.Array:
		n := &jnode{kind: stj.Array, items: []*jnode{}}
		for _, it := range el.EnumerateArray() {
			n.items = append(n.items, nodeFromElement(it))
		}
		return n
	case stj.String:
		return &jnode{kind: stj.String, str: el.Str()}
	case stj.Number:
		return &jnode{kind: stj.Number, num: el.GetRawText()}
	case stj.True, stj.False:
		return &jnode{kind: el.ValueKind()}
	}
	return nil
}

// parseNode mirrors JsonNode.Parse(string) (JsonException on bad input).
func parseNode(s string) *jnode {
	el, err := stj.Parse([]byte(s))
	if err != nil {
		dotnet.Throw("System.Text.Json.JsonException", err.Error())
	}
	return nodeFromElement(el)
}

func newObject() *jnode { return &jnode{kind: stj.Object, materialized: true} }
func newArray() *jnode  { return &jnode{kind: stj.Array, items: []*jnode{}} }
func strNode(s string) *jnode {
	return &jnode{kind: stj.String, str: s}
}
func intNode(v int) *jnode { return &jnode{kind: stj.Number, num: strconv.Itoa(v)} }

func (n *jnode) materialize() {
	if n.materialized {
		return
	}
	seen := map[string]bool{}
	for _, p := range n.props {
		if seen[p.name] {
			dotnet.Throw("System.ArgumentException", "An item with the same key has already been added. Key: "+p.name)
		}
		seen[p.name] = true
	}
	n.materialized = true
}

// get mirrors JsonObject[name] (case-sensitive).
func (n *jnode) get(name string) *jnode {
	n.materialize()
	for _, p := range n.props {
		if p.name == name {
			return p.val
		}
	}
	return nil
}

// set mirrors JsonObject[name] = value (existing key keeps its position).
func (n *jnode) set(name string, v *jnode) {
	n.materialize()
	for i := range n.props {
		if n.props[i].name == name {
			n.props[i].val = v
			return
		}
	}
	n.props = append(n.props, jprop{name, v})
}

// asString mirrors the explicit (string?) conversion of a JsonNode.
func (n *jnode) asString() *string {
	if n == nil {
		return nil
	}
	switch n.kind {
	case stj.String:
		s := n.str
		return &s
	case stj.Object, stj.Array:
		dotnet.Throw("System.InvalidOperationException", "The node must be of type 'JsonValue'.")
	}
	dotnet.Throw("System.InvalidOperationException", "An element of type '"+n.kind.String()+"' cannot be converted to a 'System.String'.")
	return nil
}

// asInt mirrors the explicit (int?) conversion of a JsonNode.
func (n *jnode) asInt() *int {
	if n == nil {
		return nil
	}
	switch n.kind {
	case stj.Number:
		v, err := strconv.ParseInt(n.num, 10, 32)
		if err != nil {
			dotnet.Throw("System.FormatException", "One of the identified items was in an invalid format.")
		}
		x := int(v)
		return &x
	case stj.Object, stj.Array:
		dotnet.Throw("System.InvalidOperationException", "The node must be of type 'JsonValue'.")
	}
	dotnet.Throw("System.InvalidOperationException", "An element of type '"+n.kind.String()+"' cannot be converted to a 'System.Int32'.")
	return nil
}

// writeNode mirrors JsonNode.WriteTo with WriteIndented.
func writeNode(b *bytes.Buffer, n *jnode, indented bool, depth int) {
	if n == nil {
		b.WriteString("null")
		return
	}
	switch n.kind {
	case stj.Object:
		if len(n.props) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, p := range n.props {
			if i > 0 {
				b.WriteByte(',')
			}
			if indented {
				writeIndent(b, depth+1)
			}
			stj.WriteString(b, p.name)
			b.WriteByte(':')
			if indented {
				b.WriteByte(' ')
			}
			writeNode(b, p.val, indented, depth+1)
		}
		if indented {
			writeIndent(b, depth)
		}
		b.WriteByte('}')
	case stj.Array:
		if len(n.items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, it := range n.items {
			if i > 0 {
				b.WriteByte(',')
			}
			if indented {
				writeIndent(b, depth+1)
			}
			writeNode(b, it, indented, depth+1)
		}
		if indented {
			writeIndent(b, depth)
		}
		b.WriteByte(']')
	case stj.String:
		stj.WriteString(b, n.str)
	case stj.Number:
		b.WriteString(n.num)
	case stj.True:
		b.WriteString("true")
	case stj.False:
		b.WriteString("false")
	}
}

// toJsonStringIndented mirrors ToJsonString(new JsonSerializerOptions { WriteIndented = true }).
func (n *jnode) toJsonStringIndented() string {
	var b bytes.Buffer
	writeNode(&b, n, true, 0)
	return b.String()
}
