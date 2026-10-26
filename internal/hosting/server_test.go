package hosting

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"solara/internal/dotnet"
	"solara/internal/logging"
	"solara/internal/stj"
)

func init() { logging.DisableFiles = true }

func newTestServer() *Server {
	s := NewServer("TestServer", "localhost", 0)
	s.Handle("GET", "v1/items/{id}/", "GetItem", func(c *Context) Result {
		return Ok(stj.O("id", c.Param("id"), "q", c.QueryString("q")))
	})
	s.Handle("POST", "/v1/echo", "Echo", func(c *Context) Result {
		return Ok(c.BodyString("name"))
	})
	s.Handle("GET", "/v1/boom", "Boom", func(c *Context) Result {
		dotnet.Throw("System.InvalidOperationException", "kaboom")
		return nil
	})
	s.Handle("GET", "/v1/none", "None", func(c *Context) Result { return nil })
	return s
}

func do(s *Server, method, url, body, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestRoutingAndResults(t *testing.T) {
	s := newTestServer()
	r := do(s, "GET", "//v1/items/42?q=a+b", "", "")
	if r.Code != 200 || r.Body.String() != `{"id":"42","q":"a b"}` || r.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("route: %d %q %q", r.Code, r.Body.String(), r.Header().Get("Content-Type"))
	}
	if r.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS header missing")
	}
	// method mismatch is a 404, as FindMatchingEndpoint also filters by method
	if r := do(s, "POST", "/v1/items/42", "", ""); r.Code != 404 || r.Body.Len() != 0 || r.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("405-as-404: %d %q", r.Code, r.Header().Get("Content-Type"))
	}
	if r := do(s, "OPTIONS", "/anything", "", ""); r.Code != 204 {
		t.Errorf("OPTIONS: %d", r.Code)
	}
	if r := do(s, "POST", "/v1/echo", `{"name":"x<y"}`, "application/json"); r.Code != 200 || r.Body.String() != "x<y" || r.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("echo: %d %q", r.Code, r.Body.String())
	}
	r = do(s, "GET", "/v1/boom", "", "")
	if r.Code != 500 || !strings.Contains(r.Body.String(), "Error while processing Boom<br><br>System.InvalidOperationException: kaboom") || r.Header().Get("Content-Type") != "text/html" {
		t.Errorf("500: %d %s", r.Code, r.Body.String())
	}
	if r := do(s, "GET", "/v1/none", "", ""); r.Code != 204 {
		t.Errorf("nil result: %d", r.Code)
	}
	// a query segment without '=' made the original throw ArgumentNullException when Queries was read
	if r := do(s, "GET", "/v1/items/1?flag", "", ""); r.Code != 500 || !strings.Contains(r.Body.String(), "ArgumentNullException") {
		t.Errorf("null query key: %d %s", r.Code, r.Body.String())
	}
	// JsonBody without a content type -> NullReferenceException
	if r := do(s, "POST", "/v1/echo", `{}`, ""); r.Code != 500 || !strings.Contains(r.Body.String(), "NullReferenceException") {
		t.Errorf("no ctype: %d", r.Code)
	}
}

func TestNameValues(t *testing.T) {
	nv := ParseNameValues("a=1&A=2&b=x%20y&c=")
	d := nv.Dict()
	if d["a"] != "1,2" || d["b"] != "x y" || d["c"] != "" {
		t.Errorf("%v", d)
	}
	if _, ok := d["A"]; ok {
		t.Error("dictionary must keep first spelling only")
	}
}

func TestBinding(t *testing.T) {
	if BindInt(" 0x10 ", true) != 16 || BindInt("abc", true) != 0 || BindInt("", false) != 0 || BindInt("-5", true) != -5 {
		t.Error("BindInt")
	}
	if BindString("", false) != "" || !BindBool("TRUE", true) || BindBool("yes", true) {
		t.Error("BindString/Bool")
	}
}

var _ = http.MethodGet
