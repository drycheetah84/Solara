// Port of AUnrealFeatures.EOSSDK/Models/EosLobbyModels.cs.
package models

import "solara/internal/stj"

// EosLobbyFilterRequest mirrors the C# class of the same name.
type EosLobbyFilterRequest struct {
	Criteria          []*EosFilterCriterion `json:"criteria"`
	MaxResults        int                   `json:"maxResults"`
	MinCurrentPlayers int                   `json:"minCurrentPlayers"`
}

// NewEosLobbyFilterRequest applies the C# property initializers.
func NewEosLobbyFilterRequest() *EosLobbyFilterRequest {
	return &EosLobbyFilterRequest{MaxResults: 100, MinCurrentPlayers: 0}
}

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosLobbyFilterRequest) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosLobbyFilterRequest()
	type plain EosLobbyFilterRequest
	return stj.FromElement(el, (*plain)(x))
}

// EosLobbyResponse mirrors the C# class of the same name.
type EosLobbyResponse struct {
	Count    int                `json:"count"`
	Sessions []*EosLobbySession `json:"sessions"`
}

// NewEosLobbyResponse applies the C# property initializers.
func NewEosLobbyResponse() *EosLobbyResponse {
	return &EosLobbyResponse{Sessions: []*EosLobbySession{}}
}

// EosLobbySession mirrors the C# class of the same name.
type EosLobbySession struct {
	Deployment        string            `json:"deployment"`
	Id                string            `json:"id"`
	Bucket            string            `json:"bucket"`
	Settings          *EosLobbySettings `json:"settings"`
	TotalPlayers      int               `json:"totalPlayers"`
	OpenPublicPlayers int               `json:"openPublicPlayers"`
	PublicPlayers     []string          `json:"publicPlayers"`
	Started           bool              `json:"started"`
	LastUpdated       string            `json:"lastUpdated"`
	Attributes        Dict[*string]     `json:"attributes"`
	Owner             string            `json:"owner"`
	OwnerPlatformId   int               `json:"ownerPlatformId"`

	// Per-member data — tracked in-memory, included in WS lobbyinfo response
	MemberData Dict[*EosLobbyMemberEntry] `json:"-"`

	// Lock token issued at create time, echoed back by client on every mutating frame
	Lock string `json:"-"`
}

// NewEosLobbySession applies the C# property initializers.
func NewEosLobbySession() *EosLobbySession {
	return &EosLobbySession{
		Deployment: "", Id: "", Bucket: "main", Settings: NewEosLobbySettings(),
		PublicPlayers: []string{}, LastUpdated: "", Attributes: NewDict[*string](), Owner: "",
		OwnerPlatformId: 4000, MemberData: NewDict[*EosLobbyMemberEntry](), Lock: "",
	}
}

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosLobbySession) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosLobbySession()
	type plain EosLobbySession
	return stj.FromElement(el, (*plain)(x))
}

// EosLobbySettings mirrors the C# class of the same name.
type EosLobbySettings struct {
	MaxPublicPlayers     int                `json:"maxPublicPlayers"`
	AllowInvites         bool               `json:"allowInvites"`
	ShouldAdvertise      bool               `json:"shouldAdvertise"`
	AllowReadById        bool               `json:"allowReadById"`
	AllowJoinViaPresence bool               `json:"allowJoinViaPresence"`
	AllowJoinInProgress  bool               `json:"allowJoinInProgress"`
	AllowConferenceRoom  bool               `json:"allowConferenceRoom"`
	CheckSanctions       bool               `json:"checkSanctions"`
	AllowMigration       bool               `json:"allowMigration"`
	RejoinAfterKick      string             `json:"rejoinAfterKick"`
	Platforms            *EosLobbyPlatforms `json:"platforms"`
}

// NewEosLobbySettings applies the C# property initializers.
func NewEosLobbySettings() *EosLobbySettings {
	return &EosLobbySettings{MaxPublicPlayers: 10, ShouldAdvertise: true, AllowReadById: true,
		AllowJoinViaPresence: true, AllowMigration: true, RejoinAfterKick: "open"}
}

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosLobbySettings) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosLobbySettings()
	type plain EosLobbySettings
	return stj.FromElement(el, (*plain)(x))
}

// EosLobbyPlatforms mirrors the C# class of the same name.
type EosLobbyPlatforms struct {
	AllowType          string `json:"allowType"`
	AllowedPlatformIds []int  `json:"allowedPlatformIds"`
}

// NewEosLobbyPlatforms applies the C# property initializers.
func NewEosLobbyPlatforms() *EosLobbyPlatforms { return &EosLobbyPlatforms{AllowType: "any"} }

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosLobbyPlatforms) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosLobbyPlatforms()
	type plain EosLobbyPlatforms
	return stj.FromElement(el, (*plain)(x))
}

// EosLobbyMemberEntry mirrors the C# class of the same name. Data values are
// bool, int64, float64, string, nil (set in code) or stj.Element (read from JSON).
type EosLobbyMemberEntry struct {
	Data       Dict[any] `json:"data"`
	PlatformId int       `json:"platformId"`
	Platform   string    `json:"platform"`
}

// NewEosLobbyMemberEntry applies the C# property initializers.
func NewEosLobbyMemberEntry() *EosLobbyMemberEntry {
	return &EosLobbyMemberEntry{Data: NewDict[any](), PlatformId: 4000, Platform: "other"}
}

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosLobbyMemberEntry) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosLobbyMemberEntry()
	type plain EosLobbyMemberEntry
	return stj.FromElement(el, (*plain)(x))
}

// EosLobbyCreateRequest mirrors the C# class of the same name.
type EosLobbyCreateRequest struct {
	Bucket     *string           `json:"bucket"`
	Settings   *EosLobbySettings `json:"settings"`
	Attributes *Dict[*string]    `json:"attributes"`
}

// NewEosLobbyCreateRequest applies the C# property initializers (none).
func NewEosLobbyCreateRequest() *EosLobbyCreateRequest { return &EosLobbyCreateRequest{} }

// EosLobbyUpdateRequest mirrors the C# class of the same name.
type EosLobbyUpdateRequest struct {
	Settings   *EosLobbySettings `json:"settings"`
	Attributes *Dict[*string]    `json:"attributes"`
	Started    *bool             `json:"started"`
}

// NewEosLobbyUpdateRequest applies the C# property initializers (none).
func NewEosLobbyUpdateRequest() *EosLobbyUpdateRequest { return &EosLobbyUpdateRequest{} }
