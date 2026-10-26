// Package hosting is the Go port of AUnrealFeatures.Hosting's HTTP stack
// (AstraHttpServer and friends). It reproduces the original pipeline:
//
//  1. WebSocket upgrade requests are handed to the server's WebSocket hook on
//     their own goroutine and never enter the HTTP pipeline.
//  2. Every other request is processed strictly one at a time per server
//     (the original listen loop awaited each request before accepting the
//     next), in this order: access log, session processor, preprocessors,
//     CORS headers, OPTIONS short-circuit (204), endpoint dispatch.
//  3. Unknown routes (or a known route with another method) yield 404 with an
//     empty text/plain body; handler exceptions yield the 500 HTML document.
package hosting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"solara/internal/dotnet"
	"solara/internal/logging"
)

// Handler is an endpoint method. Route parameters are available through
// Context.Param. Returning nil mirrors a method returning a null result
// (NoContent).
type Handler func(c *Context) Result

// Context mirrors IHttpContext plus the route parameters bound for the call.
type Context struct {
	Request  *Request
	Response *Response
	Session  *Session
	Server   *Server
	params   map[string]string
}

// Param returns a route parameter captured from {name} in the route pattern
// (raw, still percent-encoded, as the original regex extraction produced).
func (c *Context) Param(name string) string { return c.params[name] }

// HasParam reports whether the route declared {name}.
func (c *Context) HasParam(name string) bool { _, ok := c.params[name]; return ok }

// Session mirrors AstraHttpSession.
type Session struct {
	SessionId   string
	SessionType string
	ExpiresAt   dotnet.DateTime
	Scopes      []string
	Claims      map[string]string
}

func newSession() *Session {
	return &Session{SessionId: dotnet.NewGuid(), SessionType: "Default",
		ExpiresAt: dotnet.UtcNow().AddMinutes(30), Scopes: []string{}, Claims: map[string]string{}}
}

func (s *Session) IsExpired() bool { return dotnet.UtcNow().After(s.ExpiresAt) }
func (s *Session) AddScope(scope string) {
	for _, x := range s.Scopes {
		if x == scope {
			return
		}
	}
	s.Scopes = append(s.Scopes, scope)
}
func (s *Session) AddClaim(k, v string)   { s.Claims[k] = v }
func (s *Session) RemoveClaim(k string)   { delete(s.Claims, k) }
func (s *Session) Extend(d time.Duration) { s.ExpiresAt = dotnet.UtcNow().Add(d) }

// Response mirrors AstraHttpResponse. Headers set through SetHeader are
// applied when the action result is written.
type Response struct {
	w        http.ResponseWriter
	headers  map[string]string
	order    []string
	cookies  []*http.Cookie
	written  bool
	redirect string
}

func newResponse(w http.ResponseWriter) *Response {
	return &Response{w: w, headers: map[string]string{}}
}

// SetHeader mirrors IHttpResponse.SetHeader.
func (r *Response) SetHeader(k, v string) {
	if _, ok := r.headers[k]; !ok {
		r.order = append(r.order, k)
	}
	r.headers[k] = v
}

