// Ports AUnrealFeatures.Ares/Servers/SpecLevelsStore.cs.
//
// Saved Spec Editor levels. Storage: <working dir>/spec_levels/<name>.txt plus
// _index.json (Dictionary<string, Entry> written with WriteIndented). All
// writes are tmp + move, under one lock.
//
// Static members become package-level identifiers prefixed with the class
// name (SpecLevelsStoreDir, SpecLevelsStoreList, SpecLevelsStoreCleanName,
// ...ReadText, ...Put, ...Update, ...Delete, ...Boot, ...Desired, ...Status,
// ...IsTrustedServer; nested types SpecLevelsStoreEntry and
// SpecLevelsStoreLevelInfo): Dir and List clash with NetvarOverridesFile /
// NetvarDumpStore / DrafterPermissions, and the remaining names are too
// generic to leave unprefixed in a shared package.
package ares

import (
	"bytes"
	"crypto/subtle"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/secrets"
	"solara/internal/serverid"
	"solara/internal/stj"
)

// SpecLevelsStoreEntry mirrors SpecLevelsStore.Entry.
type SpecLevelsStoreEntry struct {
	Autoload     bool     `json:"autoload"`
	Loaded       bool     `json:"loaded"`
	Updated      string   `json:"updated"`
	Size         int      `json:"size"`
	ServerLoaded []string `json:"serverLoaded"`
	ServerSeen   string   `json:"serverSeen"`
}

// NewSpecLevelsStoreEntry applies the C# property initializers.
func NewSpecLevelsStoreEntry() *SpecLevelsStoreEntry {
	return &SpecLevelsStoreEntry{Updated: "", ServerLoaded: []string{}, ServerSeen: ""}
}

// SpecLevelsStoreLevelInfo mirrors the SpecLevelsStore.LevelInfo(Name, Entry) record.
type SpecLevelsStoreLevelInfo struct {
	Name  string
	Entry *SpecLevelsStoreEntry
}

var (
	a2SpecGate sync.Mutex
	// .NET '$' also matches before a final '\n'.
	a2SpecNameRx = regexp.MustCompile(`^[A-Za-z0-9 _-]{1,64}\n?$`)
)

// SpecLevelsStoreDir mirrors SpecLevelsStore.Dir
// (Path.Combine(Environment.CurrentDirectory, "spec_levels")).
func SpecLevelsStoreDir() string { return filepath.Join(a2CurrentDir(), "spec_levels") }

func a2SpecIndexPath() string { return filepath.Join(SpecLevelsStoreDir(), "_index.json") }

// a2SpecIndex is Dictionary<string, Entry>(StringComparer.Ordinal) with its
// enumeration (insertion) order.
type a2SpecIndex struct {
	keys []string
	m    map[string]*SpecLevelsStoreEntry
}

func newA2SpecIndex() *a2SpecIndex { return &a2SpecIndex{m: map[string]*SpecLevelsStoreEntry{}} }

func (x *a2SpecIndex) set(k string, e *SpecLevelsStoreEntry) {
	if _, ok := x.m[k]; !ok {
		x.keys = append(x.keys, k)
	}
	x.m[k] = e
}

func (x *a2SpecIndex) remove(k string) bool {
	if _, ok := x.m[k]; !ok {
		return false
	}
	delete(x.m, k)
	for i, s := range x.keys {
		if s == k {
			x.keys = append(x.keys[:i:i], x.keys[i+1:]...)
			break
		}
	}
	return true
}

// SpecLevelsStoreCleanName mirrors SpecLevelsStore.CleanName: route params arrive
// percent-encoded, so "My%20Level" -> "My Level"; null when the trimmed name
// is not [A-Za-z0-9 _-]{1,64}.
func SpecLevelsStoreCleanName(raw *string) *string {
	if raw == nil {
		return nil
	}
	name := a2Trim(a2UnescapeDataString(*raw))
	if !a2SpecNameRx.MatchString(name) {
		return nil
	}
	return &name
}

