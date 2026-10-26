package hosting

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// NameValues mirrors the NameValueCollection HttpListener exposes for query
// strings and HttpUtility.ParseQueryString for form bodies: keys compare
// case-insensitively, the first spelling is kept, repeated values are joined
// with ',', and a segment without '=' produces a null key.
type NameValues struct {
	keys    []string // first spelling, in order of first appearance
	vals    map[string]string
	nullKey bool
	nullVal string
}

// ParseNameValues reproduces HttpValueCollection.FillFromString.
func ParseNameValues(s string) *NameValues {
	nv := &NameValues{vals: map[string]string{}}
	l := len(s)
	for i := 0; i < l; i++ {
		si, ti := i, -1
		for i < l {
			ch := s[i]
			if ch == '=' {
				if ti < 0 {
					ti = i
				}
			} else if ch == '&' {
				break
			}
			i++
		}
		if ti >= 0 {
			nv.add(decode(s[si:ti]), true, decode(s[ti+1:i]))
		} else {
			nv.add("", false, decode(s[si:i]))
		}
		if i == l-1 && s[i] == '&' {
			nv.add("", false, "")
		}
	}
	return nv
}

func decode(s string) string {
	v, err := url.QueryUnescape(s)
	if err != nil {
		// HttpUtility leaves malformed escapes as-is
		return strings.ReplaceAll(s, "+", " ")
	}
	return v
}

func (nv *NameValues) add(k string, hasKey bool, v string) {
	if !hasKey {
		if nv.nullKey {
			nv.nullVal += "," + v
		} else {
			nv.nullKey, nv.nullVal = true, v
		}
		return
	}
	lk := strings.ToLower(k)
	if old, ok := nv.vals[lk]; ok {
		nv.vals[lk] = old + "," + v
		return
	}
	nv.keys = append(nv.keys, k)
	nv.vals[lk] = v
}

// Dict mirrors `AllKeys.ToDictionary(k => k, k => collection[k])`: it throws
// ArgumentNullException when a null key is present, and the resulting
// dictionary is case-sensitive on the first spelling of each key.
func (nv *NameValues) Dict() map[string]string {
	if nv.nullKey {
		dotnet.Throw("System.ArgumentNullException", "Value cannot be null. (Parameter 'key')")
	}
	m := make(map[string]string, len(nv.keys))
	for _, k := range nv.keys {
		m[k] = nv.vals[strings.ToLower(k)]
	}
	return m
}

// Keys returns keys in order of first appearance.
func (nv *NameValues) Keys() []string { return nv.keys }

// Request mirrors AstraHttpRequest.
type Request struct {
	raw      *http.Request
	body     []byte
	bodyRead bool
	json     *stj.Element
	form     map[string]string
	queries  map[string]string
}

func newRequest(r *http.Request) *Request { return &Request{raw: r} }

// NewRequestForTest wraps an *http.Request (tests and websocket handlers).
func NewRequest(r *http.Request) *Request { return newRequest(r) }

// Raw exposes the underlying request.
func (r *Request) Raw() *http.Request { return r.raw }

// Method mirrors Request.Method.Method.
func (r *Request) Method() string { return r.raw.Method }

// Host mirrors Request.Url.Host.
func (r *Request) Host() string {
	h := r.raw.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

// RawPath is Url.AbsolutePath without slash collapsing.
func (r *Request) RawPath() string {
	p := r.raw.URL.EscapedPath()
	if p == "" {
		p = "/"
	}
	return CanonicalizePath(p)
}

// CanonicalizePath reproduces what HTTP.sys / System.Uri did to the path
// before the original ever saw it: backslashes become '/', and "." / ".."
// segments (also percent-encoded as %2e) are resolved per RFC 3986
// remove_dot_segments, never climbing above the root. Without this, route
// parameters could carry ".." into file paths (security review finding F1).
func CanonicalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.ReplaceAll(p, "%5C", "/")
	p = strings.ReplaceAll(p, "%5c", "/")
	segs := strings.Split(p, "/")
	out := make([]string, 0, len(segs))
	for i, s := range segs {
		ls := strings.ToLower(s)
		last := i == len(segs)-1
		switch ls {
		case ".", "%2e":
			if last {
				out = append(out, "")
			}
			continue
		case "..", "%2e.", ".%2e", "%2e%2e":
			if len(out) > 1 {
				out = out[:len(out)-1]
			}
			if last {
				out = append(out, "")
			}
			continue
		}
		out = append(out, s)
	}
	res := strings.Join(out, "/")
	if !strings.HasPrefix(res, "/") {
		res = "/" + res
	}
	return res
}

// Uri mirrors AstraHttpRequest.Uri: AbsolutePath with runs of '/' collapsed.
func (r *Request) Uri() string { return CollapseSlashes(r.RawPath()) }

// PathAndQuery mirrors Url.PathAndQuery (used in the access log line).
func (r *Request) PathAndQuery() string {
	if r.raw.URL.RawQuery != "" || r.raw.URL.ForceQuery {
		return r.RawPath() + "?" + r.raw.URL.RawQuery
	}
	return r.RawPath()
}

// CollapseSlashes collapses runs of '/'.
func CollapseSlashes(p string) string {
	if !strings.Contains(p, "//") {
		return p
	}
	var b strings.Builder
	prev := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '/' {
			if prev {
				continue
			}
			prev = true
		} else {
			prev = false
		}
		b.WriteByte(c)
	}
	return b.String()
}

