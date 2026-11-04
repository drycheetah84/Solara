package ares

// Solara: GET /v1/anonymous_name?seed=<int> — the name tag for the 50451 client's anonymous mode.
//
// AxPlayerState::Server_SetAnonymousMode(bEnabled, RandomSeed) runs on the game server, which asks the station
// dashboard (DashboardApiUrl, this server) for a display name and parses FAnonymousNameResponse {Name}. The
// original backend never had this route: the request 404'd, the game logged "Failed to get anonymous name from
// station dashboard" and kept the player's real name, so anonymous mode only removed the name-tag colour.
//
// A name is Adjective + Noun + a number, picked by the seed alone (same seed and settings -> same name), so nothing
// about the player ends up in it. The word lists and the number range are editable on the dashboard
// (dashboard_anonymous_names.go); with nothing saved the built-in defaults below are used.

import (
	"strconv"
	"strings"
	"sync"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

var anonNameAdjectives = []string{
	"Quiet", "Swift", "Lucky", "Bright", "Silent", "Brave", "Clever", "Cosmic", "Crimson", "Dusty",
	"Electric", "Frosty", "Gentle", "Golden", "Hidden", "Jolly", "Lunar", "Mellow", "Misty", "Neon",
	"Nimble", "Orbital", "Pixel", "Rapid", "Rusty", "Shadow", "Sleepy", "Solar", "Spare", "Stellar",
	"Sunny", "Turbo", "Velvet", "Wild", "Wobbly", "Zesty", "Atomic", "Breezy", "Cozy", "Drifty",
}

var anonNameNouns = []string{
	"Comet", "Drifter", "Robot", "Rocket", "Pilot", "Nova", "Orbit", "Spark", "Gizmo", "Bolt",
	"Circuit", "Pebble", "Falcon", "Otter", "Panda", "Fox", "Badger", "Koala", "Puffin", "Gecko",
	"Meteor", "Nebula", "Quasar", "Satellite", "Sprocket", "Widget", "Beacon", "Glider", "Hopper", "Rover",
	"Voyager", "Wanderer", "Runner", "Racer", "Striker", "Keeper", "Ranger", "Scout", "Signal", "Static",
}

// AnonNameSettings drive the generator. NumberMin..NumberMax is the inclusive range of the trailing number;
// 0..0 means no number.
type AnonNameSettings struct {
	Adjectives []string `json:"adjectives"`
	Nouns      []string `json:"nouns"`
	NumberMin  int      `json:"number_min"`
	NumberMax  int      `json:"number_max"`
}

// AnonNameDefaults are the built-in settings (used while nothing is saved).
func AnonNameDefaults() AnonNameSettings {
	return AnonNameSettings{
		Adjectives: append([]string{}, anonNameAdjectives...),
		Nouns:      append([]string{}, anonNameNouns...),
		NumberMin:  10, NumberMax: 99,
	}
}

// AnonNameConfigDbObject is the saved generator settings (one document).
type AnonNameConfigDbObject struct {
	docdb.AstraDbObject
	AnonNameSettings
	UpdatedAt string `json:"updated_at"`
}

func NewAnonNameConfigDbObject() *AnonNameConfigDbObject { return &AnonNameConfigDbObject{} }

func a2AnonNameConfigs() *docdb.Collection[AnonNameConfigDbObject] {
	return docdb.GetCollection[AnonNameConfigDbObject](Database, true, NewAnonNameConfigDbObject)
}

var (
	anonNameMu     sync.Mutex
	anonNameCached *AnonNameSettings // nil = not loaded yet
)

// savedAnonNameConfig is the stored document, or nil when the defaults are in use.
func savedAnonNameConfig() *AnonNameConfigDbObject {
	col := a2AnonNameConfigs()
	if col == nil {
		return nil
	}
	if all := col.FindAll(); len(all) > 0 {
		return all[0]
	}
	return nil
}

// CurrentAnonNameSettings are the settings the generator uses now (saved ones, else the defaults).
func CurrentAnonNameSettings() AnonNameSettings {
	anonNameMu.Lock()
	defer anonNameMu.Unlock()
	if anonNameCached == nil {
		s := AnonNameDefaults()
		if doc := savedAnonNameConfig(); doc != nil && len(doc.Adjectives) > 0 && len(doc.Nouns) > 0 {
			s = doc.AnonNameSettings
		}
		anonNameCached = &s
	}
	return *anonNameCached
}

// SaveAnonNameSettings stores s (already validated) as the only settings document; nil resets to the defaults.
func SaveAnonNameSettings(s *AnonNameSettings, now string) {
	anonNameMu.Lock()
	defer anonNameMu.Unlock()
	if col := a2AnonNameConfigs(); col != nil {
		col.DeleteAll()
		if s != nil {
			doc := NewAnonNameConfigDbObject()
			doc.AnonNameSettings = *s
			doc.UpdatedAt = now
			col.Insert(doc)
		}
	}
	anonNameCached = nil
}

// AnonymousNameWith picks the name for seed from the given settings.
func AnonymousNameWith(s AnonNameSettings, seed int64) string {
	if len(s.Adjectives) == 0 || len(s.Nouns) == 0 {
		s = AnonNameDefaults()
	}
	// splitmix64: spreads nearby seeds across the whole name space.
	x := uint64(seed) + 0x9E3779B97F4A7C15
	x = (x ^ (x >> 30)) * 0xBF58476D1CE4E5B9
	x = (x ^ (x >> 27)) * 0x94D049BB133111EB
	x ^= x >> 31
	adj := s.Adjectives[x%uint64(len(s.Adjectives))]
	x /= uint64(len(s.Adjectives))
	noun := s.Nouns[x%uint64(len(s.Nouns))]
	x /= uint64(len(s.Nouns))
	if s.NumberMin == 0 && s.NumberMax == 0 {
		return adj + noun
	}
	span := uint64(s.NumberMax-s.NumberMin) + 1
	return adj + noun + strconv.Itoa(s.NumberMin+int(x%span))
}

// AnonymousNameForSeed returns the anonymous display name for a seed with the current settings.
func AnonymousNameForSeed(seed int64) string {
	return AnonymousNameWith(CurrentAnonNameSettings(), seed)
}

// GetAnonymousName handles GET /v1/anonymous_name?seed=<int>. A missing or malformed seed counts as 0.
func (s *A2StationDbServer) GetAnonymousName(c *hosting.Context) hosting.Result {
	seed, _ := strconv.ParseInt(strings.TrimSpace(c.Request.GetQueryParameter("seed", "")), 10, 64)
	return hosting.Ok(stj.O("name", AnonymousNameForSeed(seed)))
}
