// Package logging reproduces the original Serilog configuration
// (ModuleInitialization.InitializeLogger): minimum level Debug, a console sink
// and a daily-rolling file sink under logs/, one "Bucket" name per logger.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	// console: [{Timestamp:HH:mm:ss}] [{Bucket} - {ProcessId}/{ThreadId}] [{Level:u3}] {Message:lj}{NewLine}{Exception}
	// file:    [{Timestamp:MM/dd/yy HH:mm:ss}] [{Bucket} - {ProcessId}/{ThreadId}] [{SourceFile}({Method}:{LineNumber})] [{Level:u3}] {Message:lj}{NewLine}{Exception}
	threadID = 1 // Go has no stable thread ids; see KNOWN_ISSUES.md (LOG-001)
)

// Logger is one Serilog logger instance ("bucket").
type Logger struct {
	bucket string
	file   *rollingFile
}

var (
	consoleMu sync.Mutex
	// LogDir is where file sinks write (relative to the working directory, as
	// in the original "logs/log-.txt").
	LogDir = "logs"
	// DisableFiles turns off file sinks (used by tests).
	DisableFiles = false
)

// New mirrors ModuleInitialization.InitializeLogger(name). Each logger owns
// its own file sink; like Serilog.Sinks.File, a sink that finds the day's
// file already held by another sink rolls to a sequence-numbered name.
func New(bucket string) *Logger {
	l := &Logger{bucket: bucket}
	if !DisableFiles {
		l.file = &rollingFile{}
	}
	return l
}

var tokenRe = regexp.MustCompile(`\{[@$]?([A-Za-z0-9_]+)(:[^}]*)?\}`)

// render applies a Serilog message template positionally with :lj semantics
// (strings rendered without quotes).
func render(tmpl string, args []any) string {
	i := 0
	out := tokenRe.ReplaceAllStringFunc(tmpl, func(tok string) string {
		if i >= len(args) {
			return tok
		}
		v := args[i]
		i++
		if s, ok := v.(fmt.Stringer); ok {
			return s.String()
		}
		if b, ok := v.(bool); ok {
			if b {
				return "True"
			}
			return "False"
		}
		return fmt.Sprint(v)
	})
	return strings.ReplaceAll(strings.ReplaceAll(out, "{{", "{"), "}}", "}")
}

func (l *Logger) write(level, tmpl string, args []any) {
	if l == nil {
		return
	}
	now := time.Now()
	msg := render(tmpl, args)
	pid := os.Getpid()
	consoleMu.Lock()
	fmt.Fprintf(os.Stdout, "[%s] [%s - %d/%d] [%s] %s\n", now.Format("15:04:05"), l.bucket, pid, threadID, level, msg)
	consoleMu.Unlock()
	if l.file != nil {
		// The CallerInfo enricher only resolves frames from assemblies prefixed
		// "Astra." (none exist), so SourceFile/Method/LineNumber are absent.
		l.file.write(now, fmt.Sprintf("[%s] [%s - %d/%d] [()] [%s] %s\n", now.Format("01/02/06 15:04:05"), l.bucket, pid, threadID, level, msg))
	}
}

func (l *Logger) Debug(tmpl string, args ...any)       { l.write("DBG", tmpl, args) }
func (l *Logger) Information(tmpl string, args ...any) { l.write("INF", tmpl, args) }
func (l *Logger) Warning(tmpl string, args ...any)     { l.write("WRN", tmpl, args) }
func (l *Logger) Error(tmpl string, args ...any)       { l.write("ERR", tmpl, args) }
func (l *Logger) Fatal(tmpl string, args ...any)       { l.write("FTL", tmpl, args) }

// Console mirrors Console.WriteLine, which the original uses extensively.
func Console(format string, args ...any) {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	if len(args) == 0 {
		fmt.Fprintln(os.Stdout, format)
		return
	}
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// Main is the global Log.Logger ("Main" bucket).
var Main = New("Main")

// --- rolling file sink -----------------------------------------------------

var (
	claimedMu sync.Mutex
	claimed   = map[string]bool{}
)

type rollingFile struct {
	mu   sync.Mutex
	day  string
	f    *os.File
	path string
}

func (r *rollingFile) write(now time.Time, line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	day := now.Format("20060102")
	if r.f == nil || r.day != day {
		r.open(day)
	}
	if r.f != nil {
		_, _ = r.f.WriteString(line)
	}
}

func (r *rollingFile) open(day string) {
	if r.f != nil {
		_ = r.f.Close()
		claimedMu.Lock()
		delete(claimed, r.path)
		claimedMu.Unlock()
		r.f = nil
	}
	_ = os.MkdirAll(LogDir, 0o755)
	claimedMu.Lock()
	defer claimedMu.Unlock()
	for seq := 0; seq < 1000; seq++ {
		name := "log-" + day + ".txt"
		if seq > 0 {
			name = fmt.Sprintf("log-%s_%03d.txt", day, seq)
		}
		p := filepath.Join(LogDir, name)
		if claimed[p] {
			continue
		}
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			continue
		}
		claimed[p] = true
		r.f, r.path, r.day = f, p, day
		return
	}
}
