// Ports the `public sealed partial class AresDashboardServer` block of
// AUnrealFeatures.Ares/Servers/DrafterPermissions.cs: the admin-only Drafter
// (LE access) grant endpoints. The DrafterPermissions helper class itself and
// DrafterGrant are ported separately (DrafterPermissions* identifiers).
package ares

import (
	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/logging"
	"solara/internal/stj"
)

// dashboardDrafterError mirrors the private static DrafterError.
func dashboardDrafterError(status int, err string) hosting.Result {
	return hosting.Configurable(status, "application/json", stj.MarshalString(stj.O("error", err)))
}

func (s *AresDashboardServer) GetDrafters(c *hosting.Context) hosting.Result {
	scope := c.Param("scope")
	if !DashboardAuthPreprocessorHasValidAdminKey(c.Request) {
		return dashboardDrafterError(403, "Dashboard administrator required")
	}
	if !DrafterPermissionsValidId(scope) || !DrafterPermissionsScopeExists(scope) {
		return dashboardDrafterError(404, "Unknown scope")
	}
	accounts := []stj.Obj{}
	for _, u := range docdb.GetCollection(Database, true, models.NewUserDataResponse).FindAll() {
		if !DrafterPermissionsValidId(dashDeref(u.UserId)) {
			continue
		}
		username := u.Username
		if username == nil {
			username = u.UserId
		}
		accounts = append(accounts, stj.O("user_id", u.UserId, "username", username))
	}
	grants := []*DrafterGrant{}
	for _, g := range DrafterPermissionsList() {
		if g.Scope == scope || (scope != "global" && g.Scope == "global") {
			grants = append(grants, g)
		}
	}
	return hosting.Ok(stj.O("scope", scope, "role", "Drafter", "accounts", accounts, "grants", grants))
}

func (s *AresDashboardServer) GrantDrafter(c *hosting.Context) hosting.Result {
	return dashboardChangeDrafter(c.Request, c.Param("scope"), c.Param("user_id"), true)
}

func (s *AresDashboardServer) RevokeDrafter(c *hosting.Context) hosting.Result {
	return dashboardChangeDrafter(c.Request, c.Param("scope"), c.Param("user_id"), false)
}

// dashboardChangeDrafter mirrors the private static ChangeDrafter.
func dashboardChangeDrafter(request *hosting.Request, scope, user string, enabled bool) hosting.Result {
	if !DashboardAuthPreprocessorHasValidAdminKey(request) {
		return dashboardDrafterError(403, "Dashboard administrator required")
	}
	if !DrafterPermissionsValidId(scope) || !DrafterPermissionsScopeExists(scope) {
		return dashboardDrafterError(404, "Unknown scope")
	}
	if !DrafterPermissionsValidId(user) || (enabled && docdb.GetCollection(Database, true, models.NewUserDataResponse).
		FindOne(func(u *models.UserDataResponse) bool { return dashLiteEq(u.UserId, user) }) == nil) {
		return dashboardDrafterError(400, "Select a known user")
	}
	DrafterPermissionsSet(scope, user, enabled)
	action := "revoke"
	if enabled {
		action = "grant"
	}
	logging.Console("[DRAFTERS] " + action + " scope=" + scope + " user=" + user)
	return hosting.Ok(stj.O("success", true))
}
