package eos

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"solara/internal/hosting"
)

// Security review finding F1: route parameters must never carry the request
// outside playerdata/. Requests go over a real connection with raw request
// lines so no client-side path cleaning happens.
func TestPlayerDataTraversalBlocked(t *testing.T) {
	// a "secret" next to the executable directory and one level above it
	secretDir := filepath.Join(BaseDirectory, "config")
	_ = os.MkdirAll(secretDir, 0o755)
	_ = os.WriteFile(filepath.Join(secretDir, "legacy-secrets.env"), []byte("TOPSECRET=1"), 0o600)
	_ = os.WriteFile(filepath.Join(BaseDirectory, "server_api_key.txt"), []byte("TOPSECRET"), 0o600)

	gw := newEosGatewayServer()
	ts := httptest.NewServer(gw.HTTP)
	defer ts.Close()

	raw := func(method, target, body string) (int, string) {
		conn, err := net.Dial("tcp", ts.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		req := method + " " + target + " HTTP/1.1\r\nHost: x\r\nConnection: close\r\nContent-Length: " +
			itoa(len(body)) + "\r\n\r\n" + body
		if _, err := conn.Write([]byte(req)); err != nil {
			t.Fatal(err)
		}
		resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b := new(strings.Builder)
		_, _ = bufio.NewReader(resp.Body).WriteTo(b)
		return resp.StatusCode, b.String()
	}

	attacks := []string{
		"/datastorage/v2/data/deployment/../private/config/legacy-secrets.env",
		"/datastorage/v2/data/deployment/%2e%2e/private/config/legacy-secrets.env",
		"/datastorage/v2/data/deployment/..%5C..%5Cx/private/config/legacy-secrets.env",
		"/datastorage/v2/data/deployment/../../private/./server_api_key.txt",
		"/datastorage/v2/data/deployment/d/private/..\\..\\config/legacy-secrets.env",
		"/datastorage/v2/data/deployment/C:/private/u/f",
	}
	for _, a := range attacks {
		code, body := raw("GET", a, "")
		if strings.Contains(body, "TOPSECRET") {
			t.Errorf("GET %s leaked a file outside playerdata/ (status %d)", a, code)
		}
		code, _ = raw("PUT", strings.Replace(a, "legacy-secrets.env", "pwned.txt", 1), "owned")
		_ = code
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(BaseDirectory), "pwned.txt")); err == nil {
		t.Errorf("PUT wrote above the executable directory")
	}
	filepath.Walk(BaseDirectory, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Name() == "pwned.txt" && !strings.Contains(p, string(filepath.Separator)+"playerdata"+string(filepath.Separator)) {
			t.Errorf("PUT wrote outside playerdata/: %s", p)
		}
		return nil
	})

	// a legitimate round-trip still works
	if code, _ := raw("PUT", "/datastorage/v2/data/deployment/dep/private/puid/save.bin", "hello"); code != 200 && code != 204 {
		t.Errorf("legit PUT status %d", code)
	}
	if code, body := raw("GET", "/datastorage/v2/data/deployment/dep/private/puid/save.bin", ""); code != 200 || body != "hello" {
		t.Errorf("legit GET %d %q", code, body)
	}
}

func TestCanonicalizePath(t *testing.T) {
	cases := map[string]string{
		"/a/b/../c":       "/a/c",
		"/a/./b":          "/a/b",
		"/../../x":        "/x",
		"/a/%2e%2e/b":     "/b",
		"/a\\..\\b":       "/b",
		"/a/b/..":         "/a/",
		"/v1/items/x.y/z": "/v1/items/x.y/z",
		"//v2//player/":   "//v2//player/",
	}
	for in, want := range cases {
		if got := hosting.CanonicalizePath(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
