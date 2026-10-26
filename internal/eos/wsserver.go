// Port of AUnrealFeatures.EOSSDK/EosWsServer.cs.
//
// Listens on port 80 and accepts the EOS STOMP WebSocket connections
// (notifications + lobby) that the EOS SDK sends to the bare hostname
// before Fiddler/proxy redirection is in place for WS traffic.
package eos

import (
	"net/http"

	"solara/internal/hosting"
)

const (
	wsServerHostname = "localhost" // EosWsServer.HOSTNAME
	wsServerPort     = 80          // EosWsServer.PORT
)

// EosWsServer mirrors the C# server class.
type EosWsServer struct {
	HTTP *hosting.Server
}

// NewEosWsServer builds, registers and starts the server (constructor semantics).
func NewEosWsServer() (*EosWsServer, error) {
	s := newEosWsServer()
	if err := s.HTTP.Start(); err != nil {
		return nil, err
	}
	return s, nil
}

func newEosWsServer() *EosWsServer {
	s := &EosWsServer{HTTP: hosting.NewServer("EosWsServer", wsServerHostname, wsServerPort)}
	s.HTTP.Handle("GET", "/", "Root", s.Root)
	s.HTTP.WebSocket = s.HandleWebSocketAsync
	return s
}

// Root returns 404 for any plain HTTP that lands here.
func (s *EosWsServer) Root(c *hosting.Context) hosting.Result { return hosting.NotFound(nil) }

// HandleWebSocketAsync mirrors EosWsServer.HandleWebSocketAsync.
func (s *EosWsServer) HandleWebSocketAsync(w http.ResponseWriter, r *http.Request) bool {
	path := r.URL.EscapedPath()
	logger := s.HTTP.Logger

	if IsLobbyPath(path) {
		logger.Information("WS[80] lobby | " + path)
		// Epic sends Sec-WebSocket-Protocol: ws but expects no subprotocol echoed back
		auth, _ := hosting.NewRequest(r).Header("Authorization")
		ownerPuid := ExtractPuidFromBearer(auth)
		ws := acceptWebSocket(w, r, nil)
		func() {
			defer finishWebSocket(ws)
			LobbyWsHelperRunAsync(ws, path, ownerPuid, func(msg string) { logger.Information(msg) })
		}()
		return true
	}

	if !IsStompPath(path) {
		return false
	}

	logger.Information("WS[80] stomp | " + path)
	ws2 := acceptWebSocket(w, r, stompSubprotocol(r))
	func() {
		defer finishWebSocket(ws2)
		defer func() {
			if p := recover(); p != nil {
				logger.Error("STOMP error | " + hosting.ExceptionMessage(p))
			}
		}()
		if err := StompHelperRunAsync(ws2, path, func(msg string) { logger.Information(msg) }); err != nil {
			logger.Error("STOMP error | " + hosting.ExceptionMessage(err))
		}
	}()
	return true
}
