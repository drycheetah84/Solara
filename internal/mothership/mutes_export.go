package mothership

// Solara addition: voice mutes for the dashboard and the game servers.
//
// Mutes live in the ported MothershipMuteDbObject collection (the one /v1/moderation/automation already writes), so
// a mute made through the automation API and one made on the dashboard are the same thing. DurationMinutes 0 means
// permanent. That model has no reason field, and its shape is the original backend's, so the optional reason a
// dashboard moderator types is kept beside it in a Solara-only collection (MothershipMuteNoteDbObject, keyed by
// mute_id) instead of changing the model.
//
// The game servers fetch the active list (GET /v1/deployments/{id}/mutes, ares) and enforce it: the muted player is
// silenced for everyone on the server and sees the game's own "MUTED FOR N MINUTES" board.

import (
	"sort"
	"strings"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/mothership/models"
)

// MothershipMuteNoteDbObject is the Solara-only reason for a mute (MothershipMuteDbObject has none).
type MothershipMuteNoteDbObject struct {
	docdb.AstraDbObject
	MuteId *string `json:"mute_id"`
	Reason *string `json:"reason"`
	By     *string `json:"by"`
}

func newMuteNote() *MothershipMuteNoteDbObject { return &MothershipMuteNoteDbObject{} }

func muteNotes() *docdb.Collection[MothershipMuteNoteDbObject] { return col(newMuteNote) }

// ActiveMute is one running voice mute.
type ActiveMute struct {
	MuteId      string           `json:"mute_id"`
	PlayerId    string           `json:"player_id"`
	MetaId      string           `json:"meta_id"`
	Username    string           `json:"username"`
	Minutes     int              `json:"duration_minutes"` // 0 = permanent
	Reason      string           `json:"reason"`
	By          string           `json:"by"`
	CreatedAt   dotnet.DateTime  `json:"created_at"`
	ExpiresAt   *dotnet.DateTime `json:"expires_at"`   // null = permanent
	SecondsLeft int64            `json:"seconds_left"` // 0 = permanent
}

const muteMaxMinutes = 60 * 24 * 365

func muteExpires(m *models.MothershipMuteDbObject) (dotnet.DateTime, bool) {
	if m.DurationMinutes <= 0 {
		return dotnet.DateTime{}, false
	}
	return m.CreatedAt.AddSeconds(float64(m.DurationMinutes) * 60), true
}

// ActiveMutes lists every mute that is permanent or has not run out yet, newest first.
func ActiveMutes() []ActiveMute {
	now := dotnet.UtcNow()
	notes := map[string]*MothershipMuteNoteDbObject{}
	for _, n := range muteNotes().FindAll() {
		notes[sv(n.MuteId)] = n
	}
	out := []ActiveMute{}
	for _, m := range mutes().FindAll() {
		am := ActiveMute{MuteId: sv(m.MuteId), PlayerId: sv(m.PlayerId), Minutes: m.DurationMinutes, CreatedAt: m.CreatedAt}
		if am.PlayerId == "" {
			continue
		}
		if exp, ok := muteExpires(m); ok {
			if !exp.Time().After(now.Time()) { // by instant: dates read back from the DB come back Local-kind
				continue
			}
			e := exp
			am.ExpiresAt = &e
			am.SecondsLeft = int64(exp.Time().Sub(now.Time()).Seconds()) + 1
		}
		if n := notes[am.MuteId]; n != nil {
			am.Reason, am.By = sv(n.Reason), sv(n.By)
		}
		if p := v2Players().FindOne(func(p *models.MothershipV2PlayerDbObject) bool { return dbEq(p.PlayerId, am.PlayerId) }); p != nil {
			am.MetaId, am.Username = sv(p.ExternalProviderId), sv(p.ExternalProviderUsername)
		}
		out = append(out, am)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Time().After(out[j].CreatedAt.Time()) })
	return out
}

// AddMute mutes a Mothership player for `minutes` (0 = permanent). An existing active mute of the player is replaced.
func AddMute(playerId string, minutes int, reason, by string) (*ActiveMute, string) {
	playerId = strings.TrimSpace(playerId)
	if playerId == "" || len(playerId) > 128 || strings.ContainsAny(playerId, "\t\r\n") {
		return nil, "player_id required"
	}
	if minutes < 0 || minutes > muteMaxMinutes {
		return nil, "duration must be between 0 (permanent) and 525600 minutes"
	}
	reason = strings.Join(strings.Fields(strings.ReplaceAll(reason, "\t", " ")), " ")
	if len([]rune(reason)) > 120 {
		return nil, "reason is too long (120 characters max)"
	}
	LiftMutes(playerId)
	m := models.NewMothershipMuteDbObject()
	m.MuteId = strp(newId())
	m.PlayerId = strp(playerId)
	m.DurationMinutes = minutes
	m.CreatedAt = dotnet.UtcNow()
	mutes().Insert(m)
	if reason != "" || by != "" {
		muteNotes().Insert(&MothershipMuteNoteDbObject{MuteId: m.MuteId, Reason: strp(reason), By: strp(by)})
	}
	for _, a := range ActiveMutes() {
		if a.MuteId == sv(m.MuteId) {
			return &a, ""
		}
	}
	return nil, "mute not found after insert"
}

// LiftMutes removes every mute of the player (and its note) and returns how many there were.
func LiftMutes(playerId string) int {
	cl, nl, n := mutes(), muteNotes(), 0
	for _, m := range cl.Find(func(m *models.MothershipMuteDbObject) bool { return dbEq(m.PlayerId, playerId) }) {
		id := sv(m.MuteId)
		for _, note := range nl.Find(func(x *MothershipMuteNoteDbObject) bool { return dbEq(x.MuteId, id) }) {
			nl.Delete(note.Id)
		}
		cl.Delete(m.Id)
		n++
	}
	return n
}
