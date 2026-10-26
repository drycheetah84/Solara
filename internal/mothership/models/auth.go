// Ports Models/MothershipPlayerDbObject.cs, MothershipSessionDbObject.cs,
// MothershipV2PlayerDbObject.cs, MothershipAssociationDbObject.cs,
// MothershipLinkDbObject.cs and MothershipApiKeyDbObject.cs.
package models

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

// MothershipPlayerDbObject is a v1 player record.
type MothershipPlayerDbObject struct {
	docdb.AstraDbObject
	MothershipPlayerId      *string   `json:"MothershipPlayerId"`
	ExternalAccountNickname *string   `json:"ExternalAccountNickname"`
	ExternalAccountId       *string   `json:"ExternalAccountId"`
	ExpirationTime          int64     `json:"ExpirationTime"`
	Tags                    []*string `json:"Tags"`
	Token                   *string   `json:"Token"`
}

func NewMothershipPlayerDbObject() *MothershipPlayerDbObject {
	o := &MothershipPlayerDbObject{Tags: []*string{}}
	initStrings(o)
	return o
}

// MothershipSessionDbObject maps a session token to a player id.
type MothershipSessionDbObject struct {
	docdb.AstraDbObject
	Token    *string `json:"Token"`
	PlayerId *string `json:"PlayerId"`
}

func NewMothershipSessionDbObject() *MothershipSessionDbObject {
	o := &MothershipSessionDbObject{}
	initStrings(o)
	return o
}

// MothershipV2PlayerDbObject is a v2 (QUEST/RIFT/STEAM) player record.
type MothershipV2PlayerDbObject struct {
	docdb.AstraDbObject
	ExternalProviderId       *string   `json:"ExternalProviderId"`
	ExternalProviderUsername *string   `json:"ExternalProviderUsername"`
	IsPrimaryId              bool      `json:"IsPrimaryId"`
	PlayerId                 *string   `json:"PlayerId"`
	Tags                     []*string `json:"Tags"` // List<string>? (nil = null)
	Token                    *string   `json:"Token"`
	ExpirationTime           int64     `json:"ExpirationTime"`
	// Stored in DB for filtering, not returned to clients ([JsonIgnore]).
	ExternalService *string `json:"-"`
}

func NewMothershipV2PlayerDbObject() *MothershipV2PlayerDbObject {
	o := &MothershipV2PlayerDbObject{IsPrimaryId: true}
	initStrings(o)
	return o
}

// MothershipAssociationDbObject is a platform association of a player.
type MothershipAssociationDbObject struct {
	docdb.AstraDbObject
	AssociationId *string         `json:"association_id"`
	PlayerId      *string         `json:"player_id"`
	Platform      *string         `json:"platform"`
	CreatedAt     dotnet.DateTime `json:"created_at"`
}

func NewMothershipAssociationDbObject() *MothershipAssociationDbObject {
	o := &MothershipAssociationDbObject{}
	initStrings(o)
	return o
}

// MothershipLinkDbObject is an account link.
type MothershipLinkDbObject struct {
	docdb.AstraDbObject
	LinkId       *string         `json:"link_id"`
	Platform     *string         `json:"platform"`
	TargetPlayer *string         `json:"target_player"`
	OtherToken   *string         `json:"other_token"`
	IsPrimary    bool            `json:"is_primary"`
	CreatedAt    dotnet.DateTime `json:"created_at"`
}

func NewMothershipLinkDbObject() *MothershipLinkDbObject {
	o := &MothershipLinkDbObject{}
	initStrings(o)
	return o
}

// MothershipApiKeyDbObject is a server API key created through automation.
type MothershipApiKeyDbObject struct {
	docdb.AstraDbObject
	KeyId     *string `json:"key_id"`
	ApiKey    *string `json:"api_key"`
	Name      *string `json:"name"`
	CreatedAt *string `json:"created_at"`
}

func NewMothershipApiKeyDbObject() *MothershipApiKeyDbObject {
	o := &MothershipApiKeyDbObject{}
	initStrings(o)
	return o
}
