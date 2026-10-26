// Port of AUnrealFeatures.EOSSDK/Models/EosMatchmakingModels.cs.
package models

import "solara/internal/stj"

// EosFilterCriterion mirrors the C# class of the same name.
type EosFilterCriterion struct {
	Key   string      `json:"key"`
	Op    string      `json:"op"`
	Value stj.Element `json:"value"`
}

// NewEosFilterCriterion applies the C# property initializers.
func NewEosFilterCriterion() *EosFilterCriterion { return &EosFilterCriterion{Key: "", Op: ""} }

// EosFilterRequest mirrors the C# class of the same name.
type EosFilterRequest struct {
	Criteria   []*EosFilterCriterion `json:"criteria"`
	MaxResults int                   `json:"maxResults"`
}

// NewEosFilterRequest applies the C# property initializers.
func NewEosFilterRequest() *EosFilterRequest { return &EosFilterRequest{MaxResults: 100} }

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosFilterRequest) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosFilterRequest()
	type plain EosFilterRequest
	return stj.FromElement(el, (*plain)(x))
}

// EosMatchmakingResponse mirrors the C# class of the same name.
type EosMatchmakingResponse struct {
	Count    int                      `json:"count"`
	Sessions []*EosMatchmakingSession `json:"sessions"`
}

// NewEosMatchmakingResponse applies the C# property initializers.
func NewEosMatchmakingResponse() *EosMatchmakingResponse {
	return &EosMatchmakingResponse{Sessions: []*EosMatchmakingSession{}}
}

// EosMatchmakingSession mirrors the C# class of the same name.
type EosMatchmakingSession struct {
	Deployment                      string                `json:"deployment"`
	Id                              string                `json:"id"`
	Bucket                          string                `json:"bucket"`
	Settings                        *EosSessionSettings   `json:"settings"`
	MaxPublicPlayers                int                   `json:"maxPublicPlayers"`
	OpenPublicPlayers               int                   `json:"openPublicPlayers"`
	MaxPrivatePlayers               int                   `json:"maxPrivatePlayers"`
	OpenPrivatePlayers              int                   `json:"openPrivatePlayers"`
	PublicPlayers                   []string              `json:"publicPlayers"`
	PrivatePlayers                  []string              `json:"privatePlayers"`
	TotalPlayers                    int                   `json:"totalPlayers"`
	AllowJoinInProgress             bool                  `json:"allowJoinInProgress"`
	ShouldAdvertise                 bool                  `json:"shouldAdvertise"`
	IsDedicated                     bool                  `json:"isDedicated"`
	UsesStats                       bool                  `json:"usesStats"`
	AllowInvites                    bool                  `json:"allowInvites"`
	UsesPresence                    bool                  `json:"usesPresence"`
	AllowJoinViaPresence            bool                  `json:"allowJoinViaPresence"`
	AllowJoinViaPresenceFriendsOnly bool                  `json:"allowJoinViaPresenceFriendsOnly"`
	BuildUniqueId                   int                   `json:"buildUniqueId"`
	Started                         bool                  `json:"started"`
	LastUpdated                     string                `json:"lastUpdated"`
	Attributes                      *EosSessionAttributes `json:"attributes"`
	Owner                           string                `json:"owner"`
	OwnerPlatformId                 *string               `json:"ownerPlatformId"`
}

// NewEosMatchmakingSession applies the C# property initializers.
func NewEosMatchmakingSession() *EosMatchmakingSession {
	return &EosMatchmakingSession{Settings: NewEosSessionSettings(), PublicPlayers: []string{},
		PrivatePlayers: []string{}, Attributes: NewEosSessionAttributes()}
}

// EosSessionAttributes mirrors the C# class of the same name.
type EosSessionAttributes struct {
	ServerName         string `json:"SERVERNAME_s"`
	BUsesStats         bool   `json:"BUSESSTATS_b"`
	NumPrivateConns    int    `json:"NUMPRIVATECONNECTIONS_l"`
	UsesPresence       bool   `json:"USESPRESENCE_b"`
	Address            string `json:"ADDRESS_s"`
	PresenceSearch     bool   `json:"PRESENCESEARCH_b"`
	NumPublicConns     int    `json:"NUMPUBLICCONNECTIONS_l"`
	PublicPort         string `json:"PUBLICPORT_s"`
	BuildUniqueId      int    `json:"BUILDUNIQUEID_l"`
	AntiCheatProtected bool   `json:"BANTICHEATPROTECTED_b"`
	IsDedicated        bool   `json:"BISDEDICATED_b"`
	ImguiPort          string `json:"IMGUIPORT_s"`
}

// NewEosSessionAttributes applies the C# property initializers.
func NewEosSessionAttributes() *EosSessionAttributes {
	return &EosSessionAttributes{PresenceSearch: true, AntiCheatProtected: true, IsDedicated: true}
}

// EosSessionSettings mirrors the C# class of the same name.
type EosSessionSettings struct {
	MaxPublicPlayers     int     `json:"maxPublicPlayers"`
	AllowInvites         bool    `json:"allowInvites"`
	ShouldAdvertise      bool    `json:"shouldAdvertise"`
	AllowReadById        bool    `json:"allowReadById"`
	AllowJoinViaPresence bool    `json:"allowJoinViaPresence"`
	AllowJoinInProgress  bool    `json:"allowJoinInProgress"`
	AllowConferenceRoom  bool    `json:"allowConferenceRoom"`
	CheckSanctions       bool    `json:"checkSanctions"`
	AllowMigration       bool    `json:"allowMigration"`
	RejoinAfterKick      string  `json:"rejoinAfterKick"`
	Platforms            *string `json:"platforms"`
}

// NewEosSessionSettings applies the C# property initializers.
func NewEosSessionSettings() *EosSessionSettings { return &EosSessionSettings{} }
