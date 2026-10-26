package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"solara/internal/pak"
)

// settings is persisted in <user config dir>/pak-editor/settings.json
// (%APPDATA%\pak-editor\settings.json on Windows).
type settings struct {
	OodleDll string `json:"oodleDll"`
}

func defaultSettingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pak-editor", "settings.json")
}

func loadSettings(path string) settings {
	var s settings
	if path == "" {
		return s
	}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func saveSettings(path string, s settings) error {
	if path == "" {
		return errors.New("no settings location available")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// oodleCodec is what the server needs from a loaded Oodle DLL.
type oodleCodec interface {
	pak.Codec
	CanCompress() bool
}

// oodleState is the currently active Oodle DLL (if any).
type oodleState struct {
	mu          sync.Mutex
	setting     string // persisted path ("" = auto-detect)
	codec       oodleCodec
	path        string
	source      string // settings, env, exe-dir
	err         string
	canCompress bool
}

type settingsResponse struct {
	OodleDll         string `json:"oodleDll"`
	OodleAvailable   bool   `json:"oodleAvailable"`
	OodlePath        string `json:"oodlePath"`
	OodleSource      string `json:"oodleSource"`
	OodleCanCompress bool   `json:"oodleCanCompress"`
	OodleSupported   bool   `json:"oodleSupported"`
	OodleError       string `json:"oodleError,omitempty"`
}

func defaultLoadOodle(path string) (oodleCodec, error) {
	o, err := pak.LoadOodle(path)
	if err != nil {
		return nil, err
	}
	return o, nil
}

// reloadOodle resolves and loads the Oodle DLL: saved setting, then the
// PAK_EDITOR_OODLE_DLL environment variable, then next to the executable.
func (s *server) reloadOodle() {
	st := loadSettings(s.settingsPath)
	path, source := pak.FindOodleDLL(st.OodleDll)
	if source == "explicit" {
		source = "settings"
	}
	o := &s.oodle
	o.mu.Lock()
	defer o.mu.Unlock()
	o.setting, o.path, o.source, o.codec, o.err, o.canCompress = st.OodleDll, path, source, nil, "", false
	if path == "" {
		return
	}
	c, err := s.loadOodle(path)
	if err != nil {
		o.err = err.Error()
		return
	}
	o.codec, o.canCompress = c, c.CanCompress()
}

// codec returns the active Oodle codec or a nil interface.
func (s *server) codec() oodleCodec {
	s.oodle.mu.Lock()
	defer s.oodle.mu.Unlock()
	if s.oodle.codec == nil {
		return nil
	}
	return s.oodle.codec
}

// decompressor returns the active codec as a pak.Decompressor (nil-safe).
func (s *server) decompressor() pak.Decompressor {
	if c := s.codec(); c != nil {
		return c
	}
	return nil
}

func (s *server) settingsSnapshot() settingsResponse {
	o := &s.oodle
	o.mu.Lock()
	defer o.mu.Unlock()
	return settingsResponse{
		OodleDll: o.setting, OodleAvailable: o.codec != nil, OodlePath: o.path, OodleSource: o.source,
		OodleCanCompress: o.canCompress, OodleSupported: pak.OodleSupported, OodleError: o.err,
	}
}

func (s *server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.settingsSnapshot())
}

func (s *server) handlePostSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OodleDll *string `json:"oodleDll"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	if req.OodleDll == nil {
		writeError(w, http.StatusBadRequest, "oodleDll is required (empty string = auto-detect)")
		return
	}
	path := cleanInputPath(*req.OodleDll)
	if path != "" {
		abs, err := filepath.Abs(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := s.loadOodle(abs); err != nil {
			writeErrorCode(w, http.StatusBadRequest, err.Error(), "oodle_invalid")
			return
		}
		path = abs
	}
	if err := saveSettings(s.settingsPath, settings{OodleDll: path}); err != nil {
		writeError(w, http.StatusInternalServerError, "saving settings: "+err.Error())
		return
	}
	s.reloadOodle()
	writeJSON(w, http.StatusOK, s.settingsSnapshot())
}
