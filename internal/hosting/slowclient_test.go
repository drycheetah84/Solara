package hosting

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Security review finding F4: one client trickling a request body must not
// hold the per-server lock and stall every other request.
func TestSlowBodyDoesNotStallServer(t *testing.T) {
	oldT, oldMax := RequestBodyTimeout, MaxRequestBodyBytes
	RequestBodyTimeout, MaxRequestBodyBytes = 1*time.Second, 1024
	defer func() { RequestBodyTimeout, MaxRequestBodyBytes = oldT, oldMax }()

	s := newTestServer()
	ts := httptest.NewServer(s)
	defer ts.Close()

	// attacker: promises 100 bytes, sends 1, then goes quiet
	conn, err := net.Dial("tcp", ts.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte("POST /v1/echo HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{"))
	time.Sleep(100 * time.Millisecond)

	// a legitimate request must still be served promptly
	done := make(chan int, 1)
	go func() {
		resp, err := http.Get(ts.URL + "/v1/items/7")
		if err != nil {
			done <- -1
			return
		}
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	select {
	case code := <-done:
		if code != 200 {
			t.Fatalf("legit request status %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server stalled behind a slow request body")
	}

	// the stalled connection is dropped once the body deadline passes
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	start := time.Now()
	buf := make([]byte, 64)
	if _, err := conn.Read(buf); err == nil || time.Since(start) > 4*time.Second {
		t.Errorf("slow connection not closed by the body deadline (err=%v after %v)", err, time.Since(start))
	}

	// oversized bodies are refused without reaching a handler
	resp, err := http.Post(ts.URL+"/v1/echo", "application/json", strings.NewReader(`{"name":"`+strings.Repeat("a", 4096)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body status %d, want 413", resp.StatusCode)
	}

	// normal bodies still work
	resp, err = http.Post(ts.URL+"/v1/echo", "application/json", strings.NewReader(`{"name":"ok"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "ok" {
		t.Errorf("normal body: %d %q", resp.StatusCode, b)
	}
}
