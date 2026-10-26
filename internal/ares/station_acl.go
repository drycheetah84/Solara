// Ports AUnrealFeatures.Ares/Servers/StationAcl.cs.
//
// Station access control: which accounts may SEE a station in the EOS station
// browser. The list lives in the station's Config under "acl.whitelist" as a
// comma-separated list of usernames; empty/absent means visible to everyone.
// The values are pushed into the EOS gateway (eos.SetStationWhitelist) on
// every config change and once for every station at startup.
//
// Static members: WhitelistKey, Parse, Push, PushAll.
package ares

import (
	"strings"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/eos"
	"solara/internal/logging"
)

// WhitelistKey mirrors StationAcl.WhitelistKey.
const WhitelistKey = "acl.whitelist"

// Parse mirrors StationAcl.Parse: split on ',', ';', '\n' (empty entries
// removed), trim, drop empties.
func Parse(value *string) []string {
	out := []string{}
	for _, s := range strings.FieldsFunc(a2Deref(value), func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		s = a2Trim(s)
		if len(s) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// Push mirrors StationAcl.Push(stationId).
func Push(stationId *string) {
	defer func() {
		if p := recover(); p != nil {
			logging.Console("[ACL] could not push whitelist for " + a2Deref(stationId) + ": " + a2ExceptionMessage(p))
		}
	}()
	var station *models.StationDbObject
	if col := docdb.GetCollection[models.StationDbObject](Database, true, models.NewStationDbObject); col != nil {
		for _, s := range col.FindAll() {
			if a2EqPtr(s.StationId, stationId) {
				station = s
				break
			}
		}
	}
	var raw *string
	if station != nil && station.Config != nil {
		if v, ok := station.Config[WhitelistKey]; ok {
			raw = v
		}
	}
	eos.SetStationWhitelist(a2Deref(stationId), Parse(raw)) // null id: IsNullOrWhiteSpace -> ignored
}

// PushAll mirrors StationAcl.PushAll().
func PushAll() {
	defer func() {
		if p := recover(); p != nil {
			logging.Console("[ACL] could not push whitelists: " + a2ExceptionMessage(p))
		}
	}()
	stations := []*models.StationDbObject{}
	if col := docdb.GetCollection[models.StationDbObject](Database, true, models.NewStationDbObject); col != nil {
		stations = col.FindAll()
	}
	for _, s := range stations {
		var raw *string
		if s.Config != nil {
			if v, ok := s.Config[WhitelistKey]; ok {
				raw = v
			}
		}
		eos.SetStationWhitelist(a2Deref(s.StationId), Parse(raw))
	}
}
