package ares

import (
	"testing"
	"time"
)

func TestDistrictPopsRecordAndLookup(t *testing.T) {
	districtPopsMu.Lock()
	districtPops = map[string]*districtPopsEntry{}
	districtPopsMu.Unlock()

	recordDistrictPops("DepA", []byte(`{"deployment_id":"DepA","player_count":5,"district_populations":{"Driftball":3,"club":2,"space":0,"bad":"x","neg":-1}}`))
	id := "depa"
	got := districtPopsFor(&id, nil)
	if len(got) != 2 || got["driftball"] != 3 || got["club"] != 2 {
		t.Fatalf("got %v", got)
	}

	// A report without the map leaves the previous one in place (older DLLs).
	recordDistrictPops("DepA", []byte(`{"deployment_id":"DepA","player_count":5}`))
	if got := districtPopsFor(&id, nil); got["driftball"] != 3 {
		t.Fatalf("map was dropped by a report without it: %v", got)
	}

	// Lookup by ip (EOS-session row of the same server), and staleness.
	districtPopsMu.Lock()
	districtPops["depa"].ip = "1.2.3.4:7777"
	districtPopsMu.Unlock()
	other, ip := "eos-session", "1.2.3.4:7777"
	if got := districtPopsFor(&other, &ip); got["club"] != 2 {
		t.Fatalf("ip fallback: %v", got)
	}
	districtPopsMu.Lock()
	districtPops["depa"].at = time.Now().Add(-districtPopsTTL - time.Second)
	districtPopsMu.Unlock()
	if got := districtPopsFor(&id, &ip); got == nil || len(got) != 0 {
		t.Fatalf("stale entry should be empty (non-nil): %v", got)
	}
	if got := districtPopsFor(nil, nil); got == nil {
		t.Fatal("must never be nil")
	}
}
