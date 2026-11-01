package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildCatalog(t *testing.T) {
	files := map[string][]byte{
		"config.json": []byte("\xef\xbb\xbf" + `{"config":{"player":{"brakeStrength":{"value":600,"minValue":0,"maxValue":1000},"pushOffAlwaysAdds":false},
			"defaultUsername":"Player","featureFlags":{},"list":[1,2]},"luaObject":{}}`),
		"betaTest.json":      []byte(`{"config":{"player":{"smallThrusterBeginStrength":{"value":4000,"minValue":0,"maxValue":1000},"smallThrusterEndStrength":1000}}}`),
		"gamemodes.json":     []byte(`{"value":[]}`),
		"config_backup.json": []byte(`{}`),
	}
	levels := t.TempDir()
	dir := filepath.Join(levels, "Scraprun", "Scraprun3_a2level")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	level := `{"value":{"moduleState":{"Config":{"OptimalRunnerCount":10,"bUseTrapBundle":true},
		"DefaultConfig":{"OptimalRunnerCount":5,"TicketManagerSettings":{"MaxTeamSizes":{"0":100}}}}}}`
	if err := os.WriteFile(filepath.Join(dir, "level.json"), []byte(level), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := buildCatalog(files, levels, "test", []string{"*", "scraprun3"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]worldVar{}
	for _, w := range cat.World {
		got[w.Path] = w
	}
	if w := got["config/player/brakeStrength"]; w.Type != "number" || w.Min == nil || w.Min.String() != "0" || w.Max.String() != "1000" {
		t.Fatalf("ranged leaf = %+v", w)
	}
	if got["config/player/pushOffAlwaysAdds"].Type != "bool" || got["config/defaultUsername"].Type != "string" {
		t.Fatalf("plain leaves = %+v", cat.World)
	}
	// json-typed and non-config leaves can't be written by the DLL's world writer
	for _, p := range []string{"config/featureFlags", "config/list", "luaObject"} {
		if _, ok := got[p]; ok {
			t.Fatalf("%s should be skipped", p)
		}
	}
	if len(cat.Presets) != 1 || len(cat.Presets["betaTest"]) != 2 || cat.Presets["betaTest"][0].Path != "config/player/smallThrusterBeginStrength" {
		t.Fatalf("presets = %+v", cat.Presets)
	}
	if len(cat.Modules) != 1 || cat.Modules[0].Level != "Scraprun/Scraprun3_a2level" {
		t.Fatalf("modules = %+v", cat.Modules)
	}
	vars := map[string]moduleVar{}
	for _, v := range cat.Modules[0].Vars {
		vars[v.Name] = v
	}
	// Config wins over DefaultConfig; DefaultConfig-only vars are kept
	if vars["OptimalRunnerCount"].Default.(interface{ String() string }).String() != "10" || vars["TicketManagerSettings"].Type != "json" || vars["bUseTrapBundle"].Type != "bool" {
		t.Fatalf("module vars = %+v", vars)
	}
	if len(cat.LiveSlots) != 2 {
		t.Fatalf("live slots = %v", cat.LiveSlots)
	}
}

func TestReadSlots(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log.txt")
	log := "[HalcyonA2][SLOTMAP] slot TKB_Prime at (0, 0, 0) moduleState=1\r\n" +
		"noise\n[HalcyonA2][SLOTMAP] slot scraprun3 at (1, 2, 3)\n[HalcyonA2][SLOTMAP] slot TKB_Prime at (0, 0, 0)\n"
	if err := os.WriteFile(p, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := readSlots(p)
	if err != nil || len(s) != 3 || s[0] != "*" || s[1] != "TKB_Prime" || s[2] != "scraprun3" {
		t.Fatalf("slots = %v %v", s, err)
	}
}
