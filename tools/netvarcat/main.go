// netvarcat rebuilds the dashboard's netvar catalog (web/dashboard/public/netvar-catalog.json) from a
// game build: world netvars + presets from A2/Content/NetVars/*.json in the UFS pak, module (game-mode)
// netvars from LevelDefinitions level.json exports.
//
//	go run ./tools/netvarcat -pak <A2-Windows.pak> -oodle <oodle dll> -levels <LevelDefinitions export dir> \
//	    -build 50451 -out web/dashboard/public/netvar-catalog.json [-slots HalcyonA2.log] [-dump <dir>]
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

func main() {
	pakPath := flag.String("pak", "", "game UFS pak (A2-Windows.pak)")
	oodle := flag.String("oodle", "", "oodle DLL for compressed entries")
	levels := flag.String("levels", "", "LevelDefinitions export directory (FModel JSON)")
	build := flag.String("build", "", "build label recorded in generated_from")
	out := flag.String("out", "", "catalog output path")
	dump := flag.String("dump", "", "optional: also write the extracted NetVars json files here")
	slotsFile := flag.String("slots", "", "optional: HalcyonA2.log (or a text file) with [SLOTMAP] lines from a running server; their slot ids become the slot picker's suggestions")
	flag.Parse()
	if *pakPath == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}
	files, err := extractNetVars(*pakPath, *oodle)
	if err != nil {
		fmt.Fprintln(os.Stderr, "extract:", err)
		os.Exit(1)
	}
	if *dump != "" {
		if err := writeFiles(*dump, files); err != nil {
			fmt.Fprintln(os.Stderr, "dump:", err)
			os.Exit(1)
		}
	}
	slots, err := readSlots(*slotsFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "slots:", err)
		os.Exit(1)
	}
	cat, err := buildCatalog(files, *levels, *build, slots)
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog:", err)
		os.Exit(1)
	}
	if err := writeCatalog(*out, cat); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("catalog: %d world vars, %d presets, %d modules -> %s\n", len(cat.World), len(cat.Presets), len(cat.Modules), *out)
}

// readSlots collects the slot ids from "[SLOTMAP] slot <id> at ..." lines, "*" first, then sorted.
func readSlots(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, line := range strings.Split(string(b), "\n") {
		i := strings.Index(line, "[SLOTMAP] slot ")
		if i < 0 {
			continue
		}
		f := strings.Fields(line[i+len("[SLOTMAP] slot "):])
		if len(f) > 0 && !seen[f[0]] {
			seen[f[0]] = true
			out = append(out, f[0])
		}
	}
	sort.Strings(out)
	return append([]string{"*"}, out...), nil
}
