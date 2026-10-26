// Package serverid implements per-server identity tokens (Solara addition,
// see docs/SERVER_IDENTITY.md and KNOWN_ISSUES SEC-SID-*).
//
// The original backend authenticated every game server with two static shared
// secrets (SERVER_API_KEY in x-server-api-key, ServerMasterKey in x-api-key)
// that sat on every game box's command line. This package mints a distinct,
// revocable token per launched server (Kind "server", bound to one deployment
// id) and per allocator agent (Kind "agent", bound to one box name).
//
// Tokens look like "sst1_" + base64url(32 random bytes): only [A-Za-z0-9_-],
// so they survive Unreal command lines and HalcyonAgent.ps1's Safe-Args.
// Only a SHA-256 hash and a short display prefix are stored; the token itself
// is returned exactly once by Issue and is never logged.
//
// serverid must not import ares, mothership or halcyonsocket (they import it).
package serverid

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"solara/internal/docdb"
	"solara/internal/logging"
)

// TokenPrefix marks every token this package issues.
const TokenPrefix = "sst1_"

// Kinds of token.
const (
	KindServer = "server"
	KindAgent  = "agent"
)

// Defaults (overridable via env, see DefaultTTLs).
const (
	DefaultIdle = 24 * time.Hour
	DefaultMax  = 30 * 24 * time.Hour
	// Agent tokens live in a config file on the game box, so they default to a
	// much longer life (the CLI can override both).
	DefaultAgentIdle = 30 * 24 * time.Hour
	DefaultAgentMax  = 365 * 24 * time.Hour

	// slideEvery: Validate writes LastSeen/ExpiresAt at most this often.
	slideEvery = time.Minute
	// indexTTL: the in-memory hash index is reloaded from the database at
	// least this often (picks up rows changed by other tooling).
	indexTTL = time.Minute
	// displayLen: random characters kept in the display prefix.
	displayLen     = 6
	tokenRandBytes = 32
)

// ServerTokenDbObject is the stored token record. It never contains the token.
// Times are Unix milliseconds (UTC); 0 = unset.
type ServerTokenDbObject struct {
	docdb.AstraDbObject
	TokenHash      string  // hex SHA-256 of the full token
	Prefix         string  // "sst1_" + first 6 random chars (display only)
	Kind           string  // "server" | "agent"
	DeploymentId   *string // server tokens: the deployment it may act for
	StationId      *string // optional station binding
	Box            *string // agent tokens: the box name; server tokens: launching box
	IssuedAt       int64
	LastSeen       int64
	ExpiresAt      int64 // sliding idle expiry
	AbsoluteExpiry int64
	IdleMs         int64 // the sliding idle window this token was issued with
	RevokedAt      int64
	RevokeReason   *string
}

// NewServerTokenDbObject is the docdb constructor.
func NewServerTokenDbObject() *ServerTokenDbObject { return &ServerTokenDbObject{} }

// DotnetTypeName / DotnetFullName give the collection a stable name.
func (*ServerTokenDbObject) DotnetTypeName() string { return "ServerTokenDbObject" }
func (*ServerTokenDbObject) DotnetFullName() string {
	return "Solara.ServerIdentity.ServerTokenDbObject"
}

// IdHex returns the record id as hex.
func (r *ServerTokenDbObject) IdHex() string { return r.Id.Hex() }

// Deployment / Station / BoxName dereference the optional bindings.
func (r *ServerTokenDbObject) Deployment() string { return deref(r.DeploymentId) }
func (r *ServerTokenDbObject) Station() string    { return deref(r.StationId) }
func (r *ServerTokenDbObject) BoxName() string    { return deref(r.Box) }

// Revoked reports whether the record was revoked.
func (r *ServerTokenDbObject) Revoked() bool { return r.RevokedAt != 0 }

// TTLs configures a token's lifetime; zero fields take the defaults.
type TTLs struct {
	Idle time.Duration
	Max  time.Duration
}

// Spec describes a token to issue.
type Spec struct {
	Kind         string
	DeploymentId string
	StationId    string
	Box          string
	TTLs         TTLs
}

// Errors.
var (
	ErrNoDatabase = errors.New("serverid: token store has no database")
	ErrBadKind    = errors.New("serverid: kind must be \"server\" or \"agent\"")
	ErrNotFound   = errors.New("serverid: no matching token")
	ErrAmbiguous  = errors.New("serverid: prefix matches more than one token")
)

