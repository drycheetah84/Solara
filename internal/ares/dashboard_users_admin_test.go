package ares

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

func seedUsers(t *testing.T, n int) {
	t.Helper()
	col := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		u := models.NewUserDataResponse()
		u.UserId = dashStrPtr(fmt.Sprintf("%d", 1000000+i))
		u.Username = dashStrPtr(fmt.Sprintf("Player%03d", i))
		u.LastLogin = dotnet.FromTime(base.Add(time.Duration(i)*time.Minute), dotnet.Utc) // higher i = newer login
		u.Roles = []*string{dashStrPtr("keep-me")}
		col.Insert(u)
	}
}

func TestDashboardUserSearchPaging(t *testing.T) {
	resetDashAuth(t, nil)
	withTempDB(t)
	seedUsers(t, 120)
	s := newAresDashboardServer()

	type page struct {
		Users []struct {
			UserId   string `json:"user_id"`
			Username string `json:"username"`
		} `json:"users"`
		Total, Page, PageSize, Pages int
	}
	get := func(target string) page {
		t.Helper()
		w := do(t, s, "GET", target, "", adminHdr)
		var p struct {
			Users []struct {
				UserId   string `json:"user_id"`
				Username string `json:"username"`
			} `json:"users"`
			Total    int `json:"total"`
			Page     int `json:"page"`
			PageSize int `json:"page_size"`
			Pages    int `json:"pages"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil || w.Code != 200 {
			t.Fatalf("%s: %d %v %s", target, w.Code, err, w.Body.String())
		}
		return page{Users: p.Users, Total: p.Total, Page: p.Page, PageSize: p.PageSize, Pages: p.Pages}
	}

	p := get("/api/users/search")
	if p.Total != 120 || p.Pages != 3 || p.Page != 1 || len(p.Users) != 50 || p.Users[0].Username != "Player119" {
		t.Fatalf("page 1: total %d pages %d page %d n %d first %v", p.Total, p.Pages, p.Page, len(p.Users), p.Users[:1])
	}
	if p := get("/api/users/search?page=3"); len(p.Users) != 20 || p.Users[19].Username != "Player000" {
		t.Fatalf("page 3: n %d", len(p.Users))
	}
	if p := get("/api/users/search?page=99"); p.Page != 3 {
		t.Fatalf("page past the end must clamp to the last page, got %d", p.Page)
	}
	if p := get("/api/users/search?page_size=100000"); p.PageSize != dashUserPageSizeMax {
		t.Fatalf("page_size must be capped, got %d", p.PageSize)
	}
	// search: case-insensitive on name and id, across ALL users (not just the first 50)
	if p := get("/api/users/search?q=player00"); p.Total != 10 {
		t.Fatalf("q=player00: %d matches", p.Total)
	}
	if p := get("/api/users/search?q=1000005"); p.Total != 1 || p.Users[0].Username != "Player005" {
		t.Fatalf("q=user id: %+v", p.Users)
	}
	if p := get("/api/users/search?q=nobody"); p.Total != 0 || p.Pages != 1 || len(p.Users) != 0 {
		t.Fatalf("q=nobody: %+v", p)
	}
	// the original route is unchanged: still the 50 most recent
	var orig []map[string]any
	if w := do(t, s, "GET", "/api/users", "", adminHdr); json.Unmarshal(w.Body.Bytes(), &orig) != nil || len(orig) != 50 {
		t.Fatalf("GET /api/users must stay capped at 50, got %d", len(orig))
	}
}
