package ares

// Solara: dashboard editor for the anonymous-mode name generator (a2station_anonymous.go). Master key only:
// like /api/titledata these routes are deliberately absent from dashScopedRoutes, so scoped tokens are refused.
//
//	GET    /api/anonymous-names           -> {settings, defaults, is_default, updated_at, preview:[{seed,name}]}
//	PUT    /api/anonymous-names           body {adjectives, nouns, number_min, number_max} -> same as GET
//	DELETE /api/anonymous-names           -> back to the built-in defaults (same as GET)
//	POST   /api/anonymous-names/preview   body as PUT -> {preview:[{seed,name}]}, nothing saved
//
// A change applies to the next name a player gets (each time they turn anonymous mode on).

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"solara/internal/hosting"
	"solara/internal/stj"
)

const (
	anonNameMaxWords   = 1000
	anonNameMaxWordLen = 24
	anonNameMaxNumber  = 99999
)

var anonNameWordRe = regexp.MustCompile(`^[A-Za-z0-9]+$`)

// anonNamePreviewSeeds are fixed so a preview is stable between reloads.
var anonNamePreviewSeeds = []int64{481488160, 352739360, 546024512, 1799785600, 1923566336, 1666784512, 7, 42, 1000, 31337}

// anonNameValidate trims and de-duplicates the lists and checks every limit.
func anonNameValidate(body []byte) (AnonNameSettings, string) {
	var in struct {
		Adjectives []string `json:"adjectives"`
		Nouns      []string `json:"nouns"`
		NumberMin  *int     `json:"number_min"`
		NumberMax  *int     `json:"number_max"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return AnonNameSettings{}, "body must be JSON {adjectives:[...], nouns:[...], number_min, number_max}"
	}
	clean := func(label string, words []string) ([]string, string) {
		out, seen := []string{}, map[string]bool{}
		for _, w := range words {
			w = strings.TrimSpace(w)
			if w == "" {
				continue
			}
			if len(w) > anonNameMaxWordLen || !anonNameWordRe.MatchString(w) {
				return nil, fmt.Sprintf("%s: %q must be 1-%d letters or digits (no spaces)", label, w, anonNameMaxWordLen)
			}
			if k := strings.ToLower(w); !seen[k] {
				seen[k] = true
				out = append(out, w)
			}
		}
		if len(out) == 0 {
			return nil, label + ": add at least one word"
		}
		if len(out) > anonNameMaxWords {
			return nil, fmt.Sprintf("%s: at most %d words", label, anonNameMaxWords)
		}
		return out, ""
	}
	adj, e := clean("adjectives", in.Adjectives)
	if e != "" {
		return AnonNameSettings{}, e
	}
	nouns, e := clean("nouns", in.Nouns)
	if e != "" {
		return AnonNameSettings{}, e
	}
	if in.NumberMin == nil || in.NumberMax == nil {
		return AnonNameSettings{}, "number_min and number_max are required (0 and 0 = no number)"
	}
	lo, hi := *in.NumberMin, *in.NumberMax
	if lo < 0 || hi < 0 || lo > anonNameMaxNumber || hi > anonNameMaxNumber || lo > hi {
		return AnonNameSettings{}, fmt.Sprintf("number range must satisfy 0 <= number_min <= number_max <= %d", anonNameMaxNumber)
	}
	return AnonNameSettings{Adjectives: adj, Nouns: nouns, NumberMin: lo, NumberMax: hi}, ""
}

func anonNamePreview(s AnonNameSettings) []any {
	out := []any{}
	for _, seed := range anonNamePreviewSeeds {
		out = append(out, stj.O("seed", seed, "name", AnonymousNameWith(s, seed)))
	}
	return out
}

func anonNameState() hosting.Result {
	cur := CurrentAnonNameSettings()
	updated := ""
	doc := savedAnonNameConfig()
	if doc != nil {
		updated = doc.UpdatedAt
	}
	return dashJson(200, stj.O("settings", cur, "defaults", AnonNameDefaults(), "is_default", doc == nil,
		"updated_at", updated, "preview", anonNamePreview(cur)))
}

// GetAnonymousNamesAdmin is GET /api/anonymous-names.
func (s *AresDashboardServer) GetAnonymousNamesAdmin(c *hosting.Context) hosting.Result {
	return anonNameState()
}

// PutAnonymousNamesAdmin is PUT /api/anonymous-names.
func (s *AresDashboardServer) PutAnonymousNamesAdmin(c *hosting.Context) hosting.Result {
	settings, e := anonNameValidate(c.Request.Body())
	if e != "" {
		return dashJson(400, stj.O("error", e))
	}
	SaveAnonNameSettings(&settings, time.Now().UTC().Format(time.RFC3339))
	return anonNameState()
}

// DeleteAnonymousNamesAdmin is DELETE /api/anonymous-names (reset to the defaults).
func (s *AresDashboardServer) DeleteAnonymousNamesAdmin(c *hosting.Context) hosting.Result {
	SaveAnonNameSettings(nil, "")
	return anonNameState()
}

// PreviewAnonymousNamesAdmin is POST /api/anonymous-names/preview (nothing is saved).
func (s *AresDashboardServer) PreviewAnonymousNamesAdmin(c *hosting.Context) hosting.Result {
	settings, e := anonNameValidate(c.Request.Body())
	if e != "" {
		return dashJson(400, stj.O("error", e))
	}
	return dashJson(200, stj.O("preview", anonNamePreview(settings)))
}
