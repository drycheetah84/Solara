package ares

import (
	"strings"
	"testing"
	"time"

	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/mothership"
	mmodels "solara/internal/mothership/models"
)

func TestDashboardGameBans(t *testing.T) {
	// The DB hands DateTimes back as Local (like the .NET store); the live box runs on UTC, so pin it here.
	oldLoc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = oldLoc })
	resetDashAuth(t, nil)
	withTempDB(t)
	old := mothership.Database
	mothership.Database = Database
	t.Cleanup(func() { mothership.Database = old })
	s := newAresDashboardServer()

	ban := mmodels.NewMothershipBanDbObject()
	ban.BanId, ban.PlayerId, ban.Reason = dashStrPtr("b1"), dashStrPtr("P1"), dashStrPtr("test ban")
	ban.CreatedAt, ban.ExpiresAt = dotnet.UtcNow(), dotnet.UtcNow().AddMinutes(10)
	docdb.GetCollection(Database, true, mmodels.NewMothershipBanDbObject).Insert(ban)
	pl := mmodels.NewMothershipV2PlayerDbObject()
	pl.PlayerId, pl.ExternalProviderId, pl.ExternalProviderUsername = dashStrPtr("P1"), dashStrPtr("123"), dashStrPtr("unreleasedd")
	docdb.GetCollection(Database, true, mmodels.NewMothershipV2PlayerDbObject).Insert(pl)

	if w := do(t, s, "GET", "/api/game_bans", "", nil); w.Code == 200 {
		t.Fatal("game_bans must need the dashboard key")
	}
	b := do(t, s, "GET", "/api/game_bans", "", adminHdr).Body.String()
	if !strings.Contains(b, `"player_id":"P1"`) || !strings.Contains(b, `"username":"unreleasedd"`) || !strings.Contains(b, `"meta_id":"123"`) {
		t.Fatal(b)
	}
	if b := do(t, s, "POST", "/api/game_bans/P1/unban", "", adminHdr).Body.String(); !strings.Contains(b, `"success":true`) {
		t.Fatal(b)
	}
	if b := do(t, s, "GET", "/api/game_bans", "", adminHdr).Body.String(); b != "[]" {
		t.Fatal(b)
	}
	if b := do(t, s, "POST", "/api/game_bans/P1/unban", "", adminHdr).Body.String(); !strings.Contains(b, "no active ban") {
		t.Fatal(b)
	}
}
