package ares

import (
	"regexp"
	"testing"
)

func TestA2AnonymousName(t *testing.T) {
	s, _ := a2TestEnv(t)
	resetAnonNameCache(t) // the built-in defaults, whatever another test saved
	shape := regexp.MustCompile(`^[A-Z][a-z]+[A-Z][a-z]+[1-9][0-9]$`)
	get := func(target string) string {
		t.Helper()
		w := a2Do(t, s, "GET", target, "", nil) // open route: the game server sends no key
		if w.Code != 200 {
			t.Fatalf("%s: HTTP %d %s", target, w.Code, w.Body.String())
		}
		v, ok := a2Json(t, w).TryGetProperty("name")
		if !ok || v.GetString() == nil {
			t.Fatalf("%s: no name in %s", target, w.Body.String())
		}
		return *v.GetString()
	}

	a := get("/v1/anonymous_name?seed=481488160")
	if !shape.MatchString(a) {
		t.Fatalf("unexpected name shape %q", a)
	}
	if b := get("/v1/anonymous_name?seed=481488160"); b != a {
		t.Fatalf("same seed gave %q then %q", a, b)
	}
	// seeds the game actually sent (local server log 2026-11-04) should not all collapse to one name
	names := map[string]bool{a: true}
	for _, seed := range []string{"352739360", "546024512", "1799785600", "1923566336", "1666784512"} {
		names[get("/v1/anonymous_name?seed="+seed)] = true
	}
	if len(names) < 5 {
		t.Fatalf("too few distinct names for 6 seeds: %v", names)
	}
	// missing / malformed / negative seeds still answer
	for _, target := range []string{"/v1/anonymous_name", "/v1/anonymous_name?seed=abc", "/v1/anonymous_name?seed=-5"} {
		if n := get(target); !shape.MatchString(n) {
			t.Fatalf("%s: unexpected name %q", target, n)
		}
	}
}
