// Ports AUnrealFeatures.Ares/Servers/NetvarDumpStore.cs.
//
// Latest full netvar/state dump per deployment, as reported by the game server
// itself ("netvars" ~1.6 MB and "state" server_events). AddServerEvent never
// stores those in LiteDB; this keeps the latest string per deployment in
// memory and persists it to <working dir>/netvar_dumps/<deployment>.<type>.json
// at most once per minute per deployment+type.
//
// Static members become package-level identifiers: IsDumpType, Record, Get,
// Dump, and NetvarDumpStoreList (renamed: SpecLevelsStore and
// DrafterPermissions also have a static List).
package ares

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"solara/internal/dotnet"
	"solara/internal/logging"
	"solara/internal/stj"
)

// Dump mirrors the NetvarDumpStore.Dump record.
type Dump struct {
	DeploymentId  string
	EventType     string
	Data          string
	ReceivedAtUtc dotnet.DateTime
}

var (
	a2DumpMu           sync.Mutex // ConcurrentDictionary semantics for Latest / LastDiskWrite
	a2DumpLatest       = map[string]*Dump{}
	a2DumpLatestOrder  []string // insertion order of Latest keys (enumeration order)
	a2DumpLastDisk     = map[string]dotnet.DateTime{}
	a2DumpDiskInterval = 60 * time.Second
)

// a2DumpDir mirrors the static readonly Dir = Path.Combine(Environment.CurrentDirectory, "netvar_dumps").
func a2DumpDir() string { return filepath.Join(a2CurrentDir(), "netvar_dumps") }

// IsDumpType mirrors NetvarDumpStore.IsDumpType.
func IsDumpType(eventType *string) bool {
	return eventType != nil && (*eventType == "netvars" || *eventType == "state")
}

// Record mirrors NetvarDumpStore.Record(deploymentId, eventType, data).
func Record(deploymentId string, eventType *string, data *string) {
	if a2IsNullOrEmpty(data) || !IsDumpType(eventType) {
		return
	}
	key := deploymentId + "|" + *eventType
	now := dotnet.UtcNow()
	a2DumpMu.Lock()
	if _, ok := a2DumpLatest[key]; !ok {
		a2DumpLatestOrder = append(a2DumpLatestOrder, key)
	}
	a2DumpLatest[key] = &Dump{DeploymentId: deploymentId, EventType: *eventType, Data: *data, ReceivedAtUtc: now}
	if last, ok := a2DumpLastDisk[key]; ok && now.Sub(last) < a2DumpDiskInterval {
		a2DumpMu.Unlock()
		return
	}
	a2DumpLastDisk[key] = now
	a2DumpMu.Unlock()
	err := func() error {
		if err := os.MkdirAll(a2DumpDir(), 0o755); err != nil {
			return err
		}
		return os.WriteFile(a2DumpFilePath(deploymentId, *eventType), []byte(*data), 0o644)
	}()
	if err != nil {
		logging.Console("[NETVARS] could not persist dump: " + a2IOMessage(err))
	}
}

// NetvarDumpStoreList mirrors NetvarDumpStore.List(): in-memory entries plus
// anything persisted on disk from before a restart. Each row is the anonymous
// object { deployment_id, event_type, bytes, received_at, source }.
func NetvarDumpStoreList() []any {
	rows := stj.Obj{}
	func() {
		defer func() { _ = recover() }() // listing is best-effort
		dir := a2DumpDir()
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return
		}
		for _, f := range a2GetFiles(dir, "*.json") {
			base := filepath.Base(f)
			name := strings.TrimSuffix(base, filepath.Ext(base)) // <deployment>.<type>
			dot := strings.LastIndex(name, ".")
			if dot <= 0 {
				continue
			}
			info, err := os.Stat(f)
			if err != nil {
				panic(err)
			}
			rows.Set(name[:dot]+"|"+name[dot+1:], stj.O(
				"deployment_id", name[:dot], "event_type", name[dot+1:],
				"bytes", info.Size(), "received_at", dotnet.FromTime(info.ModTime(), dotnet.Utc), "source", "disk",
			))
		}
	}()
	a2DumpMu.Lock()
	var live []*Dump
	for _, k := range a2DumpLatestOrder {
		live = append(live, a2DumpLatest[k])
	}
	a2DumpMu.Unlock()
	for _, d := range live {
		rows.Set(d.DeploymentId+"|"+d.EventType, stj.O(
			"deployment_id", d.DeploymentId, "event_type", d.EventType,
			"bytes", a2UTF16Len(d.Data), "received_at", d.ReceivedAtUtc, "source", "live",
		))
	}
	out := make([]any, 0, len(rows))
	for _, kv := range rows {
		out = append(out, kv.V)
	}
	return out
}

