package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"solara/internal/pak"
)

// extractNetVars pulls every A2/Content/NetVars/*.json out of the game's UFS pak (Oodle-compressed
// entries are decoded with the game's own oodle DLL) and returns name -> bytes.
func extractNetVars(pakPath, oodleDll string) (map[string][]byte, error) {
	p, err := pak.Open(pakPath)
	if err != nil {
		return nil, err
	}
	if dll, _ := pak.FindOodleDLL(oodleDll); dll != "" {
		o, err := pak.LoadOodle(dll)
		if err != nil {
			return nil, fmt.Errorf("oodle %s: %w", dll, err)
		}
		p.Decompressor = o
	}
	out := map[string][]byte{}
	for _, e := range p.Entries {
		full := e.Name
		if !strings.Contains(full, "/") {
			continue
		}
		mounted := strings.TrimPrefix(p.MountPoint+full, "../../../")
		if !strings.HasPrefix(mounted, "A2/Content/NetVars/") || !strings.HasSuffix(mounted, ".json") {
			continue
		}
		b, err := p.ReadEntry(e.Name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", mounted, err)
		}
		out[filepath.Base(mounted)] = b
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no A2/Content/NetVars/*.json in %s (mount %q)", pakPath, p.MountPoint)
	}
	return out, nil
}

func writeFiles(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
