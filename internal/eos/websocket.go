// System.Net.WebSockets.WebSocket semantics (as returned by
// HttpListenerContext.AcceptWebSocketAsync) on top of gorilla/websocket, used
// by EosGatewayServer/EosWsServer HandleWebSocketAsync, StompHelper and
// LobbyWsHelper.
package eos

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"solara/internal/dotnet"
)

// WebSocketState mirrors System.Net.WebSockets.WebSocketState.
type WebSocketState int

const (
	WsNone          WebSocketState = 0
	WsConnecting    WebSocketState = 1
	WsOpen          WebSocketState = 2
	WsCloseSent     WebSocketState = 3
	WsCloseReceived WebSocketState = 4
	WsClosed        WebSocketState = 5
	WsAborted       WebSocketState = 6
)

// WebSocketMessageType mirrors System.Net.WebSockets.WebSocketMessageType.
type WebSocketMessageType int

const (
	WsText   WebSocketMessageType = 0
	WsBinary WebSocketMessageType = 1
	WsClose  WebSocketMessageType = 2
)

// keepAliveInterval mirrors WebSocket.DefaultKeepAliveInterval (30 s), which
// HttpListener uses; ManagedWebSocket sends an unsolicited Pong on it.
const keepAliveInterval = 30 * time.Second

const remoteClosedMessage = "The remote party closed the WebSocket connection without completing the close handshake."

// WebSocket is the accepted server-side socket.
type WebSocket struct {
	conn   *websocket.Conn
	sendMu sync.Mutex // ManagedWebSocket serializes concurrent sends

	mu     sync.Mutex
	state  WebSocketState
	reader io.Reader
	rtype  WebSocketMessageType
	fresh  bool

	stop     chan struct{}
	stopOnce sync.Once
}

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(*http.Request) bool { return true },
	ReadBufferSize:  16 * 1024,
	WriteBufferSize: 16 * 1024,
}

// acceptWebSocket mirrors HttpListenerContext.AcceptWebSocketAsync(subProtocol).
// A nil subProtocol accepts without echoing a protocol. Validation failures
// panic with the .NET exception the original threw.
func acceptWebSocket(w http.ResponseWriter, r *http.Request, subProtocol *string) *WebSocket {
	var hdr http.Header
	if subProtocol != nil {
		sp := *subProtocol
		if strings.TrimSpace(sp) == "" {
			dotnet.Throw("System.ArgumentException", "Empty string is not allowed. (Parameter 'subProtocol')")
		}
		offered := strings.Join(r.Header.Values("Sec-WebSocket-Protocol"), ",")
		if strings.TrimSpace(offered) == "" {
			dotnet.Throw("System.Net.WebSockets.WebSocketException",
				"The WebSocket client did not request any protocols, but server attempted to accept with protocol '"+sp+"'.")
		}
		found := false
		for _, p := range strings.Split(offered, ",") {
			if strings.EqualFold(strings.TrimSpace(p), sp) {
				found = true
				break
			}
		}
		if !found {
			dotnet.Throw("System.Net.WebSockets.WebSocketException",
				"The WebSocket client request requested '"+offered+"' protocol(s), but server is only accepting '"+sp+"' protocol(s).")
		}
		hdr = http.Header{"Sec-Websocket-Protocol": {sp}}
	}
	c, err := upgrader.Upgrade(w, r, hdr)
	if err != nil {
		dotnet.Throw("System.Net.WebSockets.WebSocketException", err.Error())
	}
	return newWebSocket(c)
}

func newWebSocket(c *websocket.Conn) *WebSocket {
	ws := &WebSocket{conn: c, state: WsOpen, stop: make(chan struct{})}
	// ManagedWebSocket does not answer a received Close frame by itself; the
	// application decides (here: the finally blocks only close while Open).
	c.SetCloseHandler(func(int, string) error { return nil })
	go ws.keepAlive()
	return ws
}

func (ws *WebSocket) keepAlive() {
	t := time.NewTicker(keepAliveInterval)
	defer t.Stop()
	for {
		select {
		case <-ws.stop:
			return
		case <-t.C:
			st := ws.State()
			if st == WsOpen || st == WsCloseReceived {
				_ = ws.conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(10*time.Second))
			}
		}
	}
}

// State mirrors WebSocket.State.
func (ws *WebSocket) State() WebSocketState {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.state
}

func (ws *WebSocket) setState(s WebSocketState) {
	ws.mu.Lock()
	ws.state = s
	ws.mu.Unlock()
}