// a2UnescapeDataString mirrors Uri.UnescapeDataString for the purpose of
// CleanName: valid %XX escapes are decoded, anything else is kept as-is.
func a2UnescapeDataString(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && a2IsHex(s[i+1]) && a2IsHex(s[i+2]) {
			b.WriteByte(a2Unhex(s[i+1])<<4 | a2Unhex(s[i+2]))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func a2IsHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func a2Unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

func a2LevelPath(name string) string { return filepath.Join(SpecLevelsStoreDir(), name+".txt") }

// a2ResolveKey mirrors ResolveKey: an existing key matched case-insensitively.
func a2ResolveKey(idx *a2SpecIndex, name string) string {
	for _, k := range idx.keys {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return name
}

// a2LoadIndex mirrors LoadIndex (call under the gate).
func a2LoadIndex() *a2SpecIndex {
	if idx, err := a2ReadIndexFile(); err != nil {
		logging.Console("[SPEC-LEVELS] index unreadable, rebuilding: " + a2ErrMessage(err))
	} else if idx != nil {
		return idx
	}
	// Missing or corrupt index: rebuild from the .txt files so a level is never lost.
	rebuilt := newA2SpecIndex()
	dir := SpecLevelsStoreDir()
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		for _, f := range a2GetFiles(dir, "*.txt") {
			base := filepath.Base(f)
			n := strings.TrimSuffix(base, filepath.Ext(base))
			if !a2SpecNameRx.MatchString(n) {
				continue
			}
			fi, err := os.Stat(f)
			if err != nil {
				panic(err)
			}
			e := NewSpecLevelsStoreEntry()
			e.Loaded, e.Autoload = false, false
			e.Size = int(int32(fi.Size()))
			e.Updated = dotnet.FromTime(fi.ModTime(), dotnet.Utc).FormatO()
			rebuilt.set(n, e)
		}
	}
	return rebuilt
}

// a2ReadIndexFile returns (nil, nil) when there is no index file or it holds
// JSON null, and an error where the original caught an exception.
func a2ReadIndexFile() (idx *a2SpecIndex, err error) {
	defer func() {
		if p := recover(); p != nil {
			idx, err = nil, a2PanicError{p}
		}
	}()
	p := a2SpecIndexPath()
	if st, e := os.Stat(p); e != nil || st.IsDir() {
		return nil, nil
	}
	text, e := a2ReadAllText(p)
	if e != nil {
		return nil, e
	}
	el, e := stj.Parse([]byte(text))
	if e != nil {
		return nil, a2PanicError{&dotnet.Exception{Type: "System.Text.Json.JsonException", Message: e.Error()}}
	}
	if el.ValueKind() == stj.Null {
		return nil, nil
	}
	if el.ValueKind() != stj.Object {
		return nil, a2PanicError{&dotnet.Exception{Type: "System.Text.Json.JsonException",
			Message: "The JSON value could not be converted to System.Collections.Generic.Dictionary`2[System.String,AUnrealFeatures.Ares.Servers.SpecLevelsStore+Entry]. Path: $ | LineNumber: 0 | BytePositionInLine: 1."}}
	}
	out := newA2SpecIndex()
	for _, prop := range el.EnumerateObject() {
		var e *SpecLevelsStoreEntry
		if prop.Value.ValueKind() != stj.Null {
			e = NewSpecLevelsStoreEntry()
			if err := stj.FromElement(prop.Value, e); err != nil {
				return nil, a2PanicError{&dotnet.Exception{Type: "System.Text.Json.JsonException", Message: err.Error()}}
			}
		}
		out.set(prop.Name, e)
	}
	for _, k := range out.keys {
		if out.m[k] == nil {
			// `foreach (var e in d.Values) e.ServerLoaded ??= ...` on a null entry
			panic(a2NullReference())
		}
		if out.m[k].ServerLoaded == nil {
			out.m[k].ServerLoaded = []string{}
		}
	}
	return out, nil
}

// a2NewLine mirrors Environment.NewLine (Utf8JsonWriter's indented line break).
func a2NewLine() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// a2SaveIndex mirrors SaveIndex: JsonSerializer.Serialize(idx, WriteIndented).
func a2SaveIndex(idx *a2SpecIndex) {
	o := stj.Obj{}
	for _, k := range idx.keys {
		o.Set(k, idx.m[k])
	}
	if err := a2SpecWriteAtomic(a2SpecIndexPath(), a2IndentJSON(stj.MustMarshal(o), a2NewLine())); err != nil {
		panic(err)
	}
}

func a2SpecWriteAtomic(path string, data []byte) error {
	if err := os.MkdirAll(SpecLevelsStoreDir(), 0o755); err != nil {
		return err
	}
	return a2WriteAtomic(path, data)
}

// a2IndentJSON re-formats compact STJ output the way Utf8JsonWriter does with
// Indented = true: two-space indentation, `"name": value`, and "{}" / "[]"
// for empty containers.
func a2IndentJSON(compact []byte, nl string) []byte {
	var b bytes.Buffer
	depth := 0
	indent := func() {
		b.WriteString(nl)
		b.WriteString(strings.Repeat("  ", depth))
	}
	inStr := false
	for i := 0; i < len(compact); i++ {
		c := compact[i]
		if inStr {
			b.WriteByte(c)
			if c == '\\' {
				i++
				b.WriteByte(compact[i])
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
			b.WriteByte(c)
		case '{', '[':
			b.WriteByte(c)
			if i+1 < len(compact) && (compact[i+1] == '}' || compact[i+1] == ']') {
				b.WriteByte(compact[i+1])
				i++
				continue
			}
			depth++
			indent()
		case '}', ']':
			depth--
			indent()
			b.WriteByte(c)
		case ',':
			b.WriteByte(c)
			indent()
		case ':':
			b.WriteString(": ")
		default:
			b.WriteByte(c)
		}
	}
	return b.Bytes()
}

func a2SpecNow() string { return dotnet.UtcNow().FormatO() }

// a2CompareOrdinalIgnoreCase mirrors StringComparer.OrdinalIgnoreCase.Compare.
func a2CompareOrdinalIgnoreCase(a, b string) int {
	return strings.Compare(strings.ToUpper(a), strings.ToUpper(b))
}

// SpecLevelsStoreList mirrors SpecLevelsStore.List() (OrderBy key, OrdinalIgnoreCase).
func SpecLevelsStoreList() []*SpecLevelsStoreLevelInfo {
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	idx := a2LoadIndex()
	keys := append([]string(nil), idx.keys...)
	sort.SliceStable(keys, func(i, j int) bool { return a2CompareOrdinalIgnoreCase(keys[i], keys[j]) < 0 })
	out := []*SpecLevelsStoreLevelInfo{}
	for _, k := range keys {
		out = append(out, &SpecLevelsStoreLevelInfo{Name: k, Entry: idx.m[k]})
	}
	return out
}

// SpecLevelsStoreReadText mirrors SpecLevelsStore.ReadText (nil when the file is missing).
func SpecLevelsStoreReadText(name string) *string {
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	p := a2LevelPath(name)
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		return nil
	}
	text, err := a2ReadAllText(p)
	if err != nil {
		panic(err)
	}
	return &text
}

// SpecLevelsStorePut mirrors SpecLevelsStore.Put(name, body).
func SpecLevelsStorePut(name string, body []byte) {
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	idx := a2LoadIndex()
	key := a2ResolveKey(idx, name)
	e, ok := idx.m[key]
	if !ok {
		e = NewSpecLevelsStoreEntry()
		e.Autoload, e.Loaded = false, true
		idx.set(key, e)
	}
	if err := a2SpecWriteAtomic(a2LevelPath(key), body); err != nil {
		panic(err)
	}
	e.Updated = a2SpecNow()
	e.Size = len(body)
	a2SaveIndex(idx)
}

// SpecLevelsStoreUpdate mirrors SpecLevelsStore.Update(name, autoload, loaded); false when
// the level does not exist.
func SpecLevelsStoreUpdate(name string, autoload *bool, loaded *bool) bool {
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	idx := a2LoadIndex()
	key := a2ResolveKey(idx, name)
	e, ok := idx.m[key]
	if !ok {
		return false
	}
	if autoload != nil {
		e.Autoload = *autoload
	}
	if loaded != nil {
		e.Loaded = *loaded
	}
	a2SaveIndex(idx)
	return true
}

// SpecLevelsStoreDelete mirrors SpecLevelsStore.Delete(name).
func SpecLevelsStoreDelete(name string) bool {
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	idx := a2LoadIndex()
	key := a2ResolveKey(idx, name)
	had := idx.remove(key)
	p := a2LevelPath(key)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		if err := os.Remove(p); err != nil {
			panic(err)
		}
		had = true
	}
	if had {
		a2SaveIndex(idx)
	}
	return had
}

// a2LoadedNames mirrors idx.Where(k => k.Value.Loaded).Select(k => k.Key)
// .OrderBy(n => n, StringComparer.OrdinalIgnoreCase).
func a2LoadedNames(idx *a2SpecIndex) []string {
	out := []string{}
	for _, k := range idx.keys {
		if idx.m[k].Loaded {
			out = append(out, k)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return a2CompareOrdinalIgnoreCase(out[i], out[j]) < 0 })
	return out
}

// SpecLevelsStoreBoot mirrors SpecLevelsStore.Boot: every level's desired state resets to its
// autoload flag. Returns the names to load.
func SpecLevelsStoreBoot(deploymentId string) []string {
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	idx := a2LoadIndex()
	for _, k := range idx.keys {
		e := idx.m[k]
		e.Loaded = e.Autoload
		// This deployment starts with nothing loaded until it reports status.
		if deploymentId != "" {
			e.ServerLoaded, _ = a2RemoveS(e.ServerLoaded, deploymentId)
		}
	}
	a2SaveIndex(idx)
	return a2LoadedNames(idx)
}

// SpecLevelsStoreDesired mirrors SpecLevelsStore.Desired (names with loaded == true).
func SpecLevelsStoreDesired() []string {
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	return a2LoadedNames(a2LoadIndex())
}

// SpecLevelsStoreStatus mirrors SpecLevelsStore.Status: a server reports the full set of
// level names it currently has loaded.
func SpecLevelsStoreStatus(deploymentId string, loadedNames []string) {
	set := map[string]bool{} // HashSet(StringComparer.OrdinalIgnoreCase)
	for _, n := range loadedNames {
		set[strings.ToUpper(n)] = true
	}
	a2SpecGate.Lock()
	defer a2SpecGate.Unlock()
	idx := a2LoadIndex()
	now := a2SpecNow()
	for _, k := range idx.keys {
		e := idx.m[k]
		has := a2ContainsS(e.ServerLoaded, deploymentId)
		if set[strings.ToUpper(k)] {
			if !has {
				e.ServerLoaded = append(e.ServerLoaded, deploymentId)
			}
		} else if has {
			e.ServerLoaded, _ = a2RemoveS(e.ServerLoaded, deploymentId)
		}
		e.ServerSeen = now
	}
	a2SaveIndex(idx)
}

// ── Game-server auth (x-server-api-key) ─────────────────────────────────────

var (
	a2SpecKeyOnce sync.Once
	a2SpecKey     string
)

// a2SpecServerApiKey mirrors the static readonly ServerApiKey = LoadServerApiKey().
func a2SpecServerApiKey() string {
	a2SpecKeyOnce.Do(func() { a2SpecKey = a2SpecLoadServerApiKey() })
	return a2SpecKey
}

// a2SpecLoadServerApiKey mirrors SpecLevelsStore.LoadServerApiKey: env
// HALCYON_SERVER_API_KEY, then server_api_key.txt next to the executable,
// then the legacy baked fallback (secret SERVER_API_KEY).
func a2SpecLoadServerApiKey() string {
	if env, ok := os.LookupEnv("HALCYON_SERVER_API_KEY"); ok && !a2IsBlank(env) {
		return a2Trim(env)
	}
	path := filepath.Join(a2BaseDirectory(), "server_api_key.txt")
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		// read errors fall through to the legacy literal
		if text, err := a2ReadAllText(path); err == nil {
			if k := a2Trim(text); len(k) > 0 {
				return k
			}
		}
	}
	return secrets.Get("SERVER_API_KEY")
}

// SpecLevelsStoreIsTrustedServer mirrors SpecLevelsStore.IsTrustedServer(req): the
// x-server-api-key header compared in constant time.
func SpecLevelsStoreIsTrustedServer(req *hosting.Request) bool {
	k, ok := req.Header("x-server-api-key")
	if !ok || k == "" {
		return false
	}
	// Solara (SEC-SID): a valid per-server token qualifies; the static key is
	// subject to the strict/compat policy.
	if serverid.LooksLikeToken(k) {
		r, ok := serverid.Validate(k)
		return ok && r.Kind == serverid.KindServer
	}
	return subtle.ConstantTimeCompare([]byte(k), []byte(a2SpecServerApiKey())) == 1 &&
		serverid.StaticKeyAllowed("SERVER_API_KEY", "A2StationDb SpecLevelsStore.IsTrustedServer")
}
