package main

import (
	"embed"
	"io/fs"
)

// The React UI (web/pak-editor) builds into ./ui (vite build.outDir =
// ../../cmd/pak-editor/ui). A placeholder index.html keeps the Go build
// working before the frontend has been built.
//
//go:embed all:ui
var uiFiles embed.FS

func uiFS() fs.FS {
	sub, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		panic(err)
	}
	return sub
}
