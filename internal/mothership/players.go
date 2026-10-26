// Ports the association / automation-player / link endpoints of
// AUnrealFeatures.AAMothership/MothershipServer.cs: CreateAssociation,
// GetAssociations, DeleteAssociations, AutomationGetPlayers,
// AutomationValidatePlayer, AutomationUpdatePlayerTags, AutomationCreateLink,
// AutomationSetPrimaryLink, AutomationGetLinks, AutomationDeleteLink,
// AutomationAllAssociations, AutomationDeleteAssociation, ExplicitLink,
// BulkPlayerLookup and BulkLinkLookup.
package mothership

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

func associations() *docdb.Collection[models.MothershipAssociationDbObject] {
	return col(models.NewMothershipAssociationDbObject)
}

func links() *docdb.Collection[models.MothershipLinkDbObject] {
	return col(models.NewMothershipLinkDbObject)
}

func (s *MothershipServer) CreateAssociation(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	assoc := models.NewMothershipAssociationDbObject()
	assoc.AssociationId = strp(newId())
	assoc.PlayerId = strp(str(body, "player_id"))
	assoc.Platform = strp(str(body, "platform"))
	assoc.CreatedAt = dotnet.UtcNow()
	associations().Insert(assoc)
	return jsonT(assoc, 200)
}

func (s *MothershipServer) GetAssociations(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	playerId := c.Param("player_id")
	results := associations().Find(func(a *models.MothershipAssociationDbObject) bool {
		return dbEq(a.PlayerId, playerId)
	})
	return jsonAnon(stj.O("Results", results), 200)
}

func (s *MothershipServer) DeleteAssociations(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	playerId := c.Param("player_id")
	associations().DeleteMany(func(a *models.MothershipAssociationDbObject) bool {
		return dbEq(a.PlayerId, playerId)
	})
	return jsonAnon(stj.O("status", "deleted"), 200)
}

func (s *MothershipServer) AutomationGetPlayers(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	results := col(models.NewMothershipPlayerDbObject).FindAll()
	return jsonAnon(stj.O("Results", results), 200)
}

func (s *MothershipServer) AutomationValidatePlayer(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	username := str(body, "username")
	return jsonAnon(stj.O("username", username, "user_id", newId(), "recent_flagged_usernames", []string{}), 200)
}

func (s *MothershipServer) AutomationUpdatePlayerTags(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	playerId := str(body, "player_id")
	tags := strList(body, "tags")
	cl := col(models.NewMothershipPlayerDbObject)
	player := cl.FindOne(func(p *models.MothershipPlayerDbObject) bool {
		return dbEq(p.MothershipPlayerId, playerId)
	})
	if player == nil {
		return errorResult("Player not found", 404)
	}
	player.Tags = strPtrs(tags)
	cl.Update(player)
	return jsonAnon(stj.O("playersToTags", stj.O(playerId, tags)), 200)
}

func (s *MothershipServer) AutomationCreateLink(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	link := models.NewMothershipLinkDbObject()
	link.LinkId = strp(newId())
	link.Platform = strp("")
	link.TargetPlayer = strp(str(body, "TargetPlayer"))
	link.OtherToken = strp(str(body, "OtherToken"))
	link.IsPrimary = false
	link.CreatedAt = dotnet.UtcNow()
	links().Insert(link)
	return jsonT(link, 200)
}

func (s *MothershipServer) AutomationSetPrimaryLink(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	linkId := str(body, "link_id")
	cl := links()
	link := cl.FindOne(func(l *models.MothershipLinkDbObject) bool { return dbEq(l.LinkId, linkId) })
	if link == nil {
		return errorResult("Link not found", 404)
	}
	link.IsPrimary = true
	cl.Update(link)
	return jsonT(link, 200)
}

func (s *MothershipServer) AutomationGetLinks(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	playerId := c.Request.GetQueryParameter("player_id", "")
	result := links().Find(func(l *models.MothershipLinkDbObject) bool { return dbEq(l.TargetPlayer, playerId) })
	return jsonAnon(stj.O("Identities", result), 200)
}

func (s *MothershipServer) AutomationDeleteLink(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	linkId := c.Param("link_id")
	cl := links()
	link := cl.FindOne(func(l *models.MothershipLinkDbObject) bool { return dbEq(l.LinkId, linkId) })
	if link == nil {
		return errorResult("Link not found", 404)
	}
	cl.Delete(link.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

func (s *MothershipServer) AutomationAllAssociations(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	mothershipId := c.Request.GetQueryParameter("mothershipId", "")
	results := associations().Find(func(a *models.MothershipAssociationDbObject) bool {
		return dbEq(a.PlayerId, mothershipId)
	})
	return jsonAnon(stj.O("Results", results), 200)
}

func (s *MothershipServer) AutomationDeleteAssociation(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	assocId := c.Param("assoc_id")
	cl := associations()
	assoc := cl.FindOne(func(a *models.MothershipAssociationDbObject) bool {
		return dbEq(a.AssociationId, assocId)
	})
	if assoc == nil {
		return errorResult("Association not found", 404)
	}
	cl.Delete(assoc.Id)
	return jsonAnon(stj.O("status", "deleted"), 200)
}

// ExplicitLink explicitly links two accounts on a given platform.
func (s *MothershipServer) ExplicitLink(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	link := models.NewMothershipLinkDbObject()
	link.LinkId = strp(newId())
	link.Platform = strp(c.Param("platform"))
	link.TargetPlayer = strp(str(body, "TargetPlayer"))
	link.OtherToken = strp(str(body, "OtherToken"))
	link.IsPrimary = false
	link.CreatedAt = dotnet.UtcNow()
	links().Insert(link)
	return jsonAnon(stj.O("link_id", link.LinkId, "status", "linked"), 200)
}

// BulkPlayerLookup looks up multiple players by their Mothership player IDs.
func (s *MothershipServer) BulkPlayerLookup(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	cl := col(models.NewMothershipPlayerDbObject)
	results := []*models.MothershipPlayerDbObject{}
	if lookupsEl, ok := body.TryGetValue("Lookups"); ok && lookupsEl.ValueKind() == stj.Array {
		for _, lookup := range lookupsEl.EnumerateArray() {
			pid := strProp(lookup, "PlayerId")
			if pid != "" {
				player := cl.FindOne(func(pl *models.MothershipPlayerDbObject) bool {
					return dbEq(pl.MothershipPlayerId, pid)
				})
				if player != nil {
					results = append(results, player)
				}
			}
		}
	}
	return jsonAnon(stj.O("Results", results), 200)
}

// BulkLinkLookup returns all account links.
func (s *MothershipServer) BulkLinkLookup(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", links().FindAll()), 200)
}
