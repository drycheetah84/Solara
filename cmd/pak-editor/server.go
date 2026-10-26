package main

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"solara/internal/pak"
)

const maxBody = 16 << 20

// server is the local pak editor HTTP API + embedded UI.
type server struct {
	token string
	port  int
	ui    fs.FS
	jobs  *jobManager
	op    chan struct{} // one save/restore at a time
	// saveOptions returns the base options for save/restore (tests).
	saveOptions func() *pak.SaveOptions
	// settingsPath is where settings.json is persisted ("" = not persisted).
	settingsPath string
	// loadOodle loads and validates an Oodle DLL (replaceable in tests).
	loadOodle func(path string) (oodleCodec, error)
	oodle     oodleState
}

func newServer(token string, port int, ui fs.FS) *server {
	return &server{
		token:       token,
		port:        port,
		ui:          ui,
		jobs:        newJobManager(),
		op:          make(chan struct{}, 1),
		saveOptions: func() *pak.SaveOptions { return &pak.SaveOptions{} },
		loadOodle:   defaultLoadOodle,
	}
}

func (s *server) handler() http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("POST /api/scan", s.handleScan)
	api.HandleFunc("GET /api/file", s.handleFile)
	api.HandleFunc("POST /api/validate", s.handleValidate)
	api.HandleFunc("POST /api/save", s.handleSave)
	api.HandleFunc("GET /api/jobs/{id}", s.handleJob)
	api.HandleFunc("GET /api/backups", s.handleBackups)
	api.HandleFunc("POST /api/restore", s.handleRestore)
	api.HandleFunc("GET /api/settings", s.handleGetSettings)
	api.HandleFunc("POST /api/settings", s.handlePostSettings)
	api.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "unknown API endpoint")
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAllowed(r.Host) {
			writeError(w, http.StatusForbidden, "forbidden host")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			got := r.Header.Get("X-Session-Token")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				writeError(w, http.StatusUnauthorized, "missing or invalid session token")
				return
			}
			if r.Method == http.MethodPost {
				mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if mt != "application/json" {
					writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			api.ServeHTTP(w, r)
			return
		}
		s.serveUI(w, r)
	})
}

// hostAllowed implements DNS-rebinding protection.
func (s *server) hostAllowed(host string) bool {
	p := strconv.Itoa(s.port)
	return host == "127.0.0.1:"+p || host == "localhost:"+p
}

func (s *server) serveUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if st, err := fs.Stat(s.ui, name); err != nil || st.IsDir() {
		name = "index.html"
	}
	if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	b, err := fs.ReadFile(s.ui, name)
	if err != nil {
		http.Error(w, "UI not built", http.StatusNotFound)
		return
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(b)))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeErrorCode adds a machine-readable "code" to the error body.
func writeErrorCode(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

// writeReadError maps entry read errors; a missing Oodle DLL gets the code
// "oodle_required".
func writeReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, pak.ErrOodleRequired) {
		writeErrorCode(w, http.StatusUnprocessableEntity, err.Error(), "oodle_required")
		return
	}
	writeError(w, http.StatusUnprocessableEntity, err.Error())
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large (limit 16 MiB)")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		}
		return false
	}
	return true
}

// resolvePak validates a pak path supplied by the client.
func resolvePak(p string) (string, error) {
	p = cleanInputPath(p)
	if p == "" {
		return "", errors.New("pak path is required")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(filepath.Ext(abs), ".pak") {
		return "", errors.New("not a .pak file")
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("cannot access pak: %v", err)
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("pak path is not a regular file")
	}
	return abs, nil
}

func cleanInputPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "\"'")
	return strings.TrimSpace(p)
}

func isIniName(name string) bool { return strings.HasSuffix(strings.ToLower(name), ".ini") }

func sha1Hex(h [20]byte) string { return hex.EncodeToString(h[:]) }

// --- /api/file -------------------------------------------------------------

type fileResponse struct {
	Name       string        `json:"name"`
	Text       string        `json:"text"`
	LineEnding string        `json:"lineEnding"`
	BOM        bool          `json:"bom"`
	Size       int           `json:"size"`
	SHA1       string        `json:"sha1"`
	Sections   []pak.Section `json:"sections"`
	Issues     []pak.Issue   `json:"issues"`
	// Compression is the entry's compression method name ("" = stored
	// uncompressed).
	Compression string `json:"compression"`
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	pakPath, err := resolvePak(r.URL.Query().Get("pak"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name := r.URL.Query().Get("name")
	if !isIniName(name) {
		writeError(w, http.StatusBadRequest, "only .ini entries can be opened")
		return
	}
	p, err := pak.Open(pakPath)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	p.Decompressor = s.decompressor()
	e, ok := p.Entry(name)
	if !ok {
		writeError(w, http.StatusNotFound, "entry not found in pak: "+name)
		return
	}
	data, err := p.ReadEntry(name)
	if err != nil {
		writeReadError(w, err)
		return
	}
	doc, err := pak.DecodeIni(data)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, name+": "+err.Error())
		return
	}
	issues, sections := pak.ValidateIni(doc.Text)
	writeJSON(w, http.StatusOK, fileResponse{
		Name: name, Text: doc.Text, LineEnding: doc.LineEnding, BOM: doc.BOM,
		Size: len(data), SHA1: sha1Hex(e.Hash), Sections: sections, Issues: issues,
		Compression: p.MethodName(e),
	})
}

// --- /api/validate ---------------------------------------------------------

