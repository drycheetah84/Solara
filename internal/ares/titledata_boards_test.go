package ares

import (
	"testing"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

func tdStation(t *testing.T, id string, board string) {
	st := models.NewStationDbObject()
	st.StationId, st.StationName = dashStrPtr(id), dashStrPtr(id)
	st.Config = map[string]*string{}
	if board != "" {
		st.Config["BoardTextureUrl1"] = dashStrPtr(board)
	}
	docdb.GetCollection(Database, true, models.NewStationDbObject).Insert(st)
}

func tdDeployment(id, station string, ageMin float64) {
	d := models.NewDeploymentDbObject()
	d.DeploymentId, d.StationId = dashStrPtr(id), dashStrPtr(station)
	d.CreatedAt = dotnet.UtcNow().AddMinutes(-ageMin)
	le := d.CreatedAt
	d.LastEvent = &le
	d.Online = true
	a2Deployments().Insert(d)
}

// 2026-11-03: a fleet without boards came online last and blanked every client's title-data boards.
func TestBoardTitleDataSkipsFleetsWithoutBoards(t *testing.T) {
	withTempDB(t)
	t.Setenv("SOLARA_TITLEDATA_STATION", "")
	tdStation(t, "main", "https://x/main.png")
	tdStation(t, "empty", "")
	tdDeployment("d1", "main", 30)
	tdDeployment("d2", "empty", 5) // newest, no boards
	if got := boardTitleData()["BoardTextureUrl1"]; got != "https://x/main.png" {
		t.Fatalf("want main's board, got %q", got)
	}
}

// SOLARA_TITLEDATA_STATION pins the fleet whose boards every client gets, even when another booted later.
func TestBoardTitleDataPinnedStation(t *testing.T) {
	withTempDB(t)
	tdStation(t, "main", "https://x/main.png")
	tdStation(t, "other", "https://x/other.png")
	tdDeployment("d1", "main", 30)
	tdDeployment("d2", "other", 5)
	t.Setenv("SOLARA_TITLEDATA_STATION", "main")
	if got := boardTitleData()["BoardTextureUrl1"]; got != "https://x/main.png" {
		t.Fatalf("pinned: got %q", got)
	}
	t.Setenv("SOLARA_TITLEDATA_STATION", "")
	if got := boardTitleData()["BoardTextureUrl1"]; got != "https://x/other.png" {
		t.Fatalf("unpinned should take the newest fleet with boards: got %q", got)
	}
}
