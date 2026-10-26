package pak

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Line ending styles.
const (
	CRLF = "CRLF"
	LF   = "LF"
)

// Text encodings recognised by DecodeIni.
const (
	EncUTF8    = "utf-8"
	EncUTF16LE = "utf-16le"
	EncUTF16BE = "utf-16be"
)

// IniDoc is an ini file decoded for editing.
type IniDoc struct {
	Text            string // LF-normalised text
	LineEnding      string // CRLF or LF (dominant style of the original)
	BOM             bool
	Encoding        string
	TrailingNewline bool
	MixedEndings    bool // original mixed CRLF and LF line endings
}

// DecodeIni decodes raw ini bytes (UTF-8 with/without BOM, UTF-16 with BOM).
func DecodeIni(b []byte) (*IniDoc, error) {
	d := &IniDoc{Encoding: EncUTF8}
	var s string
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		d.BOM = true
		b = b[3:]
		if !utf8.Valid(b) {
			return nil, errors.New("file is not valid UTF-8")
		}
		s = string(b)
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}), bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		d.BOM = true
		le := b[0] == 0xFF
		d.Encoding = EncUTF16BE
		if le {
			d.Encoding = EncUTF16LE
		}
		b = b[2:]
		if len(b)%2 != 0 {
			return nil, errors.New("UTF-16 file has an odd number of bytes")
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			if le {
				u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
			} else {
				u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
			}
		}
		s = string(utf16.Decode(u))
	default:
		if !utf8.Valid(b) {
			return nil, errors.New("file is not valid UTF-8 (and has no UTF-16 byte order mark)")
		}
		s = string(b)
	}
	crlf := strings.Count(s, "\r\n")
	lf := strings.Count(s, "\n") - crlf
	d.LineEnding = LF
	if crlf > 0 && crlf >= lf {
		d.LineEnding = CRLF
	}
	d.MixedEndings = crlf > 0 && lf > 0
	d.TrailingNewline = strings.HasSuffix(s, "\n")
	d.Text = strings.ReplaceAll(s, "\r\n", "\n")
	return d, nil
}

// Encode converts edited (LF or mixed) text back to the original file
// conventions: line endings, BOM, encoding and trailing-newline policy.
func (d *IniDoc) Encode(text string) []byte {
	t := strings.ReplaceAll(text, "\r\n", "\n")
	if d.TrailingNewline {
		if t != "" && !strings.HasSuffix(t, "\n") {
			t += "\n"
		}
	} else {
		t = strings.TrimSuffix(t, "\n")
	}
	if d.LineEnding == CRLF {
		t = strings.ReplaceAll(t, "\n", "\r\n")
	}
	var out []byte
	switch d.Encoding {
	case EncUTF16LE, EncUTF16BE:
		u := utf16.Encode([]rune(t))
		out = make([]byte, 0, 2+2*len(u))
		le := d.Encoding == EncUTF16LE
		if le {
			out = append(out, 0xFF, 0xFE)
		} else {
			out = append(out, 0xFE, 0xFF)
		}
		for _, c := range u {
			if le {
				out = append(out, byte(c), byte(c>>8))
			} else {
				out = append(out, byte(c>>8), byte(c))
			}
		}
	default:
		if d.BOM {
			out = append(out, 0xEF, 0xBB, 0xBF)
		}
		out = append(out, t...)
	}
	return out
}

// Issue severities.
const (
	SevError   = "error"
	SevWarning = "warning"
)

// Issue is a validation finding; Line is 1-based.
type Issue struct {
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Section is a [Section] header and its 1-based line.
type Section struct {
	Name string `json:"name"`
	Line int    `json:"line"`
}

// HasErrors reports whether any issue is an error.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SevError {
			return true
		}
	}
	return false
}

// ValidateIni checks Unreal ini syntax and returns issues and the section index.
func ValidateIni(text string) ([]Issue, []Section) {
	issues := []Issue{}
	sections := []Section{}
	add := func(line int, sev, format string, a ...any) {
		issues = append(issues, Issue{Line: line, Severity: sev, Message: fmt.Sprintf(format, a...)})
	}
	if !utf8.ValidString(text) {
		add(0, SevError, "text is not valid UTF-8")
	}
	text = strings.TrimPrefix(text, string([]byte{0xEF, 0xBB, 0xBF}))
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	inSection := false
	warnedOutside := false
	for i, raw := range lines {
		n := i + 1
		if bad := controlChar(raw); bad >= 0 {
			if bad == 0 {
				add(n, SevError, "NUL character in line")
			} else {
				add(n, SevError, "control character 0x%02X in line", bad)
			}
		}
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			continue
		case line[0] == ';' || line[0] == '#':
			continue
		case line[0] == '[':
			end := strings.IndexByte(line, ']')
			if end < 0 {
				add(n, SevWarning, "section header is missing a closing ']'")
				sections = append(sections, Section{Name: strings.TrimSpace(line[1:]), Line: n})
			} else {
				name := strings.TrimSpace(line[1:end])
				if name == "" {
					add(n, SevWarning, "empty section name")
				}
				if rest := strings.TrimSpace(line[end+1:]); rest != "" {
					add(n, SevWarning, "unexpected text after section header: %q", rest)
				}
				if strings.Count(line, "[") != strings.Count(line, "]") {
					add(n, SevWarning, "unbalanced brackets in section header")
				}
				sections = append(sections, Section{Name: name, Line: n})
			}
			inSection = true
			continue
		case line[0] == ']':
			add(n, SevWarning, "line starts with ']'")
			continue
		}
		if !inSection && !warnedOutside {
			add(n, SevWarning, "key outside of any [Section] (Unreal ignores it)")
			warnedOutside = true
		}
		body := line
		if strings.ContainsRune("+-.!^@*", rune(body[0])) {
			body = body[1:]
		}
		eq := strings.IndexByte(body, '=')
		if eq < 0 {
			add(n, SevWarning, "line has no '=' (not a Key=Value pair; Unreal ignores it)")
			continue
		}
		key := strings.TrimSpace(body[:eq])
		if key == "" {
			add(n, SevWarning, "empty key before '='")
		} else if strings.ContainsAny(key, "\"()") {
			add(n, SevWarning, "unusual characters in key %q", key)
		}
		checkValue(n, body[eq+1:], add)
	}
	return issues, sections
}

// controlChar returns the first disallowed control byte, or -1.
func controlChar(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 0x20 && c != '\t' && c != '\r') || c == 0x7F {
			return int(c)
		}
	}
	return -1
}

// checkValue warns on unbalanced quotes and parentheses (outside quotes).
func checkValue(n int, v string, add func(int, string, string, ...any)) {
	depth := 0
	inQuote := false
	negative := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\\' && inQuote && i+1 < len(v):
			i++ // escaped character inside quotes
		case c == '"':
			inQuote = !inQuote
		case inQuote:
		case c == '(':
			depth++
		case c == ')':
			depth--
			if depth < 0 {
				negative = true
				depth = 0
			}
		}
	}
	if inQuote {
		add(n, SevWarning, "unbalanced quotes in value")
	}
	if depth > 0 || negative {
		add(n, SevWarning, "unbalanced parentheses in value")
	}
}
