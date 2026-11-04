package mothership

// Solara: real player names on Mothership player records.
//
// Quest sign-in (/v2/player/client/auth/complete/QUEST) only carries UserId + attestation, no name, so every Quest
// player was stored as "Player_<first 8 chars of the Meta id>" - and that placeholder was rewritten on every sign-in.
// The real name IS known to Solara: the station login (A2StationDb /users/log_in, package ares) receives the player's
// Meta username for the same Meta user id. ares wires UsernameLookup at init and calls SyncPlayerName after each
// station login; Mothership sign-ins use the known name and never overwrite a real name with a placeholder.

import (
	"fmt"
	"strings"
	"time"
)

// UsernameLookup returns the station-login username for a Meta user id ("" = unknown). Set by package ares.
var UsernameLookup func(externalId string) string

// isPlaceholderName reports names we generate ourselves (never a real player name).
func isPlaceholderName(n string) bool {
	n = strings.TrimSpace(n)
	return n == "" || strings.HasPrefix(n, "Player_")
}

// resolvePlayerName picks the name to store on a Mothership player record: the station-login username if known,
// else the record's current real name, else fallback.
func resolvePlayerName(externalId string, prev *string, fallback string) string {
	if UsernameLookup != nil && externalId != "" {
		if n := strings.TrimSpace(UsernameLookup(externalId)); !isPlaceholderName(n) {
			return n
		}
	}
	if p := strings.TrimSpace(orStr(prev, "")); !isPlaceholderName(p) {
		return p
	}
	if n := nameOfPlayerId(externalId); n != "" {
		return n
	}
	return fallback
}

// nameOfPlayerId: the game server DLL's quest-fetch logins use a player's Mothership PlayerId (GUID) as the UserId,
// so such a record is named after the player owning that PlayerId ("" = none / no real name).
func nameOfPlayerId(id string) string {
	cl := v2Players()
	if id == "" || cl == nil {
		return ""
	}
	for _, p := range cl.FindAll() {
		if eqS(p.PlayerId, id) {
			if n := strings.TrimSpace(orStr(p.ExternalProviderUsername, "")); !isPlaceholderName(n) {
				return n
			}
		}
	}
	return ""
}

// SyncPlayerName copies a station-login username onto every Mothership player record of that Meta user id.
// Returns how many records changed.
func SyncPlayerName(externalId, username string) int {
	username = strings.TrimSpace(username)
	if externalId == "" || isPlaceholderName(username) {
		return 0
	}
	cl := v2Players()
	if cl == nil {
		return 0
	}
	all := cl.FindAll()
	ids := map[string]bool{externalId: true} // the player's own records + server logins keyed by their PlayerId
	for _, p := range all {
		if eqS(p.ExternalProviderId, externalId) && p.PlayerId != nil {
			ids[*p.PlayerId] = true
		}
	}
	n := 0
	for _, p := range all {
		if ids[orStr(p.ExternalProviderId, "")] && orStr(p.ExternalProviderUsername, "") != username {
			p.ExternalProviderUsername = strp(username)
			cl.Update(p)
			n++
		}
	}
	return n
}

// backfillPlayerNames replaces stored placeholder names with known station-login usernames (once, at startup;
// delayed so the station user store is up).
func backfillPlayerNames() {
	time.Sleep(20 * time.Second)
	if UsernameLookup == nil {
		return
	}
	cl := v2Players()
	if cl == nil {
		return
	}
	fixed, left := 0, 0
	sample := []string{}
	all := cl.FindAll()
	// The game server DLL's quest-fetch logins sign in with a player's Mothership PlayerId (GUID) as the UserId, which
	// creates a second QUEST record keyed by that GUID: name it after the player that PlayerId belongs to.
	byPlayerId := map[string]string{}
	for _, p := range all {
		if n := strings.TrimSpace(orStr(p.ExternalProviderUsername, "")); !isPlaceholderName(n) && p.PlayerId != nil {
			byPlayerId[*p.PlayerId] = n
		}
	}
	for _, p := range all {
		if !isPlaceholderName(orStr(p.ExternalProviderUsername, "")) {
			continue
		}
		n := strings.TrimSpace(UsernameLookup(orStr(p.ExternalProviderId, "")))
		if isPlaceholderName(n) {
			n = byPlayerId[orStr(p.ExternalProviderId, "")]
		}
		if !isPlaceholderName(n) {
			p.ExternalProviderUsername = strp(n)
			cl.Update(p)
			fixed++
		} else {
			left++
			if len(sample) < 8 {
				sample = append(sample, orStr(p.ExternalService, "?")+":"+orStr(p.ExternalProviderId, ""))
			}
		}
	}
	authLog(fmt.Sprintf("[PLAYER-NAME] startup backfill: %d placeholder name(s) replaced, %d still unknown (fixed at their next station login) e.g. %v", fixed, left, sample))
}
