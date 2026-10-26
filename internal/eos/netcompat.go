// .NET runtime helpers shared by the EOSSDK port (no single C# file):
// Encoding.UTF8.GetString, UTF-16 string slicing, AppContext.BaseDirectory,
// Path.Combine/GetFileName, and JsonSerializerOptions{WriteIndented = true}.
package eos

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"solara/internal/dotnet"
	"solara/internal/stj"
)

// BaseDirectory mirrors AppContext.BaseDirectory (the directory holding the
// executable). sessions.json, lobbies.json and playerdata/ live there. It may
// be overridden before first use (tests).
var BaseDirectory = func() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}()

// utf8GetString mirrors Encoding.UTF8.GetString: invalid sequences become U+FFFD.
func utf8GetString(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size <= 1 {
			sb.WriteRune(utf8.RuneError)
			if size == 0 {
				break
			}
		} else {
			sb.WriteRune(r)
		}
		b = b[size:]
	}
	return sb.String()
}

// utf16Len mirrors string.Length.
func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

// utf16Prefix mirrors s[..n] (C# range on UTF-16 code units). It panics with
// ArgumentOutOfRangeException when n exceeds the length, as the C# does.
func utf16Prefix(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if n > len(u) {
		dotnet.Throw("System.ArgumentOutOfRangeException", "Index and length must refer to a location within the string. (Parameter 'length')")
	}
	return string(utf16.Decode(u[:n]))
}

// pathCombine mirrors Path.Combine: a rooted later segment restarts the path.
func pathCombine(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out == "" || isRooted(p) {
			out = p
			continue
		}
		if strings.HasSuffix(out, "/") || (runtime.GOOS == "windows" && (strings.HasSuffix(out, `\`) || strings.HasSuffix(out, ":"))) {
			out += p
		} else {
			out += string(os.PathSeparator) + p
		}
	}
	return out
}

func isRooted(p string) bool {
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(p, `\`) || strings.HasPrefix(p, "/") || (len(p) >= 2 && p[1] == ':')
	}
	return strings.HasPrefix(p, "/")
}

// pathGetFileName mirrors Path.GetFileName.
func pathGetFileName(p string) string {
	seps := "/"
	if runtime.GOOS == "windows" {
		seps = `/\:`
	}
	if i := strings.LastIndexAny(p, seps); i >= 0 {
		return p[i+1:]
	}
	return p
}

// newLine mirrors Environment.NewLine.
func newLine() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// serializeIndented mirrors JsonSerializer.Serialize(v, new JsonSerializerOptions
// { WriteIndented = true }): the compact output re-laid out by Utf8JsonWriter's
// indented mode (two spaces, "name": value, empty containers as [] / {}).
func serializeIndented(v any) string {
	el := stj.MustParse(stj.MustMarshal(v))
	var b bytes.Buffer
	writeIndented(&b, el, 0)
	return b.String()
}

func writeIndented(b *bytes.Buffer, el stj.Element, depth int) {
	nl := newLine()
	pad := func(d int) { b.WriteString(strings.Repeat("  ", d)) }
	switch el.ValueKind() {
	case stj.Object:
		props := el.EnumerateObject()
		if len(props) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, p := range props {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(nl)
			pad(depth + 1)
			stj.WriteString(b, p.Name)
			b.WriteString(": ")
			writeIndented(b, p.Value, depth+1)
		}
		b.WriteString(nl)
		pad(depth)
		b.WriteByte('}')
	case stj.Array:
		items := el.EnumerateArray()
		if len(items) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, it := range items {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(nl)
			pad(depth + 1)
			writeIndented(b, it, depth+1)
		}
		b.WriteString(nl)
		pad(depth)
		b.WriteByte(']')
	default:
		b.WriteString(el.GetRawText())
	}
}

// readAllText mirrors File.ReadAllText (UTF-8, BOM stripped).
func readAllText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	return utf8GetString(b), nil
}

// fileExists mirrors File.Exists.
func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// tryRun runs f, swallowing any panic (C# `try { ... } catch { }`). It
// reports whether f completed without throwing.
func tryRun(f func()) (ok bool) {
	defer func() {
		if p := recover(); p != nil {
			ok = false
		}
	}()
	f()
	return true
}

// utcNowString mirrors DateTime.UtcNow.ToString("yyyy-MM-ddTHH:mm:ss.fffZ").
func utcNowString() string { return dotnet.UtcNow().Format("yyyy-MM-ddTHH:mm:ss.fffZ") }
