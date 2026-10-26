// Ports the shared-group endpoints of AUnrealFeatures.AAMothership/MothershipServer.cs:
// ServerSharedGroup, GetSharedGroup, UpdateSharedGroup, DeleteSharedGroup,
// GetSharedGroupData, AddSharedGroupMembers and RemoveSharedGroupMembers.
package mothership

import (
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

func sharedGroups() *docdb.Collection[models.MothershipSharedGroupDbObject] {
	return col(models.NewMothershipSharedGroupDbObject)
}

// findGroup mirrors FindOne(g => g.SharedGroupId == groupId) (LiteDB collation).
func findGroup(cl *docdb.Collection[models.MothershipSharedGroupDbObject], groupId string) *models.MothershipSharedGroupDbObject {
	g := cl.FindOne(func(g *models.MothershipSharedGroupDbObject) bool { return dbEq(g.SharedGroupId, groupId) })
	if g != nil {
		g.NormalizeData()
	}
	return g
}

// ServerSharedGroup creates a shared group (server).
func (s *MothershipServer) ServerSharedGroup(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	action := str(body, "action")
	groupId := ""
	if gid, ok := body.TryGetValue("sharedGroupId"); ok && gid.ValueKind() == stj.String {
		groupId = gid.Str()
	} else {
		groupId = newId()
	}

	if action == "create" || action == "" {
		group := models.NewMothershipSharedGroupDbObject()
		group.SharedGroupId = strp(groupId)
		group.Data = stj.Obj{}
		group.Members = []*string{}
		sharedGroups().Insert(group)
		return jsonAnon(stj.O("SharedGroupId", groupId), 200)
	}
	return errorResult("Invalid action", 400)
}

func (s *MothershipServer) GetSharedGroup(c *hosting.Context) hosting.Result {
	if !isAuthorized(c.Request) {
		return unauthorized()
	}
	group := findGroup(sharedGroups(), c.Param("group_id"))
	if group == nil {
		return errorResult("Shared group not found", 404)
	}
	return jsonT(group, 200)
}

func (s *MothershipServer) UpdateSharedGroup(c *hosting.Context) hosting.Result {
	if !isAuthorized(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	cl := sharedGroups()
	group := findGroup(cl, c.Param("group_id"))
	if group == nil {
		return errorResult("Shared group not found", 404)
	}

	if dataEl, ok := body.TryGetValue("data"); ok && dataEl.ValueKind() == stj.Object {
		for _, kv := range dataEl.EnumerateObject() {
			group.Data.Set(kv.Name, kv.Value.ToString())
		}
	}
	if removeEl, ok := body.TryGetValue("keysToRemove"); ok && removeEl.ValueKind() == stj.Array {
		for _, key := range removeEl.EnumerateArray() {
			k := ""
			if p := key.GetString(); p != nil {
				k = *p
			}
			group.Remove(k)
		}
	}
	cl.Update(group)
	return jsonT(group, 200)
}

func (s *MothershipServer) DeleteSharedGroup(c *hosting.Context) hosting.Result {
	if !isAuthorized(c.Request) {
		return unauthorized()
	}
	cl := sharedGroups()
	group := findGroup(cl, c.Param("group_id"))
	if group == nil {
		return errorResult("Shared group not found", 404)
	}
	cl.Delete(group.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

func (s *MothershipServer) GetSharedGroupData(c *hosting.Context) hosting.Result {
	if !isAuthorized(c.Request) {
		return unauthorized()
	}
	group := findGroup(sharedGroups(), c.Param("group_id"))
	if group == nil {
		return errorResult("Shared group not found", 404)
	}
	return jsonT(group, 200)
}

func (s *MothershipServer) AddSharedGroupMembers(c *hosting.Context) hosting.Result {
	if !isAuthorized(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	cl := sharedGroups()
	group := findGroup(cl, c.Param("group_id"))
	if group == nil {
		return errorResult("Shared group not found", 404)
	}
	group.Members = append(group.Members, strPtrs(strList(body, "members"))...)
	cl.Update(group)
	return jsonT(group, 200)
}

func (s *MothershipServer) RemoveSharedGroupMembers(c *hosting.Context) hosting.Result {
	if !isAuthorized(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	cl := sharedGroups()
	group := findGroup(cl, c.Param("group_id"))
	if group == nil {
		return errorResult("Shared group not found", 404)
	}
	toRemove := strList(body, "members")
	kept := group.Members[:0]
	for _, m := range group.Members {
		if m == nil || !containsStr(toRemove, *m) { // List<string>.Contains(null) is false
			kept = append(kept, m)
		}
	}
	group.Members = kept
	cl.Update(group)
	return jsonT(group, 200)
}
