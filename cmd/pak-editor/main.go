// Command pak-editor is the local "A2 Pak Config Editor": it serves a web UI
// on 127.0.0.1 for editing DefaultEngine.ini / DefaultGame.ini inside an
// Unreal Engine v8 or v11 .pak and writes the pak back through a verified,
// backed-up, atomic save pipeline (see docs/PAK_EDITOR.md).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"time"
)

func main() {
	port := flag.Int("port", 0, "port to listen on (127.0.0.1 only); 0 picks a random free port")
	noBrowser := flag.Bool("no-browser", false, "do not open the default browser")
	flag.Parse()

	if err := run(*port, !*noBrowser); err != nil {
		fmt.Fprintln(os.Stderr, "pak-editor:", err)
		os.Exit(1)
	}
}

func run(port int, openBrowser bool) error {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port
	token, err := newToken()
	if err != nil {
		return err
	}
	s := newServer(token, actualPort, uiFS())
	s.settingsPath = defaultSettingsPath()
	s.reloadOodle()
	srv := &http.Server{
		Handler:           s.handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", actualPort, token)
	fmt.Println("A2 Pak Config Editor is running.")
	fmt.Println("Open:", url)
	if st := s.settingsSnapshot(); st.OodleAvailable {
		fmt.Println("Oodle:", st.OodlePath, "(from "+st.OodleSource+")")
	} else if st.OodleError != "" {
		fmt.Println("Oodle: not available:", st.OodleError)
	} else {
		fmt.Println("Oodle: not configured (needed only for Oodle-compressed paks, e.g. Quest builds)")
	}
	fmt.Println("Press Ctrl+C to stop.")
	if openBrowser {
		if err := browse(url); err != nil {
			fmt.Fprintln(os.Stderr, "could not open a browser:", err)
		}
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-sig:
		fmt.Println("Shutting down (waiting for any running save/restore to finish)...")
		s.waitIdle()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}
	return nil
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func browse(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
