package ares

// Solara: dashboard Users tab additions.
//
//	GET /api/users/search?q=&page=1&page_size=50 -> {users, total, page, page_size, pages}
//	    Every user (GET /api/users stays the original: the 50 most recent logins), newest login first, a page at a
//	    time. q matches the username or the user id, case-insensitive. Same scoped-token access as GET /api/users.

import (
	"strconv"
	"strings"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

const (
	dashUserPageSizeDefault = 50
	dashUserPageSizeMax     = 500
)

// SearchUsers is GET /api/users/search.
func (s *AresDashboardServer) SearchUsers(c *hosting.Context) hosting.Result {
	q := strings.ToLower(strings.TrimSpace(c.Request.GetQueryParameter("q", "")))
	page, _ := strconv.Atoi(c.Request.GetQueryParameter("page", "1"))
	size, _ := strconv.Atoi(c.Request.GetQueryParameter("page_size", strconv.Itoa(dashUserPageSizeDefault)))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = dashUserPageSizeDefault
	}
	if size > dashUserPageSizeMax {
		size = dashUserPageSizeMax
	}

	matches := []*models.UserDataResponse{}
	if users := docdb.GetCollection(Database, true, models.NewUserDataResponse); users != nil {
		for _, u := range users.FindAll() {
			if dashIsNullOrWhiteSpaceP(u.UserId) {
				continue // same rule as GetUsers
			}
			if q != "" && !strings.Contains(strings.ToLower(dashDeref(u.Username)), q) && !strings.Contains(strings.ToLower(dashDeref(u.UserId)), q) {
				continue
			}
			matches = append(matches, u)
		}
	}
	dashStableSort(matches, func(a, b *models.UserDataResponse) bool { return a.LastLogin.After(b.LastLogin) })

	total := len(matches)
	pages := (total + size - 1) / size
	if pages == 0 {
		pages = 1
	}
	if page > pages {
		page = pages
	}
	from := (page - 1) * size
	to := from + size
	if to > total {
		to = total
	}

	list := []*DashboardUser{}
	for _, u := range matches[from:to] {
		list = append(list, dashToDashboardUser(u))
	}
	return dashJson(200, stj.O("users", list, "total", total, "page", page, "page_size", size, "pages", pages))
}
