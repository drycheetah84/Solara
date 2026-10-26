// Port of AUnrealFeatures.EOSSDK/Models/EosOAuthTokenResponse.cs, EosSessionInfo.cs,
// EosTelemetryRequest.cs and EosTurnCredentialsResponse.cs.
package models

import (
	"solara/internal/dotnet"
	"solara/internal/stj"
)

// EosOAuthTokenResponse mirrors the C# class of the same name.
type EosOAuthTokenResponse struct {
	AccessToken          string   `json:"access_token"`
	TokenType            string   `json:"token_type"`
	ExpiresAt            string   `json:"expires_at"`
	ExpiresIn            int      `json:"expires_in"`
	Nonce                *string  `json:"nonce,omitnull"`
	Features             []string `json:"features"`
	OrganizationId       string   `json:"organization_id"`
	ProductId            string   `json:"product_id"`
	SandboxId            string   `json:"sandbox_id"`
	DeploymentId         string   `json:"deployment_id"`
	OrganizationUserId   *string  `json:"organization_user_id,omitnull"`
	ProductUserId        *string  `json:"product_user_id,omitnull"`
	ProductUserIdCreated *bool    `json:"product_user_id_created,omitnull"`
	IdToken              *string  `json:"id_token,omitnull"`
	ClientId             *string  `json:"client_id,omitnull"`
}

// NewEosOAuthTokenResponse applies the C# property initializers.
func NewEosOAuthTokenResponse() *EosOAuthTokenResponse {
	return &EosOAuthTokenResponse{AccessToken: "", TokenType: "bearer", ExpiresAt: "", Features: []string{}}
}

// EosSessionInfo mirrors the C# class of the same name.
type EosSessionInfo struct {
	Id            string   `json:"id"`
	ServerName    string   `json:"server_name"`
	IpAddress     string   `json:"ip_address"`
	Port          string   `json:"port"`
	MaxPlayers    int      `json:"max_players"`
	Bucket        string   `json:"bucket"`
	BuildId       int      `json:"build_id"`
	ImguiPort     string   `json:"imgui_port"`
	StationId     *string  `json:"station_id"`
	DeploymentId  *string  `json:"deployment_id"`
	PublicPlayers []string `json:"public_players"`
}

// NewEosSessionInfo applies the C# property initializers (Id = Guid.NewGuid().ToString("N")).
func NewEosSessionInfo() *EosSessionInfo {
	return &EosSessionInfo{Id: dotnet.NewGuidN(), MaxPlayers: 10, Bucket: "A2_4729", BuildId: 4729, PublicPlayers: []string{}}
}

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosSessionInfo) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosSessionInfo()
	type plain EosSessionInfo
	return stj.FromElement(el, (*plain)(x))
}

// EosTelemetryRequest mirrors the C# class of the same name.
type EosTelemetryRequest struct {
	Events []*EosTelemetryEvent `json:"Events"`
}

// NewEosTelemetryRequest applies the C# property initializers.
func NewEosTelemetryRequest() *EosTelemetryRequest {
	return &EosTelemetryRequest{Events: []*EosTelemetryEvent{}}
}

// UnmarshalSTJ constructs the object (initializers) before reading members, as STJ does.
func (x *EosTelemetryRequest) UnmarshalSTJ(el stj.Element) error {
	*x = *NewEosTelemetryRequest()
	type plain EosTelemetryRequest
	return stj.FromElement(el, (*plain)(x))
}

// EosTelemetryEvent mirrors the C# class of the same name. EventData values
// are stj.Element (Dictionary<string, object> read from JSON).
type EosTelemetryEvent struct {
	EventName      string     `json:"EventName"`
	EventNamespace *string    `json:"EventNamespace"`
	DateOffset     *string    `json:"DateOffset"`
	EventData      *Dict[any] `json:"EventData"`
}

// NewEosTelemetryEvent applies the C# property initializers.
func NewEosTelemetryEvent() *EosTelemetryEvent { return &EosTelemetryEvent{EventName: ""} }

// EosTurnCredentialsResponse mirrors the C# class of the same name.
type EosTurnCredentialsResponse struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Ttl      int      `json:"ttl"`
	Uris     []string `json:"uris"`
}

// NewEosTurnCredentialsResponse applies the C# property initializers.
func NewEosTurnCredentialsResponse() *EosTurnCredentialsResponse {
	return &EosTurnCredentialsResponse{Uris: []string{}}
}
