package ares

// Solara: dashboard endpoints to give and take cosmetics / currency (see mothership/grants.go).
//
//   GET    /api/catalog/items                     -> [{id, currency}]  the grantable 50451 catalogue
//   GET    /api/users/{user_id}/inventory         -> {player_id, full_catalogue, items:[{id, quantity, currency}]}
//   POST   /api/users/{user_id}/inventory         {item_id, quantity}  give (currency: set the balance)
//   DELETE /api/users/{user_id}/inventory/{item_id}                    take away
//
// {user_id} is the dashboard user id (Meta account id) or a Mothership player id. Grants reach the player the
// next time their game logs in (the client reads its inventory at login).

import (
	"strings"

	"solara/internal/hosting"
	"solara/internal/mothership"
	"solara/internal/stj"
)

func (s *AresDashboardServer) GetGrantableItems(c *hosting.Context) hosting.Result {
	return hosting.Ok(mothership.GrantableItems())
}

func (s *AresDashboardServer) GetUserInventory(c *hosting.Context) hosting.Result {
	pid := mothership.ResolvePlayerId(c.Param("user_id"))
	if pid == "" {
		return hosting.Ok(dashActionError("this player has never logged in to the game (no Mothership account yet)"))
	}
	return hosting.Ok(stj.O("player_id", pid, "full_catalogue", mothership.HasFullCatalogue(pid), "items", mothership.OwnedItems(pid)))
}

func (s *AresDashboardServer) GrantUserItem(c *hosting.Context) hosting.Result {
	pid := mothership.ResolvePlayerId(c.Param("user_id"))
	if pid == "" {
		return hosting.Ok(dashActionError("this player has never logged in to the game (no Mothership account yet)"))
	}
	body, err := stj.Parse(c.Request.Body())
	if err != nil {
		return hosting.Ok(dashActionError("body must be JSON {item_id, quantity}"))
	}
	item := ""
	if v, ok := body.TryGetProperty("item_id"); ok && v.ValueKind() == stj.String {
		item = strings.TrimSpace(*v.GetString())
	}
	if item == "" || !mothership.IsGrantable(item) {
		return hosting.Ok(dashActionError("unknown item_id (not in the 50451 item catalogue)"))
	}
	q := 1
	if v, ok := body.TryGetProperty("quantity"); ok && v.ValueKind() == stj.Number {
		q = int(v.GetInt32())
	}
	if q < 0 || q > 2000000000 {
		return hosting.Ok(dashActionError("quantity must be between 0 and 2000000000"))
	}
	mothership.GrantItem(pid, item, q)
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(item)})
}

func (s *AresDashboardServer) RevokeUserItem(c *hosting.Context) hosting.Result {
	pid := mothership.ResolvePlayerId(c.Param("user_id"))
	if pid == "" {
		return hosting.Ok(dashActionError("this player has never logged in to the game (no Mothership account yet)"))
	}
	if !mothership.RevokeItem(pid, c.Param("item_id")) {
		return hosting.Ok(dashActionError("the player has no granted copy of that item"))
	}
	return hosting.Ok(&DashboardActionResult{Success: true, Id: dashStrPtr(c.Param("item_id"))})
}