// Header returns a header value (case-insensitive name; repeated headers are
// comma-joined, as WebHeaderCollection does).
func (r *Request) Header(name string) (string, bool) {
	if strings.EqualFold(name, "Host") {
		return r.raw.Host, r.raw.Host != ""
	}
	vs, ok := r.raw.Header[http.CanonicalHeaderKey(name)]
	if !ok {
		for k, v := range r.raw.Header {
			if strings.EqualFold(k, name) {
				vs, ok = v, true
				break
			}
		}
	}
	if !ok {
		return "", false
	}
	return strings.Join(vs, ","), true
}

// GetHeaderValue mirrors IHttpRequest.GetHeaderValue(key, defaultValue).
func (r *Request) GetHeaderValue(name, def string) string {
	if v, ok := r.Header(name); ok {
		return v
	}
	return def
}

// Headers mirrors IHttpRequest.Headers (case-insensitive dictionary).
func (r *Request) Headers() map[string]string {
	m := map[string]string{}
	for k, v := range r.raw.Header {
		m[k] = strings.Join(v, ",")
	}
	if r.raw.Host != "" {
		m["Host"] = r.raw.Host
	}
	return m
}

// Queries mirrors IHttpRequest.Queries. Panics (ArgumentNullException) when the
// query string contains a segment without '=', as the original did.
func (r *Request) Queries() map[string]string {
	if r.queries == nil {
		r.queries = ParseNameValues(r.raw.URL.RawQuery).Dict()
	}
	return r.queries
}

// Query mirrors Queries.TryGetValue(key) (case-sensitive on the first spelling).
func (r *Request) Query(key string) (string, bool) {
	v, ok := r.Queries()[key]
	return v, ok
}

// GetQueryParameter mirrors IHttpRequest.GetQueryParameter(key, defaultValue).
func (r *Request) GetQueryParameter(key, def string) string {
	if v, ok := r.Query(key); ok {
		return v
	}
	return def
}

// Body mirrors IHttpRequest.Body (read once, cached).
func (r *Request) Body() []byte {
	if !r.bodyRead {
		r.bodyRead = true
		if r.raw.Body != nil {
			r.body, _ = io.ReadAll(r.raw.Body)
		}
		if r.body == nil {
			r.body = []byte{}
		}
	}
	return r.body
}

// ContentType returns the request Content-Type and whether one was sent.
func (r *Request) ContentType() (string, bool) { return r.Header("Content-Type") }

// JsonBody mirrors IHttpRequest.JsonBody: Deserialize<Dictionary<string, object>>
// of the body, values being JsonElement. The returned element is an Object
// (or an empty object for a literal JSON null). Panics like the original:
// NullReferenceException without a Content-Type, InvalidOperationException
// for a non-JSON content type, JsonException for malformed JSON.
func (r *Request) JsonBody() stj.Element {
	if r.json == nil {
		ct, ok := r.ContentType()
		if !ok {
			dotnet.Throw("System.NullReferenceException", "Object reference not set to an instance of an object.")
		}
		if !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			dotnet.Throw("System.InvalidOperationException", "The request body is not in JSON format.")
		}
		el, err := stj.Parse(r.Body())
		if err != nil {
			dotnet.Throw("System.Text.Json.JsonException", err.Error())
		}
		switch el.ValueKind() {
		case stj.Null:
			el = stj.MustParse([]byte("{}"))
		case stj.Object:
		default:
			dotnet.Throw("System.Text.Json.JsonException", "The JSON value could not be converted to System.Collections.Generic.Dictionary`2[System.String,System.Object]. Path: $ | LineNumber: 0 | BytePositionInLine: 1.")
		}
		r.json = &el
	}
	return *r.json
}

// FormBody mirrors IHttpRequest.FormBody.
func (r *Request) FormBody() map[string]string {
	if r.form == nil {
		ct, ok := r.ContentType()
		if !ok {
			dotnet.Throw("System.NullReferenceException", "Object reference not set to an instance of an object.")
		}
		if !strings.HasPrefix(strings.ToLower(ct), "application/x-www-form-urlencoded") {
			dotnet.Throw("System.InvalidOperationException", "The request body is not in URL-encoded format.")
		}
		r.form = ParseNameValues(string(r.Body())).Dict()
	}
	return r.form
}

// Remote mirrors IHttpRequest.Remote (Cf-Connecting-Ip, then X-Real-Ip, then
// the socket peer) rendered as IPAddress.ToString(). An unparsable forwarded
// header panics with FormatException, as IPAddress.Parse does.
func (r *Request) Remote() string {
	for _, h := range []string{"Cf-Connecting-Ip", "X-Real-Ip"} {
		if v, ok := r.Header(h); ok {
			ip := net.ParseIP(strings.TrimSpace(v))
			if ip == nil {
				dotnet.Throw("System.FormatException", "An invalid IP address was specified.")
			}
			return ipString(ip)
		}
	}
	host, _, err := net.SplitHostPort(r.raw.RemoteAddr)
	if err != nil {
		host = r.raw.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ipString(ip)
	}
	return host
}

// Origin mirrors IHttpRequest.Origin (local endpoint address).
func (r *Request) Origin() string {
	if a, ok := r.raw.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		host, _, _ := net.SplitHostPort(a.String())
		if ip := net.ParseIP(host); ip != nil {
			return ipString(ip)
		}
		return host
	}
	return ""
}

func ipString(ip net.IP) string {
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

// RequestId mirrors RequestTraceIdentifier.ToString() (a Guid).
func (r *Request) RequestId() string { return dotnet.NewGuid() }
