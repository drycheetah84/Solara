package hosting

import (
	"strconv"
	"strings"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// The original BindableExtensions converted every bound value with
// TypeDescriptor.GetConverter(type).ConvertFromString(value); conversion
// failures were logged and yielded null, which MethodInfo.Invoke turns into
// default(T) for value types. These helpers reproduce that per target type.
// `present` is false when the source value was missing (null).

// BindString mirrors StringConverter: null becomes "".
func BindString(v string, present bool) string {
	if !present {
		return ""
	}
	return v
}

// BindInt mirrors Int32Converter (whitespace trimmed, optional sign, 0x/&h
// hex prefixes); failures and missing values become 0.
func BindInt(v string, present bool) int {
	n, ok := parseIntConverter(v, present, 32)
	if !ok {
		return 0
	}
	return int(n)
}

// BindInt64 mirrors Int64Converter.
func BindInt64(v string, present bool) int64 {
	n, _ := parseIntConverter(v, present, 64)
	return n
}

// BindNullableInt mirrors NullableConverter<int>: null/empty/invalid -> nil.
func BindNullableInt(v string, present bool) *int {
	if !present || strings.TrimSpace(v) == "" {
		return nil
	}
	n, ok := parseIntConverter(v, present, 32)
	if !ok {
		return nil
	}
	x := int(n)
	return &x
}

func parseIntConverter(v string, present bool, bits int) (int64, bool) {
	if !present {
		return 0, false
	}
	t := strings.TrimSpace(v)
	if strings.HasPrefix(t, "0x") || strings.HasPrefix(t, "0X") || strings.HasPrefix(t, "&h") || strings.HasPrefix(t, "&H") {
		u, err := strconv.ParseUint(t[2:], 16, bits)
		if err != nil {
			return 0, false
		}
		if bits == 32 {
			return int64(int32(uint32(u))), true
		}
		return int64(u), true
	}
	t = strings.TrimPrefix(t, "+")
	n, err := strconv.ParseInt(t, 10, bits)
	return n, err == nil
}

// BindBool mirrors BooleanConverter (bool.Parse, case-insensitive, trimmed).
func BindBool(v string, present bool) bool {
	if !present {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		return true
	}
	return false
}

// BindDouble mirrors DoubleConverter (invariant culture).
func BindDouble(v string, present bool) float64 {
	if !present {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0
	}
	return f
}

// QueryString binds a [FromQuery] string parameter.
func (c *Context) QueryString(name string) string {
	v, ok := c.Request.Query(name)
	return BindString(v, ok)
}

// QueryInt binds a [FromQuery] int parameter.
func (c *Context) QueryInt(name string) int {
	v, ok := c.Request.Query(name)
	return BindInt(v, ok)
}

// QueryBool binds a [FromQuery] bool parameter.
func (c *Context) QueryBool(name string) bool {
	v, ok := c.Request.Query(name)
	return BindBool(v, ok)
}

// HeaderString binds a [FromHeader] string parameter.
func (c *Context) HeaderString(name string) string {
	v, ok := c.Request.Header(name)
	return BindString(v, ok)
}

// BodyString binds a primitive [FromBody] string parameter: JsonBody[name].ToString().
func (c *Context) BodyString(name string) string {
	v, ok := c.Request.JsonBody().TryGetProperty(name)
	if !ok {
		return "" // missing -> null; a null string argument
	}
	return BindString(v.ToString(), true)
}

// BodyValue returns JsonBody[name] for complex [FromBody] binding.
func (c *Context) BodyValue(name string) (stj.Element, bool) {
	return c.Request.JsonBody().TryGetProperty(name)
}

// RouteInt mirrors Convert.ChangeType(routeValue, typeof(int)): a FormatException
// (→ 500 page) on failure.
func (c *Context) RouteInt(name string) int {
	n, err := strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 32)
	if err != nil {
		dotnet.Throw("System.FormatException", "Input string was not in a correct format.")
	}
	return int(n)
}
