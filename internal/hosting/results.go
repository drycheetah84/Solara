package hosting

import (
	_ "embed"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

//go:embed assets/internal_server_error.html
var internalServerErrorDocument string

// InternalServerErrorDocument mirrors HtmlDocumentCache.InternalServerErrorDocumentString.
func InternalServerErrorDocument() string { return internalServerErrorDocument }

// Result mirrors IHttpActionResult. The body is produced lazily, as the
// original serialized only when the result was applied to the response.
type Result interface {
	StatusCode() int
	ContentType() string
	Body() []byte
}

// contentResult mirrors AstraHttpActionResult<object>.
type contentResult struct {
	status  int
	content any
	ctype   *string // overrides the content-derived type
}

func (r *contentResult) StatusCode() int { return r.status }

func (r *contentResult) ContentType() string {
	if r.ctype != nil {
		return *r.ctype
	}
	return contentTypeFor(r.content)
}

func (r *contentResult) Body() []byte { return serialize(r.content) }

// Content returns the object the result carries (tests).
func (r *contentResult) Content() any { return r.content }

// isPrimitive mirrors TypeExtensions.IsPrimitive: CLR primitives, string, decimal.
func isPrimitive(v any) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case Decimal:
		return true
	}
	switch reflect.TypeOf(v).Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return true
	}
	return false
}

// Decimal is a pre-formatted System.Decimal (keeps trailing zeros, e.g. "1.50").
type Decimal string

func (d Decimal) MarshalSTJ() ([]byte, error) { return []byte(d), nil }

func contentTypeFor(v any) string {
	if v == nil || isNilPtr(v) || isPrimitive(v) {
		return "text/plain"
	}
	if _, ok := v.([]byte); ok {
		return "application/octet-stream"
	}
	return "application/json"
}

func isNilPtr(v any) bool {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return rv.IsNil() && rv.Kind() == reflect.Pointer
	}
	return false
}

// ToString mirrors object.ToString() for the primitive types.
func ToString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return dotnet.FormatBool(x)
	case float64:
		return dotnet.FormatDouble(x)
	case float32:
		return dotnet.FormatSingle(x)
	case Decimal:
		return string(x)
	case fmt.Stringer:
		return x.String()
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(rv.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(rv.Uint(), 10)
	case reflect.String:
		return rv.String()
	}
	return fmt.Sprint(v)
}

func serialize(v any) []byte {
	if v == nil || isNilPtr(v) {
		return []byte{}
	}
	if b, ok := v.([]byte); ok {
		return b
	}
	if isPrimitive(v) {
		return []byte(ToString(v))
	}
	return stj.MustMarshal(v)
}

func res(status int, content any) Result { return &contentResult{status: status, content: content} }

// The Results.* factory, one function per original static method.
func Ok(content any) Result                     { return res(200, content) }
func Created(content any) Result                { return res(201, content) }
func Accepted(content any) Result               { return res(202, content) }
func NoContent() Result                         { return res(204, nil) }
func PartialContent(content any) Result         { return res(206, content) }
func MultipleChoices(content any) Result        { return res(300, content) }
func MovedPermanently(content any) Result       { return res(301, content) }
func Found(content any) Result                  { return res(302, content) }
func SeeOther(content any) Result               { return res(303, content) }
func NotModified() Result                       { return res(304, nil) }
func TemporaryRedirect(content any) Result      { return res(307, content) }
func BadRequest(content any) Result             { return res(400, content) }
func Unauthorized(content any) Result           { return res(401, content) }
func PaymentRequired(content any) Result        { return res(402, content) }
func Forbidden(content any) Result              { return res(403, content) }
func NotFound(content any) Result               { return res(404, content) }
func MethodNotAllowed(content any) Result       { return res(405, content) }
func NotAcceptable(content any) Result          { return res(406, content) }
func ProxyAuthenticationRequired(c any) Result  { return res(407, c) }
func RequestTimeout(content any) Result         { return res(408, content) }
func Conflict(content any) Result               { return res(409, content) }
func Gone(content any) Result                   { return res(410, content) }
func LengthRequired(content any) Result         { return res(411, content) }
func PreconditionFailed(content any) Result     { return res(412, content) }
func RequestEntityTooLarge(content any) Result  { return res(413, content) }
func RequestUriTooLong(content any) Result      { return res(414, content) }
func UnsupportedMediaType(content any) Result   { return res(415, content) }
func RequestedRangeNotSatisfiable(c any) Result { return res(416, c) }
func ExpectationFailed(content any) Result      { return res(417, content) }
func UpgradeRequired(content any) Result        { return res(426, content) }
func InternalServerError(content any) Result    { return res(500, content) }
func NotImplemented(content any) Result         { return res(501, content) }
func BadGateway(content any) Result             { return res(502, content) }
func ServiceUnavailable(content any) Result     { return res(503, content) }
func GatewayTimeout(content any) Result         { return res(504, content) }
func HttpVersionNotSupported(c any) Result      { return res(505, c) }

// HtmlDocument mirrors Results.HtmlDocument (content type text/html).
func HtmlDocument(status int, html string) Result {
	ct := "text/html"
	return &contentResult{status: status, content: html, ctype: &ct}
}

// Configurable mirrors Results.Configurable(status, contentType, content).
func Configurable(status int, contentType string, content any) Result {
	ct := contentType
	return &contentResult{status: status, content: content, ctype: &ct}
}

// Status mirrors a raw `new ConfigurableActionResult`-style code with content.
func Status(status int, content any) Result { return res(status, content) }

// ExceptionText renders a recovered panic the way Exception.ToString() starts
// (type and message; the stack trace is not reproduced).
func ExceptionText(p any) string {
	switch x := p.(type) {
	case *dotnet.Exception:
		return x.String()
	case *stj.JsonError:
		return "System.Text.Json.JsonException: " + x.Msg
	case error:
		msg := x.Error()
		switch {
		case strings.Contains(msg, "nil pointer dereference") || strings.Contains(msg, "nil map"):
			return "System.NullReferenceException: Object reference not set to an instance of an object."
		case strings.Contains(msg, "index out of range"):
			return "System.ArgumentOutOfRangeException: Index was out of range. Must be non-negative and less than the size of the collection. (Parameter 'index')"
		case strings.Contains(msg, "integer divide by zero"):
			return "System.DivideByZeroException: Attempted to divide by zero."
		}
		return "System.Exception: " + msg
	}
	return fmt.Sprintf("System.Exception: %v", p)
}

// ExceptionMessage mirrors Exception.Message for a recovered panic.
func ExceptionMessage(p any) string {
	t := ExceptionText(p)
	if _, msg, ok := strings.Cut(t, ": "); ok {
		return msg
	}
	return t
}
