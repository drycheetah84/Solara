// Serves the static.aa-mothership.com objects the client fetches at boot
// (midnight redirects *aa-mothership* to this server). Bodies are the exact
// bytes captured from the live CDN on 2026-10-31; content types, ETags and
// Last-Modified match the original responses.
package mothership

import (
	"embed"

	"solara/internal/hosting"
)

//go:embed static/index.json static/TOS0302.txt static/PrivacyPolicy0302.txt
var staticFS embed.FS

type staticObject struct {
	file, contentType, etag, lastModified string
}

var staticObjects = map[string]staticObject{
	"/index.json":            {"static/index.json", "application/octet-stream", `"6b5869b94554281df90cb3b7a6fb0563"`, "Mon, 26 Oct 2026 17:39:25 GMT"},
	"/TOS0302.txt":           {"static/TOS0302.txt", "text/plain", `"f5356a64519732def5f862edb90b3da5"`, "Mon, 02 Mar 2026 21:33:24 GMT"},
	"/PrivacyPolicy0302.txt": {"static/PrivacyPolicy0302.txt", "text/plain", `"1940c5a850a7d89f69d6a9cb50ab4f16"`, "Mon, 02 Mar 2026 21:33:24 GMT"},
}

func staticHandler(path string) hosting.Handler {
	obj := staticObjects[path]
	return func(c *hosting.Context) hosting.Result {
		body, err := staticFS.ReadFile(obj.file)
		if err != nil {
			return hosting.NotFound(nil)
		}
		c.Response.SetHeader("ETag", obj.etag)
		c.Response.SetHeader("Last-Modified", obj.lastModified)
		c.Response.SetHeader("Accept-Ranges", "bytes")
		return hosting.Configurable(200, obj.contentType, body)
	}
}

func registerStatic(h *hosting.Server) {
	for _, p := range []string{"/index.json", "/TOS0302.txt", "/PrivacyPolicy0302.txt"} {
		h.Handle("GET", p, "Static"+p, staticHandler(p))
	}
}