// Get mirrors NetvarDumpStore.Get(deploymentId, eventType): deploymentId
// null/empty -> the most recently received dump of that type.
func Get(deploymentId *string, eventType string) *Dump {
	if !IsDumpType(&eventType) {
		return nil
	}
	if a2IsNullOrEmpty(deploymentId) {
		a2DumpMu.Lock()
		var newest *Dump
		for _, k := range a2DumpLatestOrder {
			d := a2DumpLatest[k]
			if d.EventType != eventType {
				continue
			}
			// OrderByDescending(...).FirstOrDefault(): first of the maxima (stable)
			if newest == nil || d.ReceivedAtUtc.After(newest.ReceivedAtUtc) {
				newest = d
			}
		}
		a2DumpMu.Unlock()
		if newest != nil {
			return newest
		}
		return func() (out *Dump) {
			defer func() {
				if recover() != nil {
					out = nil
				}
			}()
			dir := a2DumpDir()
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				return nil
			}
			files := a2GetFiles(dir, "*."+eventType+".json")
			if len(files) == 0 {
				return nil
			}
			mt := func(p string) time.Time {
				st, err := os.Stat(p)
				if err != nil {
					// File.GetLastWriteTimeUtc of a vanished file: 1601-01-01
					return time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC)
				}
				return st.ModTime()
			}
			sort.SliceStable(files, func(i, j int) bool { return mt(files[i]).After(mt(files[j])) })
			f := files[0]
			base := filepath.Base(f)
			name := strings.TrimSuffix(base, filepath.Ext(base))
			dot := strings.LastIndex(name, ".")
			if dot < 0 {
				panic("ArgumentOutOfRange") // name[..-1] throws in the original
			}
			text, err := a2ReadAllText(f)
			if err != nil {
				panic(err)
			}
			return &Dump{DeploymentId: name[:dot], EventType: eventType, Data: text, ReceivedAtUtc: dotnet.FromTime(mt(f), dotnet.Utc)}
		}()
	}
	a2DumpMu.Lock()
	hit, ok := a2DumpLatest[*deploymentId+"|"+eventType]
	a2DumpMu.Unlock()
	if ok {
		return hit
	}
	return func() (out *Dump) {
		defer func() {
			if recover() != nil {
				out = nil
			}
		}()
		f := a2DumpFilePath(*deploymentId, eventType)
		st, err := os.Stat(f)
		if err != nil || st.IsDir() {
			return nil
		}
		text, err := a2ReadAllText(f)
		if err != nil {
			panic(err)
		}
		return &Dump{DeploymentId: *deploymentId, EventType: eventType, Data: text, ReceivedAtUtc: dotnet.FromTime(st.ModTime(), dotnet.Utc)}
	}()
}

// a2DumpFilePath mirrors NetvarDumpStore.FilePath: invalid file-name chars and
// '.' become '_'.
func a2DumpFilePath(deploymentId, eventType string) string {
	var b strings.Builder
	for _, c := range deploymentId {
		if a2InvalidFileNameChar(c) || c == '.' {
			b.WriteByte('_')
		} else {
			b.WriteRune(c)
		}
	}
	return filepath.Join(a2DumpDir(), b.String()+"."+eventType+".json")
}

// a2GetFiles mirrors Directory.GetFiles(dir, pattern) for the simple
// "*<suffix>" / "<prefix>*" patterns the original uses (top directory only).
func a2GetFiles(dir, pattern string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if ok, _ := filepath.Match(a2FoldPattern(pattern), a2FoldName(e.Name())); ok {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// Windows file-name matching is case-insensitive.
func a2FoldPattern(p string) string { return a2FoldName(p) }

func a2FoldName(n string) string {
	if a2CaseInsensitiveFS {
		return strings.ToLower(n)
	}
	return n
}

// a2ReadAllText mirrors File.ReadAllText (UTF-8, BOM stripped, invalid
// sequences replaced with U+FFFD).
func a2ReadAllText(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return a2DecodeUTF8(b), nil
}

// a2DecodeUTF8 mirrors Encoding.UTF8.GetString after BOM detection.
func a2DecodeUTF8(b []byte) string {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	return strings.ToValidUTF8(string(b), "\xef\xbf\xbd")
}

// a2IOMessage renders an I/O error roughly as the .NET exception message.
func a2IOMessage(err error) string {
	if pe, ok := err.(*os.PathError); ok {
		return pe.Err.Error() + " : '" + pe.Path + "'"
	}
	return err.Error()
}
