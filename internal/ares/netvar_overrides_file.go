// Ports AUnrealFeatures.Ares/Servers/NetvarOverridesFile.cs.
//
// Dashboard netvar overrides, pushed to co-located game servers as files.
// Overrides live in a station's Config under the reserved "nv." prefix
// (NetvarOverridePrefix):
//
//	nv.module.<SlotID|*>.<Variable>   gamemode config variable
//	nv.world.<path>                   world netvar
//
// Whenever a station's config changes, BuildText is written atomically to
// <working dir>/netvar_overrides/<deploymentId>.txt for every deployment of
// that station, and to _latest.txt for the station of the most recent online
// deployment. Format:
//
//	v=<SHA1 of the lines, upper-case hex>
//	module<TAB><SlotID|*><TAB><Variable><TAB><value>
//	world<TAB><TAB><path><TAB><value>
//
// Static members: NetvarOverridesFileDir (renamed from Dir: SpecLevelsStore
// also has a public static Dir), Clean, BuildText, ResolveStation,
// OnStationConfigChanged, EnsureAllWritten.
package ares

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/logging"
)

var (
	a2NvGate   sync.Mutex
	a2NvInitMu sync.Mutex
	a2NvInit   bool
)

// NetvarOverridesFileDir mirrors NetvarOverridesFile.Dir
// (Path.Combine(Environment.CurrentDirectory, "netvar_overrides")). Evaluated
// per call: the process never changes its working directory, so this equals
// the C# static readonly value.
func NetvarOverridesFileDir() string { return filepath.Join(a2CurrentDir(), "netvar_overrides") }

// Clean mirrors NetvarOverridesFile.Clean: values typed into the dashboard
// sometimes arrive JSON-quoted; store/serve them bare, tabs/newlines -> ' '.
func Clean(v *string) string {
	s := a2Trim(a2Deref(v))
	if a2UTF16Len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	r := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ")
	return r.Replace(s)
}

// BuildText mirrors NetvarOverridesFile.BuildText(station).
func BuildText(station *models.StationDbObject) string {
	lines := []string{}
	if station != nil && station.Config != nil {
		var keys []string
		for k := range station.Config {
			if strings.HasPrefix(k, NetvarOverridePrefix) { // culture StartsWith == ordinal for this ASCII prefix
				keys = append(keys, k)
			}
		}
		sort.Strings(keys) // StringComparer.Ordinal (UTF-8 byte order == UTF-16 order except surrogates)
		for _, k := range keys {
			rest := k[len(NetvarOverridePrefix):]
			v := station.Config[k]
			value := Clean(v)
			if strings.HasPrefix(rest, "module.") {
				body := rest[len("module."):]
				dot := strings.Index(body, ".")
				if dot <= 0 || dot == len(body)-1 {
					continue
				}
				lines = append(lines, "module\t"+body[:dot]+"\t"+body[dot+1:]+"\t"+value)
			} else if strings.HasPrefix(rest, "world.") {
				lines = append(lines, "world\t\t"+rest[len("world."):]+"\t"+value)
			}
		}
		lines = append(lines, stationConfigWorldLines(station.Config)...)
	}
	joined := strings.Join(lines, "\n")
	sum := sha1.Sum([]byte(joined))
	version := strings.ToUpper(hex.EncodeToString(sum[:])) // Convert.ToHexString
	return "v=" + version + "\n" + joined
}

// ResolveStation mirrors NetvarOverridesFile.ResolveStation: an unknown/fixed
// launch id maps to the most recent online deployment's station.
func ResolveStation(deploymentId string) *models.StationDbObject {
	deps := docdb.GetCollection[models.DeploymentDbObject](Database, true, models.NewDeploymentDbObject)
	stationId := ""
	if deps != nil {
		for _, d := range deps.FindAll() {
			if a2EqS(d.DeploymentId, deploymentId) {
				stationId = a2Deref(d.StationId)
				break
			}
		}
	}
	if stationId == "" {
		stationId = a2Deref(a2LatestOnlineStationId())
	}
	if stationId == "" {
		return nil
	}
	stations := docdb.GetCollection[models.StationDbObject](Database, true, models.NewStationDbObject)
	if stations == nil {
		return nil
	}
	for _, s := range stations.FindAll() {
		if a2EqS(s.StationId, stationId) {
			return s
		}
	}
	return nil
}

// a2LatestOnlineStationId mirrors the private LatestOnlineStationId().
func a2LatestOnlineStationId() *string {
	deps := docdb.GetCollection[models.DeploymentDbObject](Database, true, models.NewDeploymentDbObject)
	if deps == nil {
		return nil
	}
	if d := a2LatestOnlineDeployment(deps.FindAll()); d != nil {
		return d.StationId
	}
	return nil
}

