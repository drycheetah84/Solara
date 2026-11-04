package ares

import (
	"encoding/json"
	"strings"
	"testing"
)

// resetAnonNameCache drops the generator's cached settings (they follow the package-level Database).
func resetAnonNameCache(t *testing.T) {
	t.Helper()
	anonNameMu.Lock()
	anonNameCached = nil
	anonNameMu.Unlock()
	t.Cleanup(func() { anonNameMu.Lock(); anonNameCached = nil; anonNameMu.Unlock() })
}

func TestAnonymousNamesAdminEditor(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	resetAnonNameCache(t)
	s := newAresDashboardServer()

	type state struct {
		Settings  AnonNameSettings `json:"settings"`
		IsDefault bool             `json:"is_default"`
		Preview   []struct {
			Seed int64  `json:"seed"`
			Name string `json:"name"`
		} `json:"preview"`
	}
	read := func(code int, body []byte) state {
		t.Helper()
		var st state
		if err := json.Unmarshal(body, &st); err != nil || code != 200 {
			t.Fatalf("state: %d %v %s", code, err, body)
		}
		return st
	}

	// defaults
	w := do(t, s, "GET", "/api/anonymous-names", "", adminHdr)
	st := read(w.Code, w.Body.Bytes())
	if !st.IsDefault || len(st.Settings.Adjectives) != len(anonNameAdjectives) || st.Settings.NumberMax != 99 || len(st.Preview) != len(anonNamePreviewSeeds) {
		t.Fatalf("default state: %s", w.Body.String())
	}

	// invalid input is refused with a reason
	for _, bad := range []string{
		`{"adjectives":[],"nouns":["Fox"],"number_min":1,"number_max":9}`,
		`{"adjectives":["Two Words"],"nouns":["Fox"],"number_min":1,"number_max":9}`,
		`{"adjectives":["Red"],"nouns":["Fox"],"number_min":9,"number_max":1}`,
		`{"adjectives":["Red"],"nouns":["Fox"]}`,
		`not json`,
	} {
		if w := do(t, s, "PUT", "/api/anonymous-names", bad, adminHdr); w.Code != 400 || !strings.Contains(w.Body.String(), "error") {
			t.Fatalf("PUT %s: want 400 with an error, got %d %s", bad, w.Code, w.Body.String())
		}
	}

	// preview does not save
	w = do(t, s, "POST", "/api/anonymous-names/preview", `{"adjectives":["Red"],"nouns":["Fox"],"number_min":0,"number_max":0}`, adminHdr)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"RedFox"`) {
		t.Fatalf("preview: %d %s", w.Code, w.Body.String())
	}
	if st := read(do(t, s, "GET", "/api/anonymous-names", "", adminHdr).Code, do(t, s, "GET", "/api/anonymous-names", "", adminHdr).Body.Bytes()); !st.IsDefault {
		t.Fatal("preview must not save")
	}

	// save: duplicates (any case) and blanks dropped; the game route uses the new lists at once
	w = do(t, s, "PUT", "/api/anonymous-names", `{"adjectives":[" Red ","red","Blue",""],"nouns":["Fox"],"number_min":5,"number_max":5}`, adminHdr)
	st = read(w.Code, w.Body.Bytes())
	if st.IsDefault || strings.Join(st.Settings.Adjectives, ",") != "Red,Blue" || st.Settings.NumberMin != 5 {
		t.Fatalf("saved state: %s", w.Body.String())
	}
	for _, seed := range anonNamePreviewSeeds {
		if n := AnonymousNameForSeed(seed); n != "RedFox5" && n != "BlueFox5" {
			t.Fatalf("seed %d after save: %q", seed, n)
		}
	}

	// a scoped token with every permission on every fleet is refused (master key only, like title data)
	all := []string{}
	for _, p := range dashScopedPerms {
		all = append(all, `"`+p.Name+`"`)
	}
	w = do(t, s, "POST", "/api/scoped-tokens", `{"name":"everything","permissions":[`+strings.Join(all, ",")+`],"stations":["*"]}`, adminHdr)
	var tok map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &tok)
	th := map[string]string{"x-dashboard-key": tok["token"].(string)}
	for _, r := range [][2]string{{"GET", "/api/anonymous-names"}, {"PUT", "/api/anonymous-names"}, {"DELETE", "/api/anonymous-names"}, {"POST", "/api/anonymous-names/preview"}} {
		if w := do(t, s, r[0], r[1], `{"adjectives":["X"],"nouns":["Y"],"number_min":0,"number_max":0}`, th); w.Code != 403 {
			t.Fatalf("scoped %s %s must be refused: %d", r[0], r[1], w.Code)
		}
	}

	// reset
	w = do(t, s, "DELETE", "/api/anonymous-names", "", adminHdr)
	if st := read(w.Code, w.Body.Bytes()); !st.IsDefault || len(st.Settings.Nouns) != len(anonNameNouns) {
		t.Fatalf("reset: %s", w.Body.String())
	}
}
