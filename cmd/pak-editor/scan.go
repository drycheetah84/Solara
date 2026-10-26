package main

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"solara/internal/pak"
)

const maxScanDepth = 6

type pakInfo struct {
	Path        string   `json:"path"`
	Name        string   `json:"name"`
	Size        int64    `json:"size"`
	Version     uint32   `json:"version"`
	Entries     int      `json:"entries"`
	Supported   bool     `json:"supported"`
	ConfigFiles []string `json:"configFiles"`
	// CompressionMethods are the footer's compression method names.
	CompressionMethods []string `json:"compressionMethods"`
	// OodleRequired is true when a config file is Oodle-compressed.
	OodleRequired bool `json:"oodleRequired"`
	// OodleAvailable is true when an Oodle DLL is loaded.
	OodleAvailable bool   `json:"oodleAvailable"`
	Error          string `json:"error,omitempty"`
}

type scanResponse struct {
	Root string    `json:"root"`
	Paks []pakInfo `json:"paks"`
}

func (s *server) handleScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &req) {
		return
	}
	res, err := scan(req.Path, s.codec() != nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// isPakFile matches *.pak exactly; backups (.original, .bak-*) and temp
// files (.solara-tmp) have other extensions and are ignored.
func isPakFile(name string) bool { return strings.EqualFold(filepath.Ext(name), ".pak") }

// pakFilesIn lists the *.pak files directly inside dir.
func pakFilesIn(dir string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.Type().IsRegular() && isPakFile(e.Name()) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// scan resolves a user-supplied path (game root, project dir, Content dir,
// Paks dir or .pak file) to the pak files it contains.
func scan(input string, oodleAvailable bool) (*scanResponse, error) {
	input = cleanInputPath(input)
	if input == "" {
		return nil, errors.New("path is required")
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, errors.New("path does not exist or is not accessible: " + abs)
	}
	var dirs, paks []string
	if !st.IsDir() {
		if !isPakFile(abs) {
			return nil, errors.New("not a .pak file or folder: " + abs)
		}
		dirs = []string{filepath.Dir(abs)}
		paks = []string{abs}
	} else {
		dirs = findPakDirs(abs)
		for _, d := range dirs {
			paks = append(paks, pakFilesIn(d)...)
		}
	}
	if len(paks) == 0 {
		return nil, errors.New("no Content/Paks/*.pak files found under " + abs)
	}
	res := &scanResponse{Root: dirs[0], Paks: []pakInfo{}}
	for _, p := range paks {
		res.Paks = append(res.Paks, describePak(p, oodleAvailable))
	}
	return res, nil
}

// findPakDirs finds **/Content/Paks directories (up to maxScanDepth levels
// below root). A root that directly contains .pak files counts as one.
func findPakDirs(root string) []string {
	if len(pakFilesIn(root)) > 0 {
		return []string{root}
	}
	var out []string
	rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != root {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if depth := strings.Count(filepath.Clean(p), string(filepath.Separator)) - rootDepth; depth > maxScanDepth {
			return fs.SkipDir
		}
		if p != root && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if strings.EqualFold(d.Name(), "Paks") && strings.EqualFold(filepath.Base(filepath.Dir(p)), "Content") {
			if len(pakFilesIn(p)) > 0 {
				out = append(out, p)
			}
			return fs.SkipDir
		}
		return nil
	})
	sort.Strings(out)
	return out
}

func describePak(path string, oodleAvailable bool) pakInfo {
	info := pakInfo{Path: path, Name: filepath.Base(path), ConfigFiles: []string{}, CompressionMethods: []string{}, OodleAvailable: oodleAvailable}
	f, err := os.Open(path)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	st, err := f.Stat()
	if err == nil {
		info.Size = st.Size()
		var ft *pak.Footer
		if ft, err = pak.ReadFooter(f, st.Size()); err == nil {
			info.Version = ft.Version
			for _, n := range ft.CompressionNames {
				if n != "" {
					info.CompressionMethods = append(info.CompressionMethods, n)
				}
			}
		}
	}
	f.Close()
	if err != nil {
		info.Error = err.Error()
		return info
	}
	p, err := pak.Open(path)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Supported = true
	info.Entries = len(p.Entries)
	info.ConfigFiles = configFiles(p)
	for _, name := range info.ConfigFiles {
		if e, ok := p.Entry(name); ok && strings.EqualFold(p.MethodName(e), pak.MethodOodle) {
			info.OodleRequired = true
		}
	}
	return info
}

// configFiles lists */Config/*.ini entries: DefaultEngine.ini and
// DefaultGame.ini first, then shallower paths, then alphabetical.
func configFiles(p *pak.Pak) []string {
	out := []string{}
	for _, e := range p.Entries {
		lower := strings.ToLower(e.Name)
		if strings.HasSuffix(lower, ".ini") && (strings.Contains(lower, "/config/") || strings.HasPrefix(lower, "config/")) {
			out = append(out, e.Name)
		}
	}
	rank := func(n string) int {
		switch strings.ToLower(n[strings.LastIndex(n, "/")+1:]) {
		case "defaultengine.ini":
			return 0
		case "defaultgame.ini":
			return 1
		}
		return 2
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
			return da < db
		}
		return a < b
	})
	return out
}
