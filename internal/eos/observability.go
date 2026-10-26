// Port of AUnrealFeatures.EOSSDK/EosObservabilityStore.cs (TelemetryEvent,
// WsMessage, WsClientSession records and the static EosObservabilityStore).
package eos

import (
	"bytes"
	"sort"
	"sync"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// ── Telemetry ─────────────────────────────────────────────────────────────────

// TelemetryEvent mirrors `record TelemetryEvent(string Id, string EventName, string Payload, DateTime Timestamp)`.
type TelemetryEvent struct {
	Id        string
	EventName string
	Payload   string
	Timestamp dotnet.DateTime
}

// ── WebSocket messages ────────────────────────────────────────────────────────

// WsMessage mirrors `record WsMessage(Direction, Command, Headers, Body, Timestamp)`.
type WsMessage struct {
	Direction string // "in" | "out"
	Command   string
	Headers   string
	Body      string
	Timestamp dotnet.DateTime
}

// WsClientSession mirrors the C# class of the same name.
type WsClientSession struct {
	SessionId   string
	Path        string
	ConnectedAt dotnet.DateTime
	// DisconnectedAt is set by CloseClient; read it through Disconnected()
	// when other goroutines may be closing the session.
	DisconnectedAt *dotnet.DateTime

	messages []WsMessage
	lock     sync.Mutex
}

// Disconnected returns DisconnectedAt under the session lock.
func (s *WsClientSession) Disconnected() *dotnet.DateTime {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.DisconnectedAt
}

// MessageCount mirrors the getter-only property.
func (s *WsClientSession) MessageCount() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return len(s.messages)
}

// AddMessage keeps at most 500 messages (oldest dropped).
func (s *WsClientSession) AddMessage(msg WsMessage) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if len(s.messages) >= 500 {
		s.messages = append(s.messages[:0:0], s.messages[1:]...)
	}
	s.messages = append(s.messages, msg)
}

// GetMessages returns a copy of the messages.
func (s *WsClientSession) GetMessages() []WsMessage {
	s.lock.Lock()
	defer s.lock.Unlock()
	return append([]WsMessage{}, s.messages...)
}

// MarshalSTJ reproduces the default serialization of the class (public
// properties in declaration order, including the computed MessageCount).
func (s *WsClientSession) MarshalSTJ() ([]byte, error) {
	return stj.Marshal(stj.O("SessionId", s.SessionId, "Path", s.Path, "ConnectedAt", s.ConnectedAt,
		"DisconnectedAt", s.Disconnected(), "MessageCount", s.MessageCount()))
}

// ── Store ─────────────────────────────────────────────────────────────────────

const (
	maxTelemetry = 1000 // MAX_TELEMETRY
	maxClients   = 200  // MAX_CLIENTS
)

var (
	// Telemetry — ring buffer, newest last
	telemetry []TelemetryEvent
	telLock   sync.Mutex

	// WS clients — keyed by session_id, capped at 200 (ConcurrentDictionary + ConcurrentQueue)
	clientsMu   sync.Mutex
	clients     = map[string]*WsClientSession{}
	clientOrder []string // _clientOrder queue
	clientSeq   = map[string]int64{}
	clientNext  int64
)

// PushTelemetry mirrors EosObservabilityStore.PushTelemetry.
func PushTelemetry(eventName string, payload string) {
	ev := TelemetryEvent{dotnet.NewGuidN()[:12], eventName, payload, dotnet.UtcNow()}
	telLock.Lock()
	defer telLock.Unlock()
	if len(telemetry) >= maxTelemetry {
		telemetry = append(telemetry[:0:0], telemetry[1:]...)
	}
	telemetry = append(telemetry, ev)
}

// GetTelemetry mirrors EosObservabilityStore.GetTelemetry(limit = 200): the
// last `limit` events, newest first (stable OrderByDescending(Timestamp)).
func GetTelemetry(limit int) []TelemetryEvent {
	telLock.Lock()
	defer telLock.Unlock()
	start := len(telemetry) - limit
	if start < 0 {
		start = 0
	}
	if len(telemetry)-start < 0 {
		dotnet.Throw("System.ArgumentException", "Offset and length were out of bounds for the array or count is greater than the number of elements from index to the end of the source collection.")
	}
	out := append([]TelemetryEvent{}, telemetry[start:]...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	return out
}

// OpenClient mirrors EosObservabilityStore.OpenClient.
func OpenClient(sessionId string, path string) *WsClientSession {
	session := &WsClientSession{SessionId: sessionId, Path: path, ConnectedAt: dotnet.UtcNow()}
	clientsMu.Lock()
	defer clientsMu.Unlock()
	if _, ok := clients[sessionId]; !ok {
		clientNext++
		clientSeq[sessionId] = clientNext
	}
	clients[sessionId] = session
	clientOrder = append(clientOrder, sessionId)

	// Evict oldest if over cap
	for len(clients) > maxClients && len(clientOrder) > 0 {
		old := clientOrder[0]
		clientOrder = clientOrder[1:]
		delete(clients, old)
		delete(clientSeq, old)
	}
	return session
}

// CloseClient mirrors EosObservabilityStore.CloseClient.
func CloseClient(sessionId string) {
	clientsMu.Lock()
	s, ok := clients[sessionId]
	clientsMu.Unlock()
	if ok {
		now := dotnet.UtcNow()
		s.lock.Lock()
		s.DisconnectedAt = &now
		s.lock.Unlock()
	}
}

// PushWsMessage mirrors EosObservabilityStore.PushWsMessage.
func PushWsMessage(sessionId, direction, command, headers, body string) {
	clientsMu.Lock()
	s, ok := clients[sessionId]
	clientsMu.Unlock()
	if ok {
		s.AddMessage(WsMessage{direction, command, headers, body, dotnet.UtcNow()})
	}
}

// GetClients mirrors EosObservabilityStore.GetClients (newest connection first).
func GetClients() []*WsClientSession {
	clientsMu.Lock()
	out := make([]*WsClientSession, 0, len(clients))
	for _, s := range clients {
		out = append(out, s)
	}
	seq := make(map[*WsClientSession]int64, len(out))
	for k, s := range clients {
		seq[s] = clientSeq[k]
	}
	clientsMu.Unlock()
	// deterministic base order (insertion), then stable sort by ConnectedAt desc
	sort.Slice(out, func(i, j int) bool { return seq[out[i]] < seq[out[j]] })
	sort.SliceStable(out, func(i, j int) bool { return out[i].ConnectedAt.After(out[j].ConnectedAt) })
	return out
}

// GetClient mirrors EosObservabilityStore.GetClient (nil when unknown).
func GetClient(sessionId string) *WsClientSession {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	return clients[sessionId]
}

// headerString mirrors string.Join(", ", headers.Select(kv => $"{kv.Key}:{kv.Value}")).
func headerString(h StompHeaders) string {
	var b bytes.Buffer
	for i, kv := range h {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(kv.Key)
		b.WriteByte(':')
		b.WriteString(kv.Value)
	}
	return b.String()
}
