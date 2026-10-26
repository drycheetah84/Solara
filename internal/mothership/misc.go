// Ports the remaining endpoints of AUnrealFeatures.AAMothership/MothershipServer.cs:
// PlatformAnalyticsBatch, ClientPermissions, ServerPermissions, CreateApiKey,
// GetApiKeys, ServerSendNotification, Index, Health, DebugStorage, GetStorage,
// CreateStorageItem, GetFriendsV2, RequestFriend, RemoveFriend and SetPrivacyState.
package mothership

import (
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/mothership/models"
	"solara/internal/stj"
)

// PlatformAnalyticsBatch receives and stores an analytics event batch for any platform.
func (s *MothershipServer) PlatformAnalyticsBatch(c *hosting.Context) hosting.Result {
	platform := c.Param("platform")
	body := parseBody(c.Request)
	cl := col(models.NewMothershipAnalyticsEventDbObject)
	events := 0
	if eventsEl, ok := body.TryGetValue("events"); ok && eventsEl.ValueKind() == stj.Array {
		for _, evt := range eventsEl.EnumerateArray() {
			e := models.NewMothershipAnalyticsEventDbObject()
			e.EventId = strp(newId())
			e.Platform = strp(platform)
			e.Timestamp = strp(utcNowO())
			e.DataJson = strp(evt.ToString())
			cl.Insert(e)
			events++
		}
	}
	return jsonAnon(stj.O("status", "success", "events_processed", events, "timestamp", utcNowO()), 200)
}

func (s *MothershipServer) ClientPermissions(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("permissions", []string{"read", "write", "execute"}, "roles", []string{"player"}), 200)
}

func (s *MothershipServer) ServerPermissions(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("permissions", []string{"admin", "read", "write", "execute", "delete"}, "roles", []string{"server", "admin"}), 200)
}

func (s *MothershipServer) CreateApiKey(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	keyObj := models.NewMothershipApiKeyDbObject()
	keyObj.KeyId = strp(newId())
	keyObj.ApiKey = strp(newId())
	keyObj.Name = strp(str(body, "name"))
	keyObj.CreatedAt = strp(utcNowO())
	col(models.NewMothershipApiKeyDbObject).Insert(keyObj)
	return jsonT(keyObj, 200)
}

func (s *MothershipServer) GetApiKeys(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	return jsonAnon(stj.O("Results", col(models.NewMothershipApiKeyDbObject).FindAll()), 200)
}

func (s *MothershipServer) ServerSendNotification(c *hosting.Context) hosting.Result {
	if !isAdmin(c.Request) {
		return unauthorized()
	}
	body := parseBody(c.Request)
	n := models.NewMothershipNotificationDbObject()
	n.NotificationId = strp(newId())
	n.PlayerId = strp(str(body, "player_id"))
	n.Message = strp(str(body, "message"))
	n.Type = strp(str(body, "type"))
	n.SentAt = strp(utcNowO())
	col(models.NewMothershipNotificationDbObject).Insert(n)
	return jsonT(n, 200)
}

// Index returns basic service info.
func (s *MothershipServer) Index(c *hosting.Context) hosting.Result {
	return jsonAnon(stj.O(
		"service", "Mothership API",
		"version", "1.0.0",
		"status", "running",
		"sdk_version", "v2025.11.24.1",
	), 200)
}

// Health returns the health status and current timestamp.
func (s *MothershipServer) Health(c *hosting.Context) hosting.Result {
	return jsonAnon(stj.O("status", "healthy", "timestamp", utcNowO()), 200)
}

// DebugStorage returns item counts across all storage collections (no auth).
func (s *MothershipServer) DebugStorage(c *hosting.Context) hosting.Result {
	return jsonAnon(stj.O(
		"players", col(models.NewMothershipPlayerDbObject).Count(nil),
		"sessions", col(models.NewMothershipSessionDbObject).Count(nil),
		"bans", col(models.NewMothershipBanDbObject).Count(nil),
		"mutes", col(models.NewMothershipMuteDbObject).Count(nil),
		"reports", col(models.NewMothershipReportDbObject).Count(nil),
		"shared_groups", col(models.NewMothershipSharedGroupDbObject).Count(nil),
		"titles", col(models.NewMothershipTitleDbObject).Count(nil),
		"environments", col(models.NewMothershipEnvironmentDbObject).Count(nil),
		"deployments", col(models.NewMothershipDeploymentDbObject).Count(nil),
		"user_data", col(models.NewMothershipUserDataDbObject).Count(nil),
		"title_data", col(models.NewMothershipTitleDataDbObject).Count(nil),
		"offers", col(models.NewMothershipOfferDbObject).Count(nil),
		"offer_bindings", col(models.NewMothershipOfferBindingDbObject).Count(nil),
		"offer_displays", col(models.NewMothershipOfferDisplayDbObject).Count(nil),
		"entitlements", col(models.NewMothershipEntitlementDbObject).Count(nil),
		"transactions", col(models.NewMothershipTransactionCatalogDbObject).Count(nil),
		"inventory", col(models.NewMothershipInventoryItemDbObject).Count(nil),
		"progression_tracks", col(models.NewMothershipProgressionTrackDbObject).Count(nil),
		"progression_trees", col(models.NewMothershipProgressionTreeDbObject).Count(nil),
		"api_keys", col(models.NewMothershipApiKeyDbObject).Count(nil),
		"notifications", col(models.NewMothershipNotificationDbObject).Count(nil),
		"analytics_events", col(models.NewMothershipAnalyticsEventDbObject).Count(nil),
	), 200)
}

