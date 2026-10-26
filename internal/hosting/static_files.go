// Ports Http/Preprocessors/Default/HttpStaticFilesProcessor.cs and
// AstraHttpServerExtensions.AddStaticFiles. No original server registers it
// (verified); ported for framework parity.
package hosting

import (
	"os"
	"path/filepath"
	"strings"
)

// StaticFilesProcessor mirrors HttpStaticFilesProcessor.
type StaticFilesProcessor struct {
	BaseFolder                string
	AllowExtensionlessLookups bool
}

// AddStaticFiles mirrors AstraHttpServerExtensions.AddStaticFiles.
func (s *Server) AddStaticFiles(baseFolder string, allowExtensionless bool) {
	s.AddPreprocessor(&StaticFilesProcessor{BaseFolder: baseFolder, AllowExtensionlessLookups: allowExtensionless})
}

func (p *StaticFilesProcessor) filePath(uri string) string {
	cwd, _ := os.Getwd()
	parts := []string{cwd, p.BaseFolder}
	for _, seg := range strings.Split(uri, "/") {
		if seg != "" {
			parts = append(parts, seg)
		}
	}
	return filepath.Join(parts...)
}

// TryPreprocessRequest mirrors the original lookup order: exact file, a
// directory's index.* file, then (optionally) a same-named file with any
// extension; otherwise NotFound with FAIL (the pipeline continues).
func (p *StaticFilesProcessor) TryPreprocessRequest(req *Request, resp *Response) (Result, PreprocessorResult) {
	path := p.filePath(req.Uri())
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return p.success(path)
	}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		if idx := findByStem(path, "index"); idx != "" {
			return p.success(idx)
		}
	}
	if p.AllowExtensionlessLookups {
		stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if f := findByStem(filepath.Dir(path), stem); f != "" {
			return p.success(f)
		}
	}
	return NotFound(nil), PreFail
}

// findByStem returns the first file in dir (Directory.GetFiles order: by
// name) whose name without extension equals stem, case-insensitively.
func findByStem(dir, stem string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.EqualFold(strings.TrimSuffix(n, filepath.Ext(n)), stem) {
			return filepath.Join(dir, n)
		}
	}
	return ""
}

func (p *StaticFilesProcessor) success(path string) (Result, PreprocessorResult) {
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return Configurable(200, MimeType(strings.TrimPrefix(filepath.Ext(path), ".")), raw), PreOK | PreStopAfter
}

// MimeType mirrors MimeTypes.MimeTypeMap.GetMimeType (MimeTypeMapOfficial):
// accepts an extension with or without a dot, or a file name; unknown →
// application/octet-stream. The table covers the web-relevant subset of the
// package's map (see KNOWN_ISSUES.md HOST-MIME-001).
func MimeType(s string) string {
	if !strings.HasPrefix(s, ".") {
		if i := strings.LastIndex(s, "."); i != -1 && len(s) > i+1 {
			s = s[i+1:]
		}
		s = "." + s
	}
	if t, ok := mimeTypes[strings.ToLower(s)]; ok {
		return t
	}
	return "application/octet-stream"
}

var mimeTypes = map[string]string{
	".htm": "text/html", ".html": "text/html", ".css": "text/css", ".js": "application/javascript",
	".mjs": "application/javascript", ".json": "application/json", ".map": "application/json",
	".txt": "text/plain", ".csv": "text/csv", ".xml": "text/xml", ".svg": "image/svg+xml",
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".ico": "image/x-icon", ".webp": "image/webp", ".bmp": "image/bmp", ".tif": "image/tiff",
	".tiff": "image/tiff", ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "application/x-font-ttf",
	".otf": "font/otf", ".eot": "application/vnd.ms-fontobject", ".wasm": "application/wasm",
	".pdf": "application/pdf", ".zip": "application/x-zip-compressed", ".gz": "application/x-gzip",
	".tar": "application/x-tar", ".7z": "application/x-7z-compressed", ".rar": "application/x-rar-compressed",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "video/ogg", ".mp4": "video/mp4",
	".webm": "video/webm", ".avi": "video/x-msvideo", ".mov": "video/quicktime",
	".exe": "application/octet-stream", ".dll": "application/x-msdownload", ".bin": "application/octet-stream",
	".apk": "application/vnd.android.package-archive", ".md": "text/markdown", ".yaml": "text/yaml",
	".ps1": "application/octet-stream", ".sh": "application/x-sh",
}
