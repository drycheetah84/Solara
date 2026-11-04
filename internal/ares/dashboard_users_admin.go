package ares

// Solara: dashboard Users tab additions.
//
//	GET /api/users/search?q=&page=1&page_size=50 -> {users, total, page, page_size, pages}
//	    Every user (GET /api/users stays the original: the 50 most recent logins), newest login first, a page at a
//	    time. q matches the username or the user id, case-insensitive. Same scoped-token access as GET /api/users.
//	    Each user also carries global_permissions: how many permissions their per-user global role grants.
//	GET /api/users/{user_id}/global_permissions -> {permissions, known}
//	PUT /api/users/{user_id}/global_permissions body {permissions:[...]} -> {permissions, known}
//	    Per-user GLOBAL permissions: a role on station "global" (role id "__global_perms__<user_id>") holding exactly
//	    these permissions, so they apply on every server -- the same mechanism as the global admin role (Grant Admin),
//	    just with a chosen set. An empty list removes the role. Master key only.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

const (
	dashUserPageSizeDefault   = 50
	dashUserPageSizeMax       = 500
	dashGlobalPermsRolePrefix = "__global_perms__"
	dashGlobalPermsMax        = 200
)

var dashPermNameRe = regexp.MustCompile(`^[a-z0-9_:.\-]{1,64}$`)

func dashGlobalPermsRoleId(userId string) string { return dashGlobalPermsRolePrefix + userId }

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

	type searchUser struct {
		DashboardUser
		GlobalPermissions int `json:"global_permissions"`
	}
	list := []*searchUser{}
	for _, u := range matches[from:to] {
		n := 0
		if r := dashGlobalPermsRole(dashDeref(u.UserId)); r != nil && dashListContains(u.Roles, dashDeref(r.RoleId)) {
			n = len(r.Permissions)
		}
		list = append(list, &searchUser{DashboardUser: *dashToDashboardUser(u), GlobalPermissions: n})
	}
	return dashJson(200, stj.O("users", list, "total", total, "page", page, "page_size", size, "pages", pages))
}

func dashGlobalPermsRole(userId string) *models.RoleResponse {
	roles := docdb.GetCollection(Database, true, models.NewRoleResponse)
	if roles == nil {
		return nil
	}
	id := dashGlobalPermsRoleId(userId)
	return roles.FindOne(func(r *models.RoleResponse) bool { return dashLiteEq(r.RoleId, id) })
}

func dashFindUserRow(userId string) (*docdb.Collection[models.UserDataResponse], *models.UserDataResponse) {
	col := docdb.GetCollection(Database, true, models.NewUserDataResponse)
	if col == nil {
		return nil, nil
	}
	return col, col.FindOne(func(u *models.UserDataResponse) bool { return dashLiteEq(u.UserId, userId) })
}

func dashGlobalPermsState(user *models.UserDataResponse) hosting.Result {
	perms := []string{}
	if r := dashGlobalPermsRole(dashDeref(user.UserId)); r != nil && dashListContains(user.Roles, dashDeref(r.RoleId)) {
		for _, p := range r.Permissions {
			if p != nil {
				perms = append(perms, *p)
			}
		}
	}
	return dashJson(200, stj.O("permissions", perms, "known", a2AllPermissions))
}

// GetUserGlobalPermissions is GET /api/users/{user_id}/global_permissions.
func (s *AresDashboardServer) GetUserGlobalPermissions(c *hosting.Context) hosting.Result {
	_, user := dashFindUserRow(c.Param("user_id"))
	if user == nil {
		return dashJson(404, stj.O("error", "user not found"))
	}
	return dashGlobalPermsState(user)
}

// PutUserGlobalPermissions is PUT /api/users/{user_id}/global_permissions.
func (s *AresDashboardServer) PutUserGlobalPermissions(c *hosting.Context) hosting.Result {
	userCol, user := dashFindUserRow(c.Param("user_id"))
	if user == nil {
		return dashJson(404, stj.O("error", "user not found"))
	}
	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || body.Permissions == nil {
		return dashJson(400, stj.O("error", `body must be {"permissions": ["..."]}`))
	}
	perms, seen := []string{}, map[string]bool{}
	for _, p := range body.Permissions {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		if !dashPermNameRe.MatchString(p) {
			return dashJson(400, stj.O("error", "invalid permission name: "+p))
		}
		seen[p] = true
		perms = append(perms, p)
	}
	if len(perms) > dashGlobalPermsMax {
		return dashJson(400, stj.O("error", "too many permissions"))
	}

	userId := dashDeref(user.UserId)
	roleId := dashGlobalPermsRoleId(userId)
	roles := docdb.GetCollection(Database, true, models.NewRoleResponse)
	role := dashGlobalPermsRole(userId)
	if len(perms) == 0 {
		if role != nil && roles != nil {
			roles.Delete(role.Id)
		}
		kept := []*string{}
		for _, r := range user.Roles {
			if r == nil || !strings.EqualFold(*r, roleId) {
				kept = append(kept, r)
			}
		}
		user.Roles = kept
	} else {
		if role == nil {
			role = models.NewRoleResponse()
			role.RoleId = dashStrPtr(roleId)
			role.StationId = dashStrPtr("global")
			role.RoleName = dashStrPtr("Global permissions")
			role.RoleDescription = dashStrPtr("Per-user global permissions set on the dashboard (apply on every server)")
		}
		role.Permissions = []*string{}
		for _, p := range perms {
			role.Permissions = append(role.Permissions, dashStrPtr(p))
		}
		if roles != nil {
			roles.Upsert(role)
		}
		if !dashListContains(user.Roles, roleId) {
			user.Roles = append(user.Roles, dashStrPtr(roleId))
		}
	}
	if user.Roles == nil {
		user.Roles = []*string{}
	}
	userCol.Update(user)
	return dashGlobalPermsState(user)
}
