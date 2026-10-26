// Ports AUnrealFeatures.Ares/Models: UserData.cs, UserDataResponse.cs,
// UserDataV1Response.cs, UserRequest.cs, UserRoleByNameRequest.cs,
// UserRoleByNameResponse.cs, UsersResponse.cs, UserApiKeyDbObject.cs,
// ApiKeyResponse.cs, BanRequest.cs, BanResponse.cs, StationPerm.cs.
package models

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

// UserData mirrors UserData : AstraDbObject ([DataMember] on every property).
type UserData struct {
	docdb.AstraDbObject
	UserId    *string         `json:"user_id"`
	Username  *string         `json:"username"`
	DiscordId *string         `json:"discord_id"`
	Platform  *string         `json:"platform"`
	CreatedAt dotnet.DateTime `json:"created"`
	LastLogin dotnet.DateTime `json:"last_login"`
}

func NewUserData() *UserData { return &UserData{} }

// UserDataResponse mirrors UserDataResponse : UserData (stored collection
// "UserDataResponse"). STJ writes the derived members (roles, ban) first.
type UserDataResponse struct {
	UserData
	Roles []*string     `json:"roles"`
	Bans  []*BanRequest `json:"ban"`
}

func NewUserDataResponse() *UserDataResponse { return &UserDataResponse{} }

// UserDataV1Response mirrors UserDataV1Response.
type UserDataV1Response struct {
	UserId    *string         `json:"user_id"`
	Username  *string         `json:"username"`
	DiscordId *string         `json:"discord_id"`
	Platform  *string         `json:"platform"`
	CreatedAt dotnet.DateTime `json:"created"`
	LastLogin dotnet.DateTime `json:"last_login"`
	Roles     []*RoleResponse `json:"roles"`
	Bans      []*BanRequest   `json:"ban"`
}

func NewUserDataV1Response() *UserDataV1Response { return &UserDataV1Response{} }

// FromStorage mirrors UserDataV1Response.FromStorage(stored, roles, includeBans).
func FromStorage(stored *UserDataResponse, roles []*RoleResponse, includeBans bool) *UserDataV1Response {
	r := &UserDataV1Response{
		UserId:    stored.UserId,
		Username:  stored.Username,
		DiscordId: stored.DiscordId,
		Platform:  stored.Platform,
		CreatedAt: stored.CreatedAt,
		LastLogin: stored.LastLogin,
		Roles:     roles,
	}
	if includeBans {
		r.Bans = stored.Bans
	}
	return r
}

// UserRequest mirrors UserRequest ([DataMember(Name = ...)] equal to the JSON names).
type UserRequest struct {
	UserId    *string `json:"user_id"`
	Username  *string `json:"username"`
	DiscordId *string `json:"discord_id"`
	Platform  *string `json:"platform"`
}

func NewUserRequest() *UserRequest { return &UserRequest{} }

// UserRoleByNameRequest mirrors UserRoleByNameRequest.
type UserRoleByNameRequest struct {
	RoleId         *string `json:"role_id"`
	Username       *string `json:"username"`
	ExpiresInHours int     `json:"expires_hours"`
}

func NewUserRoleByNameRequest() *UserRoleByNameRequest { return &UserRoleByNameRequest{} }

// UserRoleByNameResponse mirrors UserRoleByNameResponse.
type UserRoleByNameResponse struct {
	Success    bool `json:"success"`
	UserExists bool `json:"user_exists"`
}

func NewUserRoleByNameResponse() *UserRoleByNameResponse { return &UserRoleByNameResponse{} }

// UsersResponse mirrors UsersResponse.
type UsersResponse struct {
	Users []*UserDataResponse `json:"users"`
}

func NewUsersResponse() *UsersResponse { return &UsersResponse{} }

// UserApiKeyDbObject mirrors UserApiKeyDbObject : AstraDbObject.
type UserApiKeyDbObject struct {
	docdb.AstraDbObject
	UserId *string `json:"user_id"`
	ApiKey *string `json:"api_key"`
}

func NewUserApiKeyDbObject() *UserApiKeyDbObject { return &UserApiKeyDbObject{} }

// ApiKeyResponse mirrors ApiKeyResponse.
type ApiKeyResponse struct {
	ApiKey *string `json:"api_key"`
}

func NewApiKeyResponse() *ApiKeyResponse { return &ApiKeyResponse{} }

// BanRequest mirrors BanRequest (also stored inside UserDataResponse.Bans).
type BanRequest struct {
	UserId     *string         `json:"user_id"`
	StationId  *string         `json:"station_id"`
	Expiration dotnet.DateTime `json:"expiration"`
	Reason     *string         `json:"reason"`
	Revoked    bool            `json:"revoked"`
}

func NewBanRequest() *BanRequest { return &BanRequest{} }

// BanResponse mirrors BanResponse.
type BanResponse struct {
	Bans []*BanRequest `json:"bans"`
}

func NewBanResponse() *BanResponse { return &BanResponse{} }

// StationPerm mirrors StationPerm.
type StationPerm struct {
	StationId  *string `json:"station_id"`
	Permission *string `json:"permission"`
}

func NewStationPerm() *StationPerm { return &StationPerm{} }