// GetStorage returns all items in a named storage collection (no auth).
func (s *MothershipServer) GetStorage(c *hosting.Context) hosting.Result {
	var data any
	switch c.Param("key") {
	case "players":
		data = col(models.NewMothershipPlayerDbObject).FindAll()
	case "sessions":
		data = col(models.NewMothershipSessionDbObject).FindAll()
	case "account_associations":
		data = col(models.NewMothershipAssociationDbObject).FindAll()
	case "account_links":
		data = col(models.NewMothershipLinkDbObject).FindAll()
	case "bans":
		data = col(models.NewMothershipBanDbObject).FindAll()
	case "mutes":
		data = col(models.NewMothershipMuteDbObject).FindAll()
	case "reports":
		data = col(models.NewMothershipReportDbObject).FindAll()
	case "shared_groups":
		groups := col(models.NewMothershipSharedGroupDbObject).FindAll()
		for _, g := range groups {
			g.NormalizeData()
		}
		data = groups
	case "titles":
		data = col(models.NewMothershipTitleDbObject).FindAll()
	case "environments":
		data = col(models.NewMothershipEnvironmentDbObject).FindAll()
	case "deployments":
		data = col(models.NewMothershipDeploymentDbObject).FindAll()
	case "user_data":
		data = col(models.NewMothershipUserDataDbObject).FindAll()
	case "user_data_metadata":
		data = col(models.NewMothershipUserDataMetadataDbObject).FindAll()
	case "title_data":
		data = col(models.NewMothershipTitleDataDbObject).FindAll()
	case "offers":
		data = col(models.NewMothershipOfferDbObject).FindAll()
	case "offer_bindings":
		data = col(models.NewMothershipOfferBindingDbObject).FindAll()
	case "offer_displays":
		data = col(models.NewMothershipOfferDisplayDbObject).FindAll()
	case "entitlements":
		data = col(models.NewMothershipEntitlementDbObject).FindAll()
	case "transactions":
		data = col(models.NewMothershipTransactionCatalogDbObject).FindAll()
	case "inventory":
		data = col(models.NewMothershipInventoryItemDbObject).FindAll()
	case "progression_tracks":
		data = col(models.NewMothershipProgressionTrackDbObject).FindAll()
	case "progression_track_bindings":
		data = col(models.NewMothershipProgressionTrackBindingDbObject).FindAll()
	case "progression_trees":
		data = col(models.NewMothershipProgressionTreeDbObject).FindAll()
	case "progression_tree_bindings":
		data = col(models.NewMothershipProgressionTreeBindingDbObject).FindAll()
	case "api_keys":
		data = col(models.NewMothershipApiKeyDbObject).FindAll()
	case "notifications":
		data = col(models.NewMothershipNotificationDbObject).FindAll()
	case "analytics_events":
		data = col(models.NewMothershipAnalyticsEventDbObject).FindAll()
	case "friends":
		data = col(models.NewMothershipFriendEntryDbObject).FindAll()
	case "privacy_states":
		data = col(models.NewMothershipPrivacyStateDbObject).FindAll()
	}
	if data == nil {
		return errorResult("Invalid storage key", 400)
	}
	return jsonAnon(data, 200)
}

// CreateStorageItem: generic create, returns 201 with the posted body and a
// generated id (both branches of the original are identical).
func (s *MothershipServer) CreateStorageItem(c *hosting.Context) hosting.Result {
	body := parseBody(c.Request)
	if _, ok := body.TryGetValue("id"); !ok {
		return jsonAnon(stj.O("id", newId(), "data", body), 201)
	}
	return jsonAnon(stj.O("id", newId(), "data", body), 201)
}

// ---- FRIENDS ----------------------------------------------------------------

func friendsCol() *docdb.Collection[models.MothershipFriendEntryDbObject] {
	return col(models.NewMothershipFriendEntryDbObject)
}

func privacyCol() *docdb.Collection[models.MothershipPrivacyStateDbObject] {
	return col(models.NewMothershipPrivacyStateDbObject)
}