// RemoveHeader mirrors IHttpResponse.RemoveHeader.
func (r *Response) RemoveHeader(k string) {
	delete(r.headers, k)
	for i, x := range r.order {
		if x == k {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// Headers returns the pending response headers.
func (r *Response) Headers() map[string]string { return r.headers }

// SetCookie mirrors IHttpResponse.SetCookie.
func (r *Response) SetCookie(c *http.Cookie) { r.cookies = append(r.cookies, c) }

// Redirect mirrors HttpListenerResponse.Redirect (Location header; the status
// is subsequently overwritten by the action result, as in the original).
func (r *Response) Redirect(url string) { r.redirect = url }

// EnableCors mirrors IHttpResponse.EnableCors with its defaults.
func (r *Response) EnableCors() {
	r.SetHeader("Access-Control-Allow-Origin", "*")
	r.SetHeader("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	r.SetHeader("Access-Control-Allow-Headers", "*")
}

// SetContentDisposition mirrors IHttpResponse.SetContentDisposition.
func (r *Response) SetContentDisposition(fileName string, inline bool) {
	d := "attachment"
	if inline {
		d = "inline"
	}
	r.SetHeader("Content-Disposition", fmt.Sprintf("%s; filename=\"%s\"", d, fileName))
}

// SetCacheControl mirrors IHttpResponse.SetCacheControl.
func (r *Response) SetCacheControl(maxAge int) {
	r.SetHeader("Cache-Control", fmt.Sprintf("max-age=%d, public", maxAge))
}

// Written reports whether a result has already been applied.
func (r *Response) Written() bool { return r.written }

// apply mirrors ApplyToHttpListenerResponse. Only the first application
// reaches the wire (HTTP headers can only be sent once).
func (r *Response) apply(res Result) {
	if res == nil || r.written {
		return
	}
	body := res.Body() // may panic (serialization); caller recovers
	ct := res.ContentType()
	h := r.w.Header()
	for _, k := range r.order {
		h.Set(k, r.headers[k])
	}
	for _, c := range r.cookies {
		h.Add("Set-Cookie", c.String())
	}
	if r.redirect != "" {
		h.Set("Location", r.redirect)
	}
	if ct != "" {
		h.Set("Content-Type", ct)
	} else {
		h["Content-Type"] = nil // suppress Go's content sniffing
	}
	h.Set("Content-Length", fmt.Sprint(len(body)))
	r.written = true
	r.w.WriteHeader(res.StatusCode())
	if len(body) > 0 {
		_, _ = r.w.Write(body)
	}
}

// PreprocessorResult mirrors HttpPreprocessorResult flags.
type PreprocessorResult int

const (
	PreOK        PreprocessorResult = 0
	PreFail      PreprocessorResult = 1
	PreStopAfter PreprocessorResult = 2
)

// Preprocessor mirrors IHttpRequestPreprocessor.
type Preprocessor interface {
	TryPreprocessRequest(req *Request, resp *Response) (Result, PreprocessorResult)
}

// Endpoint mirrors AstraHttpEndpoint.
type Endpoint struct {
	Method   string
	Name     string
	RouteUri string
	Handler  Handler
	re       *regexp.Regexp
	reErr    error
	names    []string
}

var paramRe = regexp.MustCompile(`\{([^}]+)\}`)

func compileRoute(route string) (*regexp.Regexp, []string, error) {
	var b strings.Builder
	b.WriteString("^")
	var names []string
	last := 0
	for _, m := range paramRe.FindAllStringSubmatchIndex(route, -1) {
		b.WriteString(regexp.QuoteMeta(route[last:m[0]]))
		name := route[m[2]:m[3]]
		names = append(names, name)
		b.WriteString("(?P<" + name + ">[^/]+)")
		last = m[1]
	}
	b.WriteString(regexp.QuoteMeta(route[last:]))
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return re, names, err
}

// Server mirrors AstraHttpServer.
type Server struct {
	Name     string
	Hostname string
	Port     uint16
	// BindAddress restricts the listening interface ("" = all interfaces, as
	// the original http://+:port/ prefix). Security hardening option.
	BindAddress string
	Logger      *logging.Logger

	endpoints     []*Endpoint
	preprocessors []Preprocessor
	// SessionProcessor mirrors [HttpSessionProcessor] (none of the original
	// servers declares one; kept for completeness).
	SessionProcessor func(c *Context)
	// WebSocket mirrors HandleWebSocketAsync. It runs on its own goroutine.
	// Returning false leaves the request unanswered in the original; Solara
	// closes the connection instead (KNOWN_ISSUES HOST-WS-001).
	WebSocket func(w http.ResponseWriter, r *http.Request) bool

	mu       sync.Mutex // serializes the HTTP pipeline (original: one request at a time)
	srv      *http.Server
	listener net.Listener
	started  bool
}

// NewServer mirrors the AstraHttpServer(hostname, port) constructor, minus the
// implicit Start(), which callers invoke after registering endpoints (the
// original scanned attributes inside Start()).
func NewServer(name, hostname string, port uint16) *Server {
	return &Server{Name: name, Hostname: hostname, Port: port, Logger: logging.New(name)}
}

// Handle registers an attribute-style endpoint: the URI is normalised to
// "/" + uri.Trim('/'), exactly as ScanForServerBasedEndpoints did. Endpoints
// must be registered in the original declaration (reflection) order because
// the first match wins.
func (s *Server) Handle(method, uri, name string, h Handler) *Endpoint {
	route := "/" + strings.Trim(uri, "/")
	return s.AddEndpoint(method, route, name, h)
}

// AddEndpoint mirrors AstraHttpServer.AddEndpoint (route used verbatim).
func (s *Server) AddEndpoint(method, route, name string, h Handler) *Endpoint {
	re, names, err := compileRoute(route)
	e := &Endpoint{Method: method, Name: name, RouteUri: route, Handler: h, re: re, reErr: err, names: names}
	s.endpoints = append(s.endpoints, e)
	return e
}

// Endpoints mirrors IHttpServer.Endpoints.
func (s *Server) Endpoints() []*Endpoint { return s.endpoints }

// AddPreprocessor mirrors AddPreprocessor<T> (one instance per concrete type).
func (s *Server) AddPreprocessor(p Preprocessor) {
	for _, x := range s.preprocessors {
		if fmt.Sprintf("%T", x) == fmt.Sprintf("%T", p) {
			return
		}
	}
	s.preprocessors = append(s.preprocessors, p)
}

// DisableListen makes Start() a no-op that marks the server started without
// binding a socket. Used by the differential tests (the .NET oracle likewise
// neutralises HttpListener.Start) so servers can be constructed in-process.
var DisableListen = false

// Start mirrors AstraHttpServer.Start: binds http://+:{port}/ and begins
// serving. A bind failure is returned (the original threw from the
// constructor, aborting host startup).
func (s *Server) Start() error {
	if s.started {
		s.Logger.Warning("Cannot start HTTP server when it is already listening!")
		return nil
	}
	if DisableListen {
		s.started = true
		return nil
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(s.BindAddress, fmt.Sprint(s.Port)))
	if err != nil {
		return err
	}
	s.listener = ln
	s.srv = &http.Server{Handler: s, ReadHeaderTimeout: 2 * time.Minute, IdleTimeout: 2 * time.Minute}
	s.started = true
	go func() { _ = s.srv.Serve(ln) }()
	s.Logger.Information("Started HTTP server on '{Host}:{Port}'", s.Hostname, s.Port)
	return nil
}

// Stop mirrors AstraHttpServer.Stop.
func (s *Server) Stop() {
	if !s.started {
		s.Logger.Warning("Cannot stop HTTP server when it is not listening!")
		return
	}
	if s.srv == nil {
		s.started = false
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.srv.Shutdown(ctx)
	s.started = false
	s.Logger.Information("Stopped HTTP server")
}

// Addr returns the bound address (tests).
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// IsWebSocketRequest mirrors HttpListenerRequest.IsWebSocketRequest.
func IsWebSocketRequest(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	conn := strings.ToLower(r.Header.Get("Connection"))
	return strings.Contains(conn, "upgrade") && strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// ServeHTTP is the ListenImpl loop body.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if IsWebSocketRequest(r) {
		func() {
			defer func() {
				if p := recover(); p != nil {
					s.Logger.Error("WebSocket error: {Message}", ExceptionMessage(p))
				}
			}()
			if s.WebSocket == nil || !s.WebSocket(w, r) {
				// original: request left unanswered; we close it
				if hj, ok := w.(http.Hijacker); ok {
					if c, _, err := hj.Hijack(); err == nil {
						_ = c.Close()
					}
				}
			}
		}()
		return
	}
	// Security review finding F4: read the whole body BEFORE taking the
	// per-server lock, bounded in time and size, so a slow or never-finishing
	// upload cannot stall every other request (HTTP.sys bounded these with its
	// ~2 minute entity-body timeout). Deadlines are per request rather than
	// http.Server.ReadTimeout/WriteTimeout, which would also apply to hijacked
	// WebSocket connections.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(RequestBodyTimeout))
	if r.Body != nil {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxRequestBodyBytes))
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			}
			if errors.As(err, &tooBig) {
				return
			}
			// timeout / client gone: drop the connection (as HTTP.sys did)
			// without a response and without taking the lock
			panic(http.ErrAbortHandler)
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Now().Add(ResponseWriteTimeout))

	s.mu.Lock()
	defer s.mu.Unlock()
	s.process(w, r)
}

// Transport limits (security hardening; see KNOWN_ISSUES SEC-F4).
var (
	// RequestBodyTimeout bounds how long a client may take to send its body.
	RequestBodyTimeout = 2 * time.Minute
	// ResponseWriteTimeout bounds how long a client may take to read the response.
	ResponseWriteTimeout = 2 * time.Minute
	// MaxRequestBodyBytes caps request bodies (largest handler limit is the
	// 8 MB board upload, which still answers with its own 400 below this).
	MaxRequestBodyBytes int64 = 64 << 20
)

// Process runs the HTTP pipeline for one request (exported for tests).
func (s *Server) process(w http.ResponseWriter, r *http.Request) {
	c := &Context{Request: newRequest(r), Response: newResponse(w), Session: newSession(), Server: s}
	defer func() {
		if p := recover(); p != nil {
			s.Logger.Error("Error processing request: {Message}", ExceptionMessage(p))
			func() {
				defer func() { _ = recover() }()
				c.Response.apply(HtmlDocument(500, strings.Replace(internalServerErrorDocument, "{0}", ExceptionMessage(p), -1)))
			}()
		}
	}()
	s.Logger.Information("{IpAddress} {HttpMethod} {Uri}", c.Request.Remote(), r.Method, c.Request.PathAndQuery())
	if s.SessionProcessor != nil {
		s.SessionProcessor(c)
	}
	cont := true
	for _, p := range s.preprocessors {
		res, flags := p.TryPreprocessRequest(c.Request, c.Response)
		cont = flags&PreStopAfter == 0
		if !cont {
			c.Response.apply(res)
		}
	}
	c.Response.EnableCors()
	if r.Method == http.MethodOptions {
		c.Response.apply(NoContent())
	} else if cont {
		c.Response.apply(s.processRequest(c))
	}
}

func (s *Server) findEndpoint(method, uri string) (*Endpoint, map[string]string) {
	for _, e := range s.endpoints {
		if e.reErr != nil {
			dotnet.Throw("System.ArgumentException", fmt.Sprintf("Invalid pattern '%s'", e.RouteUri))
		}
		m := e.re.FindStringSubmatch(uri)
		if m == nil || e.Method != method {
			continue
		}
		params := map[string]string{}
		for i, n := range e.re.SubexpNames() {
			if n != "" {
				params[n] = m[i]
			}
		}
		return e, params
	}
	return nil, nil
}

func (s *Server) processRequest(c *Context) (out Result) {
	e, params := s.findEndpoint(c.Request.Method(), c.Request.Uri())
	if e == nil {
		return NotFound(nil)
	}
	c.params = params
	defer func() {
		if p := recover(); p != nil {
			out = HtmlDocument(500, strings.Replace(internalServerErrorDocument, "{0}",
				fmt.Sprintf("Error while processing %s<br><br>%s", e.Name, ExceptionText(p)), -1))
		}
	}()
	res := e.Handler(c)
	if res == nil {
		return NoContent()
	}
	return res
}

// ErrServerClosed re-exports http.ErrServerClosed.
var ErrServerClosed = errors.New("server closed")