// Store is a token store over one docdb database.
type Store struct {
	mu  sync.Mutex
	db  *docdb.Database
	Now func() time.Time

	index    map[string]*ServerTokenDbObject // hash -> record
	loadedAt time.Time
}

// NewStore makes a store over db (nil allowed: every operation then fails
// closed with ErrNoDatabase / not-valid).
func NewStore(db *docdb.Database) *Store {
	return &Store{db: db, Now: time.Now}
}

// Default is the process-wide store the servers use. The host sets its
// database once the database opens (SetDatabase).
var Default = NewStore(nil)

// SetDatabase points the default store at db (and creates the collection).
func SetDatabase(db *docdb.Database) {
	Default.SetDatabase(db)
}

// SetDatabase points the store at db and drops the cached index.
func (s *Store) SetDatabase(db *docdb.Database) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db = db
	s.index = nil
	if db != nil {
		docdb.GetCollection(db, true, NewServerTokenDbObject)
	}
}

func (s *Store) col() *docdb.Collection[ServerTokenDbObject] {
	if s.db == nil {
		return nil
	}
	return docdb.GetCollection(s.db, true, NewServerTokenDbObject)
}

func (s *Store) nowMs() int64 { return s.Now().UTC().UnixMilli() }

// ─── token format ────────────────────────────────────────────────────────────

