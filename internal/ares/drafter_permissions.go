// Ports AUnrealFeatures.Ares/Servers/DrafterPermissions.cs: the DrafterGrant
// stored model, the static DrafterPermissions helper, and the
// A2StationDbServer partial (GET /v1/spec/drafters/{deployment_id}, the
// ServerDrafters handler, registered in a2station_server.go). The
// AresDashboardServer partial of the same file is ported in
// dashboard_drafters.go.
//
// DrafterPermissions statics are package-level identifiers prefixed with the
// class name (DrafterPermissionsApplies, ...ValidId, ...ScopeExists, ...List,
// ...Set, ...Effective): List clashes with NetvarDumpStore / SpecLevelsStore,
// and Set/Applies are too generic for the shared package.
package ares

import (
	"sort"
	"strings"
	"sync"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
)

// DrafterGrant mirrors DrafterGrant : AstraDbObject (stored collection "DrafterGrant").
//
// UserId/Scope stay plain strings although LiteDB stores "" / whitespace-only
// strings as null (read back as null) and trims: the only writer is
// DrafterPermissions.Set, reached only from ChangeDrafter after
// ValidId(scope) && ValidId(user) — 1..128 chars of [A-Za-z0-9_-] — so a
// stored value is never empty, never padded, and round-trips unchanged.
type DrafterGrant struct {
	docdb.AstraDbObject
	UserId    string          `json:"user_id"`
	Scope     string          `json:"scope"`
	GrantedAt dotnet.DateTime `json:"granted_at"`
}

// NewDrafterGrant applies the C# initializers (GrantedAt = DateTime.UtcNow).
func NewDrafterGrant() *DrafterGrant {
	return &DrafterGrant{UserId: "", Scope: "", GrantedAt: dotnet.UtcNow()}
}

var a2DrafterGate sync.Mutex

// DrafterPermissionsApplies mirrors DrafterPermissions.Applies(scope, station).
func DrafterPermissionsApplies(scope, station string) bool {
	return !a2IsBlank(station) && (scope == "global" || scope == station)
}

// DrafterPermissionsValidId mirrors DrafterPermissions.ValidId: 1..128 chars
// of [A-Za-z0-9_-] (UTF-16 length).
func DrafterPermissionsValidId(id string) bool {
	n := a2UTF16Len(id)
	if n <= 0 || n > 128 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// DrafterPermissionsScopeExists mirrors DrafterPermissions.ScopeExists.
func DrafterPermissionsScopeExists(scope string) bool {
	if scope == "global" {
		return true
	}
	col := docdb.GetCollection[models.StationDbObject](Database, true, models.NewStationDbObject)
	return col != nil && col.FindOne(func(s *models.StationDbObject) bool { return a2EqFoldS(s.StationId, scope) }) != nil
}

// DrafterPermissionsList mirrors DrafterPermissions.List().
func DrafterPermissionsList() []*DrafterGrant {
	a2DrafterGate.Lock()
	defer a2DrafterGate.Unlock()
	return docdb.GetCollection[DrafterGrant](Database, true, NewDrafterGrant).FindAll()
}

// DrafterPermissionsSet mirrors DrafterPermissions.Set(scope, user, enabled).
func DrafterPermissionsSet(scope, user string, enabled bool) {
	a2DrafterGate.Lock()
	defer a2DrafterGate.Unlock()
	col := docdb.GetCollection[DrafterGrant](Database, true, NewDrafterGrant)
	matches := col.Find(func(g *DrafterGrant) bool { return docdb.EqFold(g.Scope, scope) && docdb.EqFold(g.UserId, user) })
	if enabled && len(matches) == 0 {
		g := NewDrafterGrant()
		g.Scope, g.UserId = scope, user
		col.Insert(g)
	}
	if !enabled {
		for _, item := range matches {
			col.Delete(item.Id)
		}
	}
}

// DrafterPermissionsEffective mirrors DrafterPermissions.Effective(station):
// applicable valid user ids, distinct (OrdinalIgnoreCase, first kept), then
// OrderBy(x => x) with the invariant-mode comparer (ordinal).
func DrafterPermissionsEffective(station string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, g := range DrafterPermissionsList() {
		if !DrafterPermissionsApplies(g.Scope, station) || !DrafterPermissionsValidId(g.UserId) {
			continue
		}
		k := strings.ToUpper(g.UserId)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, g.UserId)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ServerDrafters mirrors A2StationDbServer.ServerDrafters
// (GET /v1/spec/drafters/{deployment_id}). The read-only permission feed is
// public; the scope is resolved from the registered deployment, never a
// station claimed by the editor client.
func (s *A2StationDbServer) ServerDrafters(c *hosting.Context) hosting.Result {
	deploymentId := c.Param("deployment_id")
	dep := docdb.GetCollection[models.DeploymentDbObject](Database, true, models.NewDeploymentDbObject).
		FindOne(func(d *models.DeploymentDbObject) bool { return a2EqFoldS(d.DeploymentId, deploymentId) })
	if dep == nil || a2IsNullOrWhiteSpace(dep.StationId) || !DrafterPermissionsScopeExists(*dep.StationId) {
		return a2SpecErr(404, "Unknown deployment or station")
	}
	seen := map[string]bool{}
	lines := []string{}
	for _, g := range DrafterPermissionsList() {
		if !DrafterPermissionsApplies(g.Scope, *dep.StationId) || !DrafterPermissionsValidId(g.UserId) {
			continue
		}
		prefix := "S|"
		if g.Scope == "global" {
			prefix = "G|"
		}
		line := prefix + g.UserId
		if seen[line] { // Distinct() (ordinal)
			continue
		}
		seen[line] = true
		lines = append(lines, line)
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i] < lines[j] })
	return hosting.Configurable(200, "text/plain", "halcyon-drafters-v1\n"+strings.Join(lines, "\n")+"\n")
}
