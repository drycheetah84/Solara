// Ports the progression track / tree endpoints of
// AUnrealFeatures.AAMothership/MothershipServer.cs: CreateProgression,
// GetProgression, DeleteProgression, AutomationCreateProgressionTrack,
// AutomationGetProgressionTracks, AutomationDeleteProgressionTrack,
// AutomationCreateProgressionBinding, AutomationGetProgressionBindings,
// ServerProgression, CreateProgressionTree, GetProgressionTree,
// DeleteProgressionTree, AutomationCreateProgressionTree,
// AutomationGetProgressionTrees, AutomationDeleteProgressionTree,
// AutomationCreateTreeBinding, AutomationGetTreeBindings and
// AutomationDeleteTreeBinding.
package mothership

import (
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

func tracks() *docdb.Collection[models.MothershipProgressionTrackDbObject] {
	return col(models.NewMothershipProgressionTrackDbObject)
}

func trackBindings() *docdb.Collection[models.MothershipProgressionTrackBindingDbObject] {
	return col(models.NewMothershipProgressionTrackBindingDbObject)
}

func trees() *docdb.Collection[models.MothershipProgressionTreeDbObject] {
	return col(models.NewMothershipProgressionTreeDbObject)
}

func treeBindings() *docdb.Collection[models.MothershipProgressionTreeBindingDbObject] {
	return col(models.NewMothershipProgressionTreeBindingDbObject)
}

func newTrack(body jsonBody) *models.MothershipProgressionTrackDbObject {
	track := models.NewMothershipProgressionTrackDbObject()
	track.TrackId = strp(newId())
	track.Name = strp(str(body, "name"))
	track.LevelsJson = strp(elementOr(body, "levels", "[]"))
	track.TriggersJson = strp(elementOr(body, "triggers", "[]"))
	return track
}

func deleteTrack(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	trackId := c.Request.GetQueryParameter("track_id", "")
	cl := tracks()
	track := cl.FindOne(func(t *models.MothershipProgressionTrackDbObject) bool { return dbEq(t.TrackId, trackId) })
	if track == nil {
		return errorResult("Track not found", 404)
	}
	cl.Delete(track.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

// CreateProgression creates a new progression track.
func (s *MothershipServer) CreateProgression(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	track := newTrack(parseBody(c.Request))
	tracks().Insert(track)
	return jsonT(track, 200)
}

func (s *MothershipServer) GetProgression(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	trackId, _ := c.Request.Query("track_id")
	cl := tracks()
	if trackId != "" {
		track := cl.FindOne(func(t *models.MothershipProgressionTrackDbObject) bool { return dbEq(t.TrackId, trackId) })
		if track != nil {
			return jsonT(track, 200)
		}
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

func (s *MothershipServer) DeleteProgression(c *hosting.Context) hosting.Result {
	return deleteTrack(c)
}

// AutomationCreateProgressionTrack creates a progression track, level or trigger.
func (s *MothershipServer) AutomationCreateProgressionTrack(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	action := str(body, "action")
	if action == "create_level" {
		return jsonAnon(stj.O("level_id", newId()), 200)
	}
	if action == "create_trigger" {
		return jsonAnon(stj.O("trigger_id", newId()), 200)
	}
	track := newTrack(body)
	tracks().Insert(track)
	return jsonT(track, 200)
}

func (s *MothershipServer) AutomationGetProgressionTracks(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", tracks().FindAll()), 200)
}

func (s *MothershipServer) AutomationDeleteProgressionTrack(c *hosting.Context) hosting.Result {
	return deleteTrack(c)
}

func (s *MothershipServer) AutomationCreateProgressionBinding(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	binding := models.NewMothershipProgressionTrackBindingDbObject()
	binding.BindingId = strp(newId())
	binding.TrackId = strp(str(body, "track_id"))
	binding.DeploymentId = strp(str(body, "deployment_id"))
	trackBindings().Insert(binding)
	return jsonT(binding, 200)
}

func (s *MothershipServer) AutomationGetProgressionBindings(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", trackBindings().FindAll()), 200)
}

// ServerProgression records additional progress for a player on a track (echo only).
func (s *MothershipServer) ServerProgression(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	playerId := str(body, "PlayerId")
	trackId := str(body, "TrackId")
	additionalProgress := intOr(body, "AdditionalProgress", 0)
	return jsonAnon(stj.O(
		"player_id", playerId,
		"track_id", trackId,
		"progress", additionalProgress,
		"updated_at", utcNowO(),
	), 200)
}

// ---- PROGRESSION TREES ------------------------------------------------------

func newTree(body jsonBody) *models.MothershipProgressionTreeDbObject {
	tree := models.NewMothershipProgressionTreeDbObject()
	tree.TreeId = strp(newId())
	tree.Name = strp(str(body, "name"))
	tree.NodesJson = strp(elementOr(body, "nodes", "[]"))
	return tree
}

func deleteTree(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	treeId := c.Request.GetQueryParameter("tree_id", "")
	cl := trees()
	tree := cl.FindOne(func(t *models.MothershipProgressionTreeDbObject) bool { return dbEq(t.TreeId, treeId) })
	if tree == nil {
		return errorResult("Tree not found", 404)
	}
	cl.Delete(tree.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

func (s *MothershipServer) CreateProgressionTree(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	tree := newTree(parseBody(c.Request))
	trees().Insert(tree)
	return jsonT(tree, 200)
}

func (s *MothershipServer) GetProgressionTree(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	treeId, _ := c.Request.Query("tree_id")
	cl := trees()
	if treeId != "" {
		tree := cl.FindOne(func(t *models.MothershipProgressionTreeDbObject) bool { return dbEq(t.TreeId, treeId) })
		if tree != nil {
			return jsonT(tree, 200)
		}
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

func (s *MothershipServer) DeleteProgressionTree(c *hosting.Context) hosting.Result {
	return deleteTree(c)
}

// AutomationCreateProgressionTree creates a progression tree, node or lock action.
func (s *MothershipServer) AutomationCreateProgressionTree(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	action := str(body, "action")
	if action == "create_node" {
		return jsonAnon(stj.O("node_id", newId()), 200)
	}
	if action == "lock_node" {
		return jsonAnon(stj.O("status", "locked"), 200)
	}
	tree := newTree(body)
	trees().Insert(tree)
	return jsonT(tree, 200)
}

func (s *MothershipServer) AutomationGetProgressionTrees(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", trees().FindAll()), 200)
}

func (s *MothershipServer) AutomationDeleteProgressionTree(c *hosting.Context) hosting.Result {
	return deleteTree(c)
}

func (s *MothershipServer) AutomationCreateTreeBinding(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	binding := models.NewMothershipProgressionTreeBindingDbObject()
	binding.BindingId = strp(newId())
	binding.TreeId = strp(str(body, "tree_id"))
	binding.DeploymentId = strp(str(body, "deployment_id"))
	treeBindings().Insert(binding)
	return jsonT(binding, 200)
}

func (s *MothershipServer) AutomationGetTreeBindings(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	bindingId, _ := c.Request.Query("binding_id")
	cl := treeBindings()
	if bindingId != "" {
		b := cl.FindOne(func(x *models.MothershipProgressionTreeBindingDbObject) bool {
			return dbEq(x.BindingId, bindingId)
		})
		if b != nil {
			return jsonT(b, 200)
		}
	}
	return jsonAnon(stj.O("Results", cl.FindAll()), 200)
}

func (s *MothershipServer) AutomationDeleteTreeBinding(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	bindingId := c.Request.GetQueryParameter("binding_id", "")
	cl := treeBindings()
	binding := cl.FindOne(func(b *models.MothershipProgressionTreeBindingDbObject) bool {
		return dbEq(b.BindingId, bindingId)
	})
	if binding == nil {
		return errorResult("Binding not found", 404)
	}
	cl.Delete(binding.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}