// Hash returns the hex SHA-256 of a token.
func Hash(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// LooksLikeToken reports whether s has the token shape (cheap pre-check; a
// header that does not look like a token is never looked up).
func LooksLikeToken(s string) bool {
	if !strings.HasPrefix(s, TokenPrefix) {
		return false
	}
	body := s[len(TokenPrefix):]
	if len(body) != base64.RawURLEncoding.EncodedLen(tokenRandBytes) {
		return false
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// DisplayPrefix is the non-secret prefix shown in listings and logs.
func DisplayPrefix(token string) string {
	n := len(TokenPrefix) + displayLen
	if len(token) < n {
		return token
	}
	return token[:n]
}

func newToken() (string, error) {
	b := make([]byte, tokenRandBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return TokenPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// ─── TTL configuration ───────────────────────────────────────────────────────

// DefaultTTLs returns the configured TTLs for a kind: server tokens use
// SOLARA_SERVER_TOKEN_IDLE_HOURS (default 24) and SOLARA_SERVER_TOKEN_MAX_DAYS
// (default 30); agent tokens default to 30 days idle / 365 days absolute.
func DefaultTTLs(kind string) TTLs {
	if kind == KindAgent {
		return TTLs{Idle: DefaultAgentIdle, Max: DefaultAgentMax}
	}
	t := TTLs{Idle: DefaultIdle, Max: DefaultMax}
	if v, ok := envPositive("SOLARA_SERVER_TOKEN_IDLE_HOURS"); ok {
		t.Idle = time.Duration(v * float64(time.Hour))
	}
	if v, ok := envPositive("SOLARA_SERVER_TOKEN_MAX_DAYS"); ok {
		t.Max = time.Duration(v * float64(24*time.Hour))
	}
	return t
}

func envPositive(name string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(name)), 64)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// ─── operations ──────────────────────────────────────────────────────────────

// Issue mints a token of kind for deploymentId/box with the given TTLs
// (zero fields = defaults). The token is returned once and never stored.
func (s *Store) Issue(kind, deploymentId, box string, ttls TTLs) (string, *ServerTokenDbObject, error) {
	return s.IssueSpec(Spec{Kind: kind, DeploymentId: deploymentId, Box: box, TTLs: ttls})
}

// IssueSpec mints a token from a full spec (incl. an optional station binding).
func (s *Store) IssueSpec(sp Spec) (string, *ServerTokenDbObject, error) {
	if sp.Kind != KindServer && sp.Kind != KindAgent {
		return "", nil, ErrBadKind
	}
	if sp.Kind == KindServer && strings.TrimSpace(sp.DeploymentId) == "" {
		return "", nil, errors.New("serverid: a server token needs a deployment id")
	}
	if sp.Kind == KindAgent && strings.TrimSpace(sp.Box) == "" {
		return "", nil, errors.New("serverid: an agent token needs a box name")
	}
	def := DefaultTTLs(sp.Kind)
	if sp.TTLs.Idle <= 0 {
		sp.TTLs.Idle = def.Idle
	}
	if sp.TTLs.Max <= 0 {
		sp.TTLs.Max = def.Max
	}
	tok, err := newToken()
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.col()
	if c == nil {
		return "", nil, ErrNoDatabase
	}
	now := s.nowMs()
	r := NewServerTokenDbObject()
	r.TokenHash = Hash(tok)
	r.Prefix = DisplayPrefix(tok)
	r.Kind = sp.Kind
	r.DeploymentId = optStr(sp.DeploymentId)
	r.StationId = optStr(sp.StationId)
	r.Box = optStr(sp.Box)
	r.IssuedAt = now
	r.LastSeen = now
	r.AbsoluteExpiry = now + sp.TTLs.Max.Milliseconds()
	r.IdleMs = sp.TTLs.Idle.Milliseconds()
	r.ExpiresAt = min(now+sp.TTLs.Idle.Milliseconds(), r.AbsoluteExpiry)
	if err := protect(func() { c.Insert(r) }); err != nil {
		return "", nil, err
	}
	if s.index != nil {
		s.index[r.TokenHash] = r
	}
	cp := *r
	return tok, &cp, nil
}

// loadIndex (re)builds the hash index when missing or stale. Caller holds mu.
func (s *Store) loadIndex(c *docdb.Collection[ServerTokenDbObject]) error {
	if s.index != nil && s.Now().Sub(s.loadedAt) < indexTTL {
		return nil
	}
	var all []*ServerTokenDbObject
	if err := protect(func() { all = c.FindAll() }); err != nil {
		return err
	}
	idx := make(map[string]*ServerTokenDbObject, len(all))
	for _, r := range all {
		idx[r.TokenHash] = r
	}
	s.index, s.loadedAt = idx, s.Now()
	return nil
}

// Validate checks a token: known, not revoked, not past its idle or absolute
// expiry. On success it records LastSeen and slides ExpiresAt (at most once
// per minute). The returned record is a copy.
func (s *Store) Validate(token string) (*ServerTokenDbObject, bool) {
	if !LooksLikeToken(token) {
		return nil, false
	}
	h := Hash(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.col()
	if c == nil || s.loadIndex(c) != nil {
		return nil, false
	}
	r, ok := s.index[h]
	// The map lookup is keyed by the hash (not the secret); confirm in
	// constant time anyway.
	if !ok || subtle.ConstantTimeCompare([]byte(r.TokenHash), []byte(h)) != 1 {
		return nil, false
	}
	now := s.nowMs()
	if r.Revoked() || now >= r.ExpiresAt || now >= r.AbsoluteExpiry {
		return nil, false
	}
	if now-r.LastSeen >= slideEvery.Milliseconds() {
		upd := *r
		upd.LastSeen = now
		upd.ExpiresAt = min(now+r.IdleMs, r.AbsoluteExpiry)
		if protect(func() { c.Update(&upd) }) == nil {
			*r = upd
		}
	}
	cp := *r
	return &cp, true
}

// List returns every record (newest first). Records never carry the token.
func (s *Store) List() ([]*ServerTokenDbObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.col()
	if c == nil {
		return nil, ErrNoDatabase
	}
	var all []*ServerTokenDbObject
	if err := protect(func() { all = c.FindAll() }); err != nil {
		return nil, err
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].IssuedAt > all[j].IssuedAt })
	return all, nil
}

// Revoke revokes the token whose record id (hex) or display prefix matches
// idOrPrefix. A prefix must be unambiguous. Revoking twice is not an error.
func (s *Store) Revoke(idOrPrefix, reason string) (*ServerTokenDbObject, error) {
	idOrPrefix = strings.TrimSpace(idOrPrefix)
	if idOrPrefix == "" {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.col()
	if c == nil {
		return nil, ErrNoDatabase
	}
	var all []*ServerTokenDbObject
	if err := protect(func() { all = c.FindAll() }); err != nil {
		return nil, err
	}
	var hit []*ServerTokenDbObject
	for _, r := range all {
		if r.IdHex() == strings.ToLower(idOrPrefix) || r.Prefix == idOrPrefix {
			hit = append(hit, r)
		}
	}
	if len(hit) == 0 {
		return nil, ErrNotFound
	}
	if len(hit) > 1 {
		return nil, ErrAmbiguous
	}
	r := hit[0]
	if !r.Revoked() {
		r.RevokedAt = s.nowMs()
		r.RevokeReason = optStr(reason)
		if err := protect(func() { c.Update(r) }); err != nil {
			return nil, err
		}
	}
	s.index = nil
	cp := *r
	return &cp, nil
}

// RevokeDeployment revokes every live server token bound to deploymentId and
// returns how many were revoked.
func (s *Store) RevokeDeployment(deploymentId, reason string) (int, error) {
	if strings.TrimSpace(deploymentId) == "" {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.col()
	if c == nil {
		return 0, ErrNoDatabase
	}
	var hits []*ServerTokenDbObject
	if err := protect(func() {
		hits = c.Find(func(r *ServerTokenDbObject) bool {
			return r.RevokedAt == 0 && r.Kind == KindServer && r.DeploymentId != nil && *r.DeploymentId == deploymentId
		})
	}); err != nil {
		return 0, err
	}
	now := s.nowMs()
	n := 0
	for _, r := range hits {
		r.RevokedAt = now
		r.RevokeReason = optStr(reason)
		if protect(func() { c.Update(r) }) == nil {
			n++
		}
	}
	if n > 0 {
		s.index = nil
	}
	return n, nil
}

// Prune deletes records that are revoked or expired for longer than grace
// (0 = immediately) and returns how many were removed.
func (s *Store) Prune(grace time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.col()
	if c == nil {
		return 0, ErrNoDatabase
	}
	now := s.nowMs()
	cut := now - grace.Milliseconds()
	var n int
	err := protect(func() {
		n = c.DeleteMany(func(r *ServerTokenDbObject) bool {
			dead := r.RevokedAt
			if dead == 0 {
				if exp := min(r.ExpiresAt, r.AbsoluteExpiry); exp <= now {
					dead = exp
				}
			}
			return dead != 0 && dead <= cut
		})
	})
	if n > 0 {
		s.index = nil
	}
	return n, err
}

// ─── package-level API over Default ──────────────────────────────────────────

// Issue mints a token with the default store.
func Issue(kind, deploymentId, box string, ttls TTLs) (string, *ServerTokenDbObject, error) {
	return Default.Issue(kind, deploymentId, box, ttls)
}

// IssueSpec mints a token from a spec with the default store.
func IssueSpec(sp Spec) (string, *ServerTokenDbObject, error) { return Default.IssueSpec(sp) }

// Validate validates a token with the default store.
func Validate(token string) (*ServerTokenDbObject, bool) { return Default.Validate(token) }

// Revoke revokes by id or prefix with the default store.
func Revoke(idOrPrefix, reason string) (*ServerTokenDbObject, error) {
	return Default.Revoke(idOrPrefix, reason)
}

// RevokeDeployment revokes a deployment's server tokens with the default store.
// A store without a database is a no-op (0, nil) so hooks can call it blindly.
func RevokeDeployment(deploymentId, reason string) (int, error) {
	n, err := Default.RevokeDeployment(deploymentId, reason)
	if errors.Is(err, ErrNoDatabase) {
		return 0, nil
	}
	if n > 0 {
		logging.Console(fmt.Sprintf("[ServerId] revoked %d server token(s) of deployment %s (%s)", n, deploymentId, reason))
	}
	return n, err
}

// List lists the default store's records.
func List() ([]*ServerTokenDbObject, error) { return Default.List() }

// Prune prunes the default store.
func Prune(grace time.Duration) (int, error) { return Default.Prune(grace) }

// ─── helpers ─────────────────────────────────────────────────────────────────

// protect turns a docdb panic (LiteException) into an error.
func protect(f func()) (err error) {
	defer func() {
		if p := recover(); p != nil {
			if e, ok := p.(error); ok {
				err = e
			} else {
				err = fmt.Errorf("%v", p)
			}
		}
	}()
	f()
	return nil
}

func optStr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// FormatMs renders a stored time as RFC 3339 UTC ("" for 0).
func FormatMs(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}