// Receive mirrors ReceiveAsync(buf): it returns at most len(buf) bytes of the
// current message, so a message larger than the buffer arrives in pieces.
// A received Close frame yields WsClose; transport failures return a
// WebSocketException-equivalent error.
func (ws *WebSocket) Receive(buf []byte) (int, WebSocketMessageType, error) {
	for {
		if ws.reader == nil {
			mt, r, err := ws.conn.NextReader()
			if err != nil {
				var ce *websocket.CloseError
				if errors.As(err, &ce) {
					ws.mu.Lock()
					if ws.state == WsCloseSent {
						ws.state = WsClosed
					} else if ws.state < WsCloseReceived {
						ws.state = WsCloseReceived
					}
					ws.mu.Unlock()
					return 0, WsClose, nil
				}
				ws.setState(WsAborted)
				return 0, WsClose, &dotnet.Exception{Type: "System.Net.WebSockets.WebSocketException", Message: remoteClosedMessage}
			}
			ws.reader = r
			ws.fresh = true
			ws.rtype = WsText
			if mt == websocket.BinaryMessage {
				ws.rtype = WsBinary
			}
		}
		n, err := io.ReadFull(ws.reader, buf)
		switch {
		case err == nil:
			ws.fresh = false
			return n, ws.rtype, nil
		case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
			wasFresh := ws.fresh
			ws.reader = nil
			if n == 0 && !wasFresh {
				continue // the previous piece already ended exactly at the message end
			}
			return n, ws.rtype, nil
		default:
			ws.reader = nil
			ws.setState(WsAborted)
			return 0, WsClose, &dotnet.Exception{Type: "System.Net.WebSockets.WebSocketException", Message: remoteClosedMessage}
		}
	}
}

// SendText mirrors SendAsync(bytes, Text, endOfMessage: true).
func (ws *WebSocket) SendText(b []byte) error {
	st := ws.State()
	if st != WsOpen && st != WsCloseReceived {
		return &dotnet.Exception{Type: "System.Net.WebSockets.WebSocketException",
			Message: "The WebSocket is in an invalid state ('" + st.String() + "') for this operation. Valid states are: 'Open, CloseReceived'"}
	}
	ws.sendMu.Lock()
	defer ws.sendMu.Unlock()
	if err := ws.conn.WriteMessage(websocket.TextMessage, b); err != nil {
		ws.setState(WsAborted)
		return &dotnet.Exception{Type: "System.Net.WebSockets.WebSocketException", Message: remoteClosedMessage}
	}
	return nil
}

// CloseAsync mirrors CloseAsync(status, description): send Close, then wait
// for the peer's Close frame.
func (ws *WebSocket) CloseAsync(code int, description string) error {
	ws.sendMu.Lock()
	err := ws.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, description), time.Now().Add(time.Minute))
	ws.sendMu.Unlock()
	if err != nil {
		ws.setState(WsAborted)
		return err
	}
	ws.mu.Lock()
	if ws.state == WsCloseReceived {
		ws.state = WsClosed
		ws.mu.Unlock()
		return nil
	}
	ws.state = WsCloseSent
	ws.mu.Unlock()
	buf := make([]byte, 4096)
	for ws.State() == WsCloseSent {
		if _, _, err := ws.Receive(buf); err != nil {
			return err
		}
	}
	return nil
}

// Dispose mirrors WebSocket.Dispose (aborts the connection).
func (ws *WebSocket) Dispose() {
	ws.stopOnce.Do(func() { close(ws.stop) })
	ws.mu.Lock()
	if ws.state < WsClosed {
		ws.state = WsClosed
	}
	ws.mu.Unlock()
	_ = ws.conn.Close()
}

// String mirrors the enum name.
func (s WebSocketState) String() string {
	switch s {
	case WsNone:
		return "None"
	case WsConnecting:
		return "Connecting"
	case WsOpen:
		return "Open"
	case WsCloseSent:
		return "CloseSent"
	case WsCloseReceived:
		return "CloseReceived"
	case WsClosed:
		return "Closed"
	case WsAborted:
		return "Aborted"
	}
	return "Unknown"
}

// finishWebSocket is the shared `finally` of HandleWebSocketAsync.
func finishWebSocket(ws *WebSocket) {
	if ws.State() == WsOpen {
		_ = ws.CloseAsync(1000, "bye") // NormalClosure
	}
	ws.Dispose()
}
