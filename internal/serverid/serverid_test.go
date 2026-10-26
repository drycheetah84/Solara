package serverid

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"solara/internal/docdb"
	"solara/internal/logging"
)

func init() { logging.DisableFiles = true }

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func testStore(t *testing.T) (*Store, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sid"+docdb.FileSuffix)
	db, err := docdb.Open(&docdb.Descriptor{DatabaseId: 9, Name: "sid", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	clk := &clock{t: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)}
	s := NewStore(db)
	s.Now = clk.now
	s.SetDatabase(db)
	return s, clk, path
}

var tokenRe = regexp.MustCompile(`^sst1_[A-Za-z0-9_-]{43}$`)

func TestIssueValidateAndHashOnlyStorage(t *testing.T) {
	s, _, path := testStore(t)
	tok, rec, err := s.IssueSpec(Spec{Kind: KindServer, DeploymentId: "depA", StationId: "st1", Box: "gb1"})
	if err != nil {
		t.Fatal(err)
	}
	if !tokenRe.MatchString(tok) || !LooksLikeToken(tok) {
		t.Fatalf("token shape %q", tok)
	}
	if rec.Prefix != tok[:11] || rec.TokenHash != Hash(tok) || rec.Deployment() != "depA" || rec.Station() != "st1" || rec.BoxName() != "gb1" {
		t.Fatalf("record %+v", rec)
	}
	got, ok := s.Validate(tok)
	if !ok || got.Deployment() != "depA" || got.Kind != KindServer {
		t.Fatalf("validate: %v %+v", ok, got)
	}
	for _, bad := range []string{"", "sst1_", tok + "x", tok[:len(tok)-1] + "A", "Bearer " + tok, strings.ToUpper(tok)} {
		if bad == tok {
			continue
		}
		if _, ok := s.Validate(bad); ok {
			t.Errorf("accepted %q", bad)
		}
	}
	// the store must hold only the hash: the secret part never reaches disk
	// (the dev database is unencrypted, so a plain byte search is meaningful)
	s.SetDatabase(nil)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(tok[len(TokenPrefix):])) || bytes.Contains(raw, []byte(tok[11:30])) {
		t.Fatal("token secret stored in the database")
	}
	if !bytes.Contains(raw, []byte(Hash(tok))) {
		t.Fatal("hash not stored (is the test database encrypted?)")
	}
}

func TestIssueValidation(t *testing.T) {
	s, _, _ := testStore(t)
	if _, _, err := s.Issue("admin", "d", "b", TTLs{}); err != ErrBadKind {
		t.Fatal(err)
	}
	if _, _, err := s.Issue(KindServer, " ", "b", TTLs{}); err == nil {
		t.Fatal("server token without deployment")
	}
	if _, _, err := s.Issue(KindAgent, "", "", TTLs{}); err == nil {
		t.Fatal("agent token without box")
	}
	if _, _, err := NewStore(nil).Issue(KindAgent, "", "b", TTLs{}); err != ErrNoDatabase {
		t.Fatal(err)
	}
	if _, ok := NewStore(nil).Validate("sst1_" + strings.Repeat("A", 43)); ok {
		t.Fatal("no-db store validated")
	}
}

