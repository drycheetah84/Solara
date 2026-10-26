// Port of AUnrealFeatures.EOSSDK/StompHelper.cs (EOS notifications STOMP socket).
package eos

import (
	"strings"
	"time"

	"solara/internal/dotnet"
)

// StompHeader is one header of a frame.
type StompHeader struct {
	Key   string
	Value string
}

// StompHeaders mirrors the Dictionary<string, string> used for frame
// headers: enumeration follows insertion order; ParseFrame's dictionary is
// OrdinalIgnoreCase (a repeated header keeps the first spelling and position
// and takes the last value).
type StompHeaders []StompHeader

// set mirrors the indexer setter; ci selects OrdinalIgnoreCase key matching.
func (h *StompHeaders) set(k, v string, ci bool) {
	for i := range *h {
		if (ci && strings.EqualFold((*h)[i].Key, k)) || (*h)[i].Key == k {
			(*h)[i].Value = v
			return
		}
	}
	*h = append(*h, StompHeader{k, v})
}

// TryGetValue mirrors the OrdinalIgnoreCase lookup of ParseFrame's dictionary.
func (h StompHeaders) TryGetValue(k string) (string, bool) {
	for _, kv := range h {
		if strings.EqualFold(kv.Key, k) {
			return kv.Value, true
		}
	}
	return "", false
}

// IsStompPath mirrors StompHelper.IsStompPath.
func IsStompPath(path string) bool {
	return hasPrefixFold(path, "/notifications/v1/") && hasSuffixFold(path, "/connect")
}

func hasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix)
}

// StompHelperRunAsync mirrors StompHelper.RunAsync (renamed: LobbyWsHelper
// also has a static RunAsync). Receive failures are returned as the error the
// original let propagate to HandleWebSocketAsync.
func StompHelperRunAsync(ws *WebSocket, path string, log func(string)) (err error) {
	buf := make([]byte, 64*1024)
	sessionId := dotnet.NewGuidN()[:16]
	_ = OpenClient(sessionId, path)

	heartbeatStop := make(chan struct{})
	var stopped bool
	cancel := func() {
		if !stopped {
			stopped = true
			close(heartbeatStop)
		}
	}

	go func() {
		nl := []byte("\n")
		for {
			// Task.Delay(30_000, token).ContinueWith(_ => { }) — cancellation ends the delay early
			select {
			case <-time.After(30 * time.Second):
			case <-heartbeatStop:
			}
			if ws.State() == WsOpen {
				if ws.SendText(nl) != nil {
					return // faulted task
				}
			}
			select {
			case <-heartbeatStop:
				return
			default:
			}
		}
	}()

	defer func() {
		cancel()
		CloseClient(sessionId)
	}()

	for ws.State() == WsOpen {
		n, mt, rerr := ws.Receive(buf)
		if rerr != nil {
			return rerr
		}
		if mt == WsClose {
			break
		}

		raw := strings.TrimRight(utf8GetString(buf[:n]), "\x00")
		if strings.TrimSpace(raw) == "" {
			continue
		}

		command, headers, body := ParseFrame(raw)
		log("STOMP " + command + " | " + sessionId + " | " + path)

		// Log inbound frame
		PushWsMessage(sessionId, "in", command, headerString(headers), body)

		switch strings.ToUpper(command) {
		case "CONNECT", "STOMP":
			connected := StompHeaders{
				{"version", "1.2"},
				{"session", sessionId},
				{"heart-beat", "0,0"},
				{"server", "EOS/1.0"},
			}
			if err := sendFrameTracked(ws, sessionId, "CONNECTED", connected, ""); err != nil {
				return err
			}

		case "SUBSCRIBE", "UNSUBSCRIBE", "SEND":
			if rid, ok := headers.TryGetValue("receipt"); ok {
				if err := sendFrameTracked(ws, sessionId, "RECEIPT", StompHeaders{{"receipt-id", rid}}, ""); err != nil {
					return err
				}
			}

		case "DISCONNECT":
			cancel()
			if drid, ok := headers.TryGetValue("receipt"); ok {
				if err := sendFrameTracked(ws, sessionId, "RECEIPT", StompHeaders{{"receipt-id", drid}}, ""); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return nil
}

func sendFrameTracked(ws *WebSocket, sessionId, command string, headers StompHeaders, body string) error {
	PushWsMessage(sessionId, "out", command, headerString(headers), body)
	return SendFrame(ws, command, headers, body)
}

// EncodeFrame builds the wire text of a frame: COMMAND\n(key:value\n)*\nbody\0.
func EncodeFrame(command string, headers StompHeaders, body string) []byte {
	var sb strings.Builder
	sb.WriteString(command)
	sb.WriteByte('\n')
	for _, kv := range headers {
		sb.WriteString(kv.Key)
		sb.WriteByte(':')
		sb.WriteString(kv.Value)
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')
	sb.WriteString(body)
	sb.WriteByte(0)
	return []byte(sb.String())
}

// SendFrame mirrors StompHelper.SendFrame (sent only while the socket is Open).
func SendFrame(ws *WebSocket, command string, headers StompHeaders, body string) error {
	bytes := EncodeFrame(command, headers, body)
	if ws.State() == WsOpen {
		return ws.SendText(bytes)
	}
	return nil
}

// ParseFrame mirrors StompHelper.ParseFrame.
func ParseFrame(raw string) (string, StompHeaders, string) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	command := strings.TrimSpace(lines[0])
	headers := StompHeaders{}
	i := 1
	for ; i < len(lines); i++ {
		line := lines[i]
		if len(line) == 0 {
			i++
			break
		}
		colon := strings.IndexByte(line, ':')
		if colon > 0 {
			headers.set(strings.TrimSpace(line[:colon]), strings.TrimSpace(line[colon+1:]), true)
		}
	}
	body := ""
	if i < len(lines) {
		body = strings.TrimRight(strings.Join(lines[i:], "\n"), "\x00")
	}
	return command, headers, body
}