// GetFriendsV2 returns a player's friend list and privacy state.
func (s *MothershipServer) GetFriendsV2(c *hosting.Context) hosting.Result {
	body := parseBody(c.Request)
	playFabId := str(body, "PlayFabId")
	if playFabId == "" {
		return jsonAnon(stj.O("StatusCode", 400, "Error", "PlayFabId is required"), 400)
	}

	friends := friendsCol().Find(func(f *models.MothershipFriendEntryDbObject) bool {
		return dbEq(f.OwnerId, playFabId)
	})
	privacyRow := privacyCol().FindOne(func(p *models.MothershipPrivacyStateDbObject) bool {
		return dbEq(p.PlayFabId, playFabId)
	})
	privacyState := 0
	if privacyRow != nil {
		privacyState = privacyRow.PrivacyState
	}

	friendList := []any{}
	for _, f := range friends {
		friendList = append(friendList, stj.O(
			"Presence", stj.O(
				"FriendLinkId", f.FriendLinkId,
				"UserName", f.UserName,
				"RoomId", f.RoomId,
				"Zone", f.Zone,
				"Region", f.Region,
				"IsPublic", f.IsPublic,
			),
			"Created", f.Created,
		))
	}

	return jsonAnon(stj.O(
		"Result", stj.O("Friends", friendList, "MyPrivacyState", privacyState),
		"StatusCode", 200,
		"Error", nil,
	), 200)
}

// RequestFriend adds a friend entry for a player.
func (s *MothershipServer) RequestFriend(c *hosting.Context) hosting.Result {
	body := parseBody(c.Request)
	playFabId := str(body, "PlayFabId")
	myFriendLinkId := str(body, "MyFriendLinkId")
	friendFriendLinkId := str(body, "FriendFriendLinkId")

	if playFabId == "" || myFriendLinkId == "" || friendFriendLinkId == "" {
		return errorResult("Missing required fields", 400)
	}

	cl := friendsCol()
	existing := cl.FindOne(func(f *models.MothershipFriendEntryDbObject) bool {
		return dbEq(f.OwnerId, playFabId) && dbEq(f.FriendLinkId, friendFriendLinkId)
	})
	if existing != nil {
		return errorResult("Already friends", 409)
	}

	entry := models.NewMothershipFriendEntryDbObject()
	entry.OwnerId = strp(playFabId)
	entry.FriendLinkId = strp(friendFriendLinkId)
	entry.UserName = strp("Player_" + leftUTF16(friendFriendLinkId, 8))
	entry.RoomId = strp("")
	entry.Zone = strp("")
	entry.Region = strp("")
	entry.IsPublic = true
	entry.Created = strp(utcNowO())
	cl.Insert(entry)

	return jsonAnon(stj.O("success", true), 200)
}

// RemoveFriend removes a friend entry from a player's friend list.
func (s *MothershipServer) RemoveFriend(c *hosting.Context) hosting.Result {
	body := parseBody(c.Request)
	playFabId := str(body, "PlayFabId")
	friendFriendLinkId := str(body, "FriendFriendLinkId")

	if playFabId == "" || friendFriendLinkId == "" {
		return errorResult("Missing required fields", 400)
	}

	cl := friendsCol()
	friend := cl.FindOne(func(f *models.MothershipFriendEntryDbObject) bool {
		return dbEq(f.OwnerId, playFabId) && dbEq(f.FriendLinkId, friendFriendLinkId)
	})
	if friend != nil {
		cl.Delete(friend.Id)
	}
	return jsonAnon(stj.O("success", true), 200)
}

// SetPrivacyState sets the privacy visibility state for a player.
func (s *MothershipServer) SetPrivacyState(c *hosting.Context) hosting.Result {
	body := parseBody(c.Request)
	playFabId := str(body, "PlayFabId")
	privacyStateStr := str(body, "PrivacyState")

	if playFabId == "" {
		return jsonAnon(stj.O("StatusCode", 400, "Error", "PlayFabId is required"), 400)
	}

	privacyState := 0
	switch privacyStateStr {
	case "PUBLIC_ONLY":
		privacyState = 1
	case "HIDDEN":
		privacyState = 2
	}

	cl := privacyCol()
	existing := cl.FindOne(func(p *models.MothershipPrivacyStateDbObject) bool {
		return dbEq(p.PlayFabId, playFabId)
	})
	if existing != nil {
		existing.PrivacyState = privacyState
		cl.Update(existing)
	} else {
		ps := models.NewMothershipPrivacyStateDbObject()
		ps.PlayFabId = strp(playFabId)
		ps.PrivacyState = privacyState
		cl.Insert(ps)
	}
	return jsonAnon(stj.O("StatusCode", 200, "Error", nil), 200)
}
