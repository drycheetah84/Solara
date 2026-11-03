package ares

// Solara: title-data editor for the dashboard (Developers / the master key only: these routes are deliberately
// absent from dashScopedRoutes, so every scoped token is refused).
//
//	GET    /api/titledata          -> {known:[...], stored:[...], generated:[...]}
//	PUT    /api/titledata/{key}    body {"value": "..."} -> the stored row
//	DELETE /api/titledata/{key}    -> {deleted: key}
//
// Clients download title data once, at game start (GET /v1/title-data/client), so a change shows on a player's
// next launch. A stored row wins over the generated board URLs of the main fleet.

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"solara/internal/hosting"
	"solara/internal/mothership"
	"solara/internal/stj"
)

// titleDataKnownKey: a key the 50451 client or game server is known to read (IDA + asset export, 2026-11-03).
type titleDataKnownKey struct {
	Key, Reader, Purpose, Format string // Format: "text" | "url" | "json"
	Template                     string // starting value offered for an unset JSON key
}

var titleDataKnownKeys = []titleDataKnownKey{
	{"motd", "Hangar spawn text sign (AxTitleDataTextRenderActor, PlayerStart_Hangar)", "Message of the day shown in the hangar", "text", ""},
	{"credits", "Settings & Safety menu, credit names (AxTitleDataTextBlock)", "The credits list", "text", ""},
	{"StationBrowserFlair", "Station browser (AA2StationBrowser::InitializeTitleDataDrivingNewFlairVisibility)",
		"\"NEW\" badge per game-mode tile", "json",
		`{"ClubNewFlair":false,"DriftballNewFlair":false,"DriftblitzNewFlair":false,"ParkourNewFlair":false,"ScraprunNewFlair":false}`},
	{"VoipSettings", "Game server (UAxTitleDataSubsystem::Initialize, server title data)", "Voice chat tuning", "json",
		`{"SsrTriggerPercentage":0}`},
	{"BoardTextureUrl1", "Promo boards: Club golf, TKB landing platform, large transit landing pad", "Board image", "url", ""},
	{"BoardTextureUrl2", "Promo boards: Club golf, TKB landing platform, large transit landing pad", "Board image", "url", ""},
	{"BoardTextureUrl11", "Boards: Parkour quests, ScrapRun entrance, TKB grand entrance", "Board image", "url", ""},
	{"ShuttlePosterUrl", "Hangar shuttle poster", "Poster image", "url", ""},
	{"ShuttleUrl4", "Hangar shuttle poster", "Poster image", "url", ""},
	{"ShuttleUrl5", "Hangar shuttle poster", "Poster image", "url", ""},
	{"ShuttleUrl6", "Hangar shuttle poster", "Poster image", "url", ""},
}

var titleDataKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]{1,100}$`)

func titleDataFormat(key string) string {
	for _, k := range titleDataKnownKeys {
		if strings.EqualFold(k.Key, key) {
			return k.Format
		}
	}
	return ""
}

// GetTitleDataAdmin is GET /api/titledata.
func (s *AresDashboardServer) GetTitleDataAdmin(c *hosting.Context) hosting.Result {
	known := []any{}
	for _, k := range titleDataKnownKeys {
		known = append(known, stj.O("key", k.Key, "reader", k.Reader, "purpose", k.Purpose, "format", k.Format, "template", k.Template))
	}
	stored := []any{}
	for _, r := range mothership.ListTitleData() {
		stored = append(stored, stj.O("key", r.Key, "value", r.Value, "updated_at", r.UpdatedAt))
	}
	gen := mothership.GeneratedTitleData()
	gk := make([]string, 0, len(gen))
	for k := range gen {
		gk = append(gk, k)
	}
	sort.Strings(gk)
	generated := stj.Obj{}
	for _, k := range gk {
		generated = append(generated, stj.KV{K: k, V: gen[k]})
	}
	return dashJson(200, stj.O("known", known, "stored", stored, "generated", generated))
}

// PutTitleDataAdmin is PUT /api/titledata/{key} with {"value": "..."}.
func (s *AresDashboardServer) PutTitleDataAdmin(c *hosting.Context) hosting.Result {
	key := c.Param("key")
	if !titleDataKeyRe.MatchString(key) {
		return dashJson(400, stj.O("error", "key must be 1-100 letters, digits, _ . -"))
	}
	var body struct {
		Value *string `json:"value"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || body.Value == nil {
		return dashJson(400, stj.O("error", `body must be {"value": "..."}`))
	}
	if len(*body.Value) > 64*1024 {
		return dashJson(400, stj.O("error", "value too large (max 64 KB)"))
	}
	if titleDataFormat(key) == "json" && !json.Valid([]byte(*body.Value)) {
		return dashJson(400, stj.O("error", key+" must be valid JSON"))
	}
	r := mothership.SetTitleDataValue(key, *body.Value)
	return dashJson(200, stj.O("key", r.Key, "value", r.Value, "updated_at", r.UpdatedAt))
}

// DeleteTitleDataAdmin is DELETE /api/titledata/{key}.
func (s *AresDashboardServer) DeleteTitleDataAdmin(c *hosting.Context) hosting.Result {
	key := c.Param("key")
	if !mothership.DeleteTitleDataKey(key) {
		return dashJson(404, stj.O("error", "no stored value for "+key))
	}
	return dashJson(200, stj.O("deleted", key))
}