func (s *server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Text string `json:"text"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	issues, sections := pak.ValidateIni(req.Text)
	writeJSON(w, http.StatusOK, map[string]any{"issues": issues, "sections": sections})
}

// --- /api/save -------------------------------------------------------------

type fileIssue struct {
	File string `json:"file"`
	pak.Issue
}

func (s *server) handleSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pak        string            `json:"pak"`
		Files      map[string]string `json:"files"`
		ExpectSha1 map[string]string `json:"expectSha1"`
		Recompress bool              `json:"recompress"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	pakPath, err := resolvePak(req.Pak)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Files) == 0 {
		writeError(w, http.StatusBadRequest, "no files given")
		return
	}
	p, err := pak.Open(pakPath)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	codec := s.codec()
	p.Decompressor = s.decompressor()
	if req.Recompress {
		if p.Footer.Version < pak.VersionFnv64BugFix {
			writeError(w, http.StatusBadRequest, "recompressing is only supported for version 11 paks")
			return
		}
		if codec == nil {
			writeErrorCode(w, http.StatusUnprocessableEntity, "recompressing requires the Oodle DLL", "oodle_required")
			return
		}
		if !codec.CanCompress() {
			writeError(w, http.StatusUnprocessableEntity, "the configured Oodle DLL does not export OodleLZ_Compress; save without recompressing")
			return
		}
	}
	var issues []fileIssue
	repl := map[string][]byte{}
	for name, text := range req.Files {
		if !isIniName(name) {
			writeError(w, http.StatusBadRequest, "only .ini entries can be saved: "+name)
			return
		}
		e, ok := p.Entry(name)
		if !ok {
			writeError(w, http.StatusNotFound, "entry not found in pak: "+name)
			return
		}
		if !e.Editable() {
			writeError(w, http.StatusUnprocessableEntity, name+" is encrypted (or compressed in a version 8 pak) and cannot be edited")
			return
		}
		want, ok := req.ExpectSha1[name]
		if !ok || want == "" {
			writeError(w, http.StatusBadRequest, "expectSha1 missing for "+name)
			return
		}
		if !strings.EqualFold(want, sha1Hex(e.Hash)) {
			writeError(w, http.StatusConflict, name+" changed on disk since it was loaded; reload it before saving")
			return
		}
		orig, err := p.ReadEntry(name)
		if err != nil {
			writeReadError(w, err)
			return
		}
		doc, err := pak.DecodeIni(orig)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, name+": "+err.Error())
			return
		}
		iss, _ := pak.ValidateIni(text)
		for _, i := range iss {
			if i.Severity == pak.SevError {
				issues = append(issues, fileIssue{File: name, Issue: i})
			}
		}
		if nb := doc.Encode(text); string(nb) != string(orig) {
			repl[name] = nb
		}
	}
	if len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation failed; fix the errors before saving", "issues": issues})
		return
	}
	if len(repl) == 0 {
		writeError(w, http.StatusBadRequest, "no changes")
		return
	}
	s.startOp(w, func(opts *pak.SaveOptions) (*jobResult, error) {
		opts.Recompress = req.Recompress
		if codec != nil {
			opts.Codec = codec
		}
		res, err := pak.Save(pakPath, repl, opts)
		if err != nil {
			return nil, err
		}
		return &jobResult{Backup: res.Backup, OriginalBackup: res.OriginalBackup, NewSize: res.NewSize, Changed: res.Changed, Recompressed: nonNil(res.Recompressed)}, nil
	})
}

// startOp runs a save/restore as a background job (one at a time).
func (s *server) startOp(w http.ResponseWriter, run func(*pak.SaveOptions) (*jobResult, error)) {
	select {
	case s.op <- struct{}{}:
	default:
		writeError(w, http.StatusConflict, "another save or restore is in progress")
		return
	}
	j := s.jobs.create()
	opts := s.saveOptions()
	opts.Progress = j.setPhase
	go func() {
		defer func() { <-s.op }()
		res, err := run(opts)
		j.finish(res, err)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"jobId": j.id})
}

// waitIdle blocks until no save/restore is running (used on shutdown).
func (s *server) waitIdle() {
	s.op <- struct{}{}
}

func (s *server) handleJob(w http.ResponseWriter, r *http.Request) {
	j := s.jobs.get(r.PathValue("id"))
	if j == nil {
		writeError(w, http.StatusNotFound, "unknown job")
		return
	}
	writeJSON(w, http.StatusOK, j.snapshot())
}

// --- /api/backups, /api/restore -------------------------------------------

type backupInfo struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	Kind    string `json:"kind"`
}

func (s *server) handleBackups(w http.ResponseWriter, r *http.Request) {
	pakPath, err := resolvePak(r.URL.Query().Get("pak"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bs, err := pak.ListBackups(pakPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []backupInfo{}
	for _, b := range bs {
		out = append(out, backupInfo{Path: b.Path, Name: b.Name, Size: b.Size, ModTime: b.ModTime.Format(time.RFC3339), Kind: b.Kind})
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": out})
}

func (s *server) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Pak    string `json:"pak"`
		Backup string `json:"backup"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	pakPath, err := resolvePak(req.Pak)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bs, err := pak.ListBackups(pakPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	want, _ := filepath.Abs(cleanInputPath(req.Backup))
	backup := ""
	for _, b := range bs {
		if strings.EqualFold(b.Path, want) {
			backup = b.Path
		}
	}
	if backup == "" {
		writeError(w, http.StatusBadRequest, "not a known backup of this pak")
		return
	}
	s.startOp(w, func(opts *pak.SaveOptions) (*jobResult, error) {
		res, err := pak.Restore(pakPath, backup, opts)
		if err != nil {
			return nil, err
		}
		return &jobResult{Backup: res.Backup, OriginalBackup: res.OriginalBackup, NewSize: res.NewSize, Changed: []string{}, Recompressed: []string{}}, nil
	})
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
