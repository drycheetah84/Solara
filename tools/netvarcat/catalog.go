package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Catalog shape read by web/dashboard/app/stations/[id]/NetvarsTab.tsx.
type worldVar struct {
	Path    string       `json:"path"`
	Type    string       `json:"type"`
	Default any          `json:"default"`
	Min     *json.Number `json:"min,omitempty"`
	Max     *json.Number `json:"max,omitempty"`
}

type presetEntry struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

type moduleVar struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default any    `json:"default"`
}

type module struct {
	Level   string      `json:"level"`
	SlotKey *string     `json:"slot_key"`
	Vars    []moduleVar `json:"vars"`
}

type catalog struct {
	GeneratedFrom string                   `json:"generated_from"`
	World         []worldVar               `json:"world"`
	Presets       map[string][]presetEntry `json:"presets"`
	Modules       []module                 `json:"modules"`
	StationKeys   []string                 `json:"station_keys"`
	LiveSlots     []string                 `json:"live_slots,omitempty"`
}

// Station config keys the game applies into config/stationConfig (board / sign images, announcement).
// They come from the station /config map, not from the netvar files; kept as in the previous catalog.
var stationKeys = []string{
	"StationAnnouncement",
	"BoardTextureUrl1", "BoardTextureUrl2", "BoardTextureUrl3", "BoardTextureUrl4", "BoardTextureUrl5", "BoardTextureUrl6",
	"SignPlazaFront", "SignPlazaWest", "SignPlazaEast", "SignComplexWest", "SignComplexEast", "SignComplexBasement",
	"SignArenaPrime", "statusBoardTKBMainUrl", "statusBoardTKBSidesUrl", "statusBoardTKBBack1Url",
	"statusBoardTKBBack2Url", "kingsCourtTeamLogosUrl",
}

// NetVars files that are not presets: the base config, scratch copies, and the gamemode list.
var notPresets = map[string]bool{"config.json": true, "config_backup.json": true, "config_test2.json": true, "gamemodes.json": true}

func decode(b []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// ranged reports whether v is a {value, minValue, maxValue} leaf (a slider in the game's NetVar UI).
func ranged(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	if _, ok := m["value"]; !ok {
		return nil, false
	}
	for k := range m {
		if k != "value" && k != "minValue" && k != "maxValue" {
			return nil, false
		}
	}
	return m, true
}

func typeOf(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case json.Number:
		return "number"
	case string:
		return "string"
	default:
		return "json"
	}
}

func num(v any) *json.Number {
	if n, ok := v.(json.Number); ok {
		return &n
	}
	return nil
}

// walkWorld flattens config.json into path leaves. A ranged leaf is one number variable with min/max;
// arrays and empty objects are single json variables; other objects are recursed.
func walkWorld(v any, path string, out *[]worldVar) {
	if r, ok := ranged(v); ok {
		*out = append(*out, worldVar{Path: path, Type: typeOf(r["value"]), Default: r["value"], Min: num(r["minValue"]), Max: num(r["maxValue"])})
		return
	}
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkWorld(m[k], path+"/"+k, out)
		}
		return
	}
	*out = append(*out, worldVar{Path: path, Type: typeOf(v), Default: v})
}

// walkPreset flattens a preset file the same way; a ranged leaf contributes its value.
func walkPreset(v any, path string, out *[]presetEntry) {
	if r, ok := ranged(v); ok {
		*out = append(*out, presetEntry{Path: path, Value: r["value"]})
		return
	}
	if m, ok := v.(map[string]any); ok && len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkPreset(m[k], path+"/"+k, out)
		}
		return
	}
	*out = append(*out, presetEntry{Path: path, Value: v})
}

func trimPath(p string) string { return strings.TrimPrefix(p, "/") }

// levelModules reads every LevelDefinitions/**/level.json (FModel export) whose moduleState carries a
// config: variables = DefaultConfig overlaid with Config (Config wins), as the game merges them.
func levelModules(root string) ([]module, error) {
	var mods []module
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "level.json" {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		v, err := decode(b)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		top, _ := v.(map[string]any)
		val, _ := top["value"].(map[string]any)
		ms, _ := val["moduleState"].(map[string]any)
		if ms == nil {
			return nil
		}
		merged := map[string]any{}
		for _, k := range []string{"DefaultConfig", "Config"} {
			if c, ok := ms[k].(map[string]any); ok {
				for n, x := range c {
					merged[n] = x
				}
			}
		}
		if len(merged) == 0 {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(p))
		m := module{Level: filepath.ToSlash(rel)}
		names := make([]string, 0, len(merged))
		for n := range merged {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			m.Vars = append(m.Vars, moduleVar{Name: n, Type: typeOf(merged[n]), Default: merged[n]})
		}
		mods = append(mods, m)
		return nil
	})
	sort.Slice(mods, func(i, j int) bool { return strings.ToLower(mods[i].Level) < strings.ToLower(mods[j].Level) })
	return mods, err
}

func buildCatalog(files map[string][]byte, levels, build string, liveSlots []string) (*catalog, error) {
	base, ok := files["config.json"]
	if !ok {
		return nil, fmt.Errorf("config.json missing from the NetVars files")
	}
	v, err := decode(base)
	if err != nil {
		return nil, fmt.Errorf("config.json: %w", err)
	}
	cat := &catalog{
		GeneratedFrom: fmt.Sprintf("build %s: A2-Windows.pak Content/NetVars/*.json (world vars + presets) + LevelDefinitions/**/level.json moduleState DefaultConfig merged with Config (gamemode vars); live slots from the running server's SLOTMAP log. Generated by tools/netvarcat.", build),
		Presets:       map[string][]presetEntry{},
		StationKeys:   stationKeys,
		LiveSlots:     liveSlots,
	}
	var all []worldVar
	walkWorld(v, "", &all)
	for _, w := range all {
		w.Path = trimPath(w.Path)
		// The DLL's world writer only takes bool / number / string leaves (rc -2 otherwise), and the
		// dashboard groups world vars by their config/<section>; skip anything that could never apply.
		if w.Type == "json" || !strings.HasPrefix(w.Path, "config/") {
			continue
		}
		cat.World = append(cat.World, w)
	}
	for name, b := range files {
		if notPresets[name] {
			continue
		}
		pv, err := decode(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		var es []presetEntry
		walkPreset(pv, "", &es)
		for i := range es {
			es[i].Path = trimPath(es[i].Path)
		}
		cat.Presets[strings.TrimSuffix(name, ".json")] = es
	}
	if levels != "" {
		if cat.Modules, err = levelModules(levels); err != nil {
			return nil, err
		}
	}
	if cat.Modules == nil {
		cat.Modules = []module{}
	}
	return cat, nil
}

func writeCatalog(path string, cat *catalog) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(cat); err != nil {
		return err
	}
	return os.WriteFile(path, bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}