// a2LatestOnlineDeployment mirrors
// .Where(d => d.Online && !string.IsNullOrEmpty(d.StationId)).OrderByDescending(d => d.CreatedAt).FirstOrDefault()
// (a stable sort: the first of equal maxima wins).
func a2LatestOnlineDeployment(all []*models.DeploymentDbObject) *models.DeploymentDbObject {
	var best *models.DeploymentDbObject
	for _, d := range all {
		if !d.Online || a2IsNullOrEmpty(d.StationId) {
			continue
		}
		if best == nil || d.CreatedAt.After(best.CreatedAt) {
			best = d
		}
	}
	return best
}

// OnStationConfigChanged mirrors NetvarOverridesFile.OnStationConfigChanged.
func OnStationConfigChanged(stationId *string) {
	err := func() (err error) {
		defer func() {
			if p := recover(); p != nil {
				err = a2PanicError{p}
			}
		}()
		a2NvGate.Lock()
		defer a2NvGate.Unlock()
		if err := os.MkdirAll(NetvarOverridesFileDir(), 0o755); err != nil {
			return err
		}
		var station *models.StationDbObject
		if col := docdb.GetCollection[models.StationDbObject](Database, true, models.NewStationDbObject); col != nil {
			for _, s := range col.FindAll() {
				if a2EqPtr(s.StationId, stationId) {
					station = s
					break
				}
			}
		}
		text := BuildText(station)
		var deps []*string
		if col := docdb.GetCollection[models.DeploymentDbObject](Database, true, models.NewDeploymentDbObject); col != nil {
			for _, d := range col.FindAll() {
				if a2EqPtr(d.StationId, stationId) {
					deps = append(deps, d.DeploymentId)
				}
			}
		}
		for _, dep := range deps {
			if dep == nil {
				// Safe(null) -> NullReferenceException in the original
				panic(a2NullReference())
			}
			if err := a2WriteAtomic(filepath.Join(NetvarOverridesFileDir(), a2SafeFileName(*dep)+".txt"), []byte(text)); err != nil {
				return err
			}
		}
		if a2EqPtr(a2LatestOnlineStationId(), stationId) {
			if err := a2WriteAtomic(filepath.Join(NetvarOverridesFileDir(), "_latest.txt"), []byte(text)); err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		logging.Console("[NETVARS] could not write override files for " + a2Deref(stationId) + ": " + a2ErrMessage(err))
	}
}

// EnsureAllWritten mirrors NetvarOverridesFile.EnsureAllWritten: the first
// call (the game server's startup fetch) writes every station's files.
func EnsureAllWritten() {
	a2NvInitMu.Lock()
	if a2NvInit {
		a2NvInitMu.Unlock()
		return
	}
	a2NvInit = true
	a2NvInitMu.Unlock()
	var ids []*string
	if col := docdb.GetCollection[models.StationDbObject](Database, true, models.NewStationDbObject); col != nil {
		for _, s := range col.FindAll() {
			ids = append(ids, s.StationId)
		}
	}
	for _, id := range ids {
		OnStationConfigChanged(id)
	}
}

// a2WriteAtomic mirrors File.WriteAllText(tmp) + File.Move(tmp, path, overwrite: true).
func a2WriteAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// a2SafeFileName mirrors NetvarOverridesFile.Safe (invalid file-name chars -> '_').
func a2SafeFileName(s string) string {
	var b strings.Builder
	for _, c := range s {
		if a2InvalidFileNameChar(c) {
			b.WriteByte('_')
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// stationConfigWorldLines (Solara extension): the station's board / sign images and announcement as world
// netvar overrides under config/stationConfig/<key>. On the original build the game server fetched its own
// deployment with include_station_config at dashboard init and applied these natively (sub_5409770); on 50451
// that fetch is not triggered (its DLL trigger still uses stale RVAs), so without this the boards never got
// their URLs. Only non-empty values are sent, and an explicit nv.world.config/stationConfig/<key> wins.
func stationConfigWorldLines(cfg map[string]*string) []string {
	keys := append([]string{}, stationConfigNetvarKeys...)
	sort.Strings(keys)
	out := []string{}
	for _, k := range keys {
		if _, explicit := cfg[NetvarOverridePrefix+"world.config/stationConfig/"+k]; explicit {
			continue
		}
		v, ok := cfg[k]
		if !ok || v == nil || strings.TrimSpace(*v) == "" {
			continue
		}
		out = append(out, "world\t\tconfig/stationConfig/"+k+"\t"+Clean(v))
	}
	return out
}