func TestIdleExpirySlides(t *testing.T) {
	s, clk, _ := testStore(t)
	tok, _, err := s.Issue(KindServer, "dep", "", TTLs{Idle: time.Hour, Max: 10 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	// used every 50 minutes: the idle window keeps sliding
	for i := 0; i < 5; i++ {
		clk.add(50 * time.Minute)
		if _, ok := s.Validate(tok); !ok {
			t.Fatalf("expired at step %d", i)
		}
	}
	// within one minute of the last use, LastSeen is not rewritten
	r1, _ := s.Validate(tok)
	clk.add(30 * time.Second)
	r2, _ := s.Validate(tok)
	if r1.LastSeen != r2.LastSeen {
		t.Fatal("LastSeen rewritten inside a minute")
	}
	// idle for an hour: expired
	clk.add(time.Hour)
	if _, ok := s.Validate(tok); ok {
		t.Fatal("idle token still valid")
	}
}

func TestAbsoluteExpiry(t *testing.T) {
	s, clk, _ := testStore(t)
	tok, _, _ := s.Issue(KindServer, "dep", "", TTLs{Idle: time.Hour, Max: 3 * time.Hour})
	for i := 0; i < 5; i++ {
		clk.add(30 * time.Minute)
		if _, ok := s.Validate(tok); !ok {
			t.Fatalf("early expiry at %d", i)
		}
	}
	clk.add(31 * time.Minute) // 3h01m after issue, used continuously
	if _, ok := s.Validate(tok); ok {
		t.Fatal("absolute expiry not enforced")
	}
}

func TestDefaultTTLsFromEnv(t *testing.T) {
	t.Setenv("SOLARA_SERVER_TOKEN_IDLE_HOURS", "2")
	t.Setenv("SOLARA_SERVER_TOKEN_MAX_DAYS", "1")
	if d := DefaultTTLs(KindServer); d.Idle != 2*time.Hour || d.Max != 24*time.Hour {
		t.Fatalf("%+v", d)
	}
	t.Setenv("SOLARA_SERVER_TOKEN_IDLE_HOURS", "-1")
	t.Setenv("SOLARA_SERVER_TOKEN_MAX_DAYS", "x")
	if d := DefaultTTLs(KindServer); d.Idle != DefaultIdle || d.Max != DefaultMax {
		t.Fatalf("%+v", d)
	}
	if d := DefaultTTLs(KindAgent); d.Idle != DefaultAgentIdle || d.Max != DefaultAgentMax {
		t.Fatalf("%+v", d)
	}
}

func TestRevokeAndRevokeDeployment(t *testing.T) {
	s, _, _ := testStore(t)
	a1, r1, _ := s.Issue(KindServer, "depA", "", TTLs{})
	a2, _, _ := s.Issue(KindServer, "depA", "", TTLs{})
	b1, _, _ := s.Issue(KindServer, "depB", "", TTLs{})
	ag, rag, _ := s.Issue(KindAgent, "", "gb1", TTLs{})

	if _, err := s.Revoke("nope", "x"); err != ErrNotFound {
		t.Fatal(err)
	}
	if _, err := s.Revoke("sst1_", "x"); err != ErrNotFound { // a prefix must match a display prefix exactly
		t.Fatal(err)
	}
	got, err := s.Revoke(rag.Prefix, "agent retired")
	if err != nil || !got.Revoked() || got.RevokeReason == nil || *got.RevokeReason != "agent retired" {
		t.Fatalf("revoke by prefix: %v %+v", err, got)
	}
	if _, ok := s.Validate(ag); ok {
		t.Fatal("revoked agent token valid")
	}
	if _, ok := s.Validate(a1); !ok {
		t.Fatal("unrelated token invalid")
	}
	n, err := s.RevokeDeployment("depA", "server exited")
	if err != nil || n != 2 {
		t.Fatalf("revoke deployment: %d %v", n, err)
	}
	for _, tk := range []string{a1, a2} {
		if _, ok := s.Validate(tk); ok {
			t.Fatal("deployment token still valid")
		}
	}
	if _, ok := s.Validate(b1); !ok {
		t.Fatal("other deployment revoked")
	}
	// by record id, and idempotent
	if _, err := s.Revoke(r1.IdHex(), "again"); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.RevokeDeployment("depA", "x"); n != 0 {
		t.Fatal("double revoke counted")
	}
	list, _ := s.List()
	if len(list) != 4 {
		t.Fatalf("list %d", len(list))
	}
	for _, r := range list {
		if r.TokenHash == "" || strings.Contains(r.Prefix, a1[11:]) {
			t.Fatal("list leaks")
		}
	}
}

func TestPrune(t *testing.T) {
	s, clk, _ := testStore(t)
	live, _, _ := s.Issue(KindServer, "live", "", TTLs{Idle: time.Hour, Max: 100 * time.Hour})
	_, _, _ = s.Issue(KindServer, "idle", "", TTLs{Idle: time.Hour, Max: 100 * time.Hour})
	_, _, _ = s.Issue(KindServer, "gone", "", TTLs{})
	_, _ = s.RevokeDeployment("gone", "x")
	clk.add(90 * time.Minute)
	_, _ = s.Validate(live) // expired by now? no: issued 90m ago with 1h idle -> yes, expired
	live2, _, _ := s.Issue(KindServer, "live2", "", TTLs{})
	n, err := s.Prune(24 * time.Hour) // nothing dead for a day yet
	if err != nil || n != 0 {
		t.Fatalf("prune grace: %d %v", n, err)
	}
	n, _ = s.Prune(0)
	if n != 3 { // live (idle-expired), idle, gone
		t.Fatalf("prune: %d", n)
	}
	if _, ok := s.Validate(live2); !ok {
		t.Fatal("pruned a live token")
	}
	if l, _ := s.List(); len(l) != 1 {
		t.Fatalf("left %d", len(l))
	}
}

func TestStaticKeyPolicy(t *testing.T) {
	var got []string
	prev, prevNow := WarnLog, warnNow
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	WarnLog = func(s string) { got = append(got, s) }
	warnNow = func() time.Time { return now }
	staticWarnMu.Lock()
	staticWarnLast = map[string]time.Time{}
	staticWarnMu.Unlock()
	t.Cleanup(func() { WarnLog, warnNow = prev, prevNow })

	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "")
	for i := 0; i < 3; i++ {
		if !StaticKeyAllowed("ServerMasterKey", "POST /x") {
			t.Fatal("compat rejected")
		}
	}
	if len(got) != 1 || !strings.Contains(got[0], "POST /x") || !strings.Contains(got[0], "DEPRECATED") {
		t.Fatalf("warnings %q", got)
	}
	now = now.Add(61 * time.Minute)
	StaticKeyAllowed("ServerMasterKey", "POST /x")
	StaticKeyAllowed("ServerMasterKey", "GET /y")
	if len(got) != 3 {
		t.Fatalf("hourly warning: %q", got)
	}
	t.Setenv("SOLARA_REQUIRE_SERVER_TOKENS", "1")
	if StaticKeyAllowed("ServerMasterKey", "POST /x") {
		t.Fatal("strict accepted")
	}
}
