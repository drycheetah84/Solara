// Ports AUnrealFeatures.Ares/Models: DeploymentDbObject.cs,
// DeploymentRequest.cs (DeploymentRequest, DeploymentExpanded),
// ServerDeploymentResponse.cs, StationDeploymentResponse.cs,
// CreateDeploymentResponse.cs, RegisterServerModels.cs
// (RegisterServerRequest, RegisterServerResponse, UpdatePlayerCountRequest,
// ServerHeartbeatRequest).
package models

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

// DeploymentDbObject mirrors DeploymentDbObject : AstraDbObject.
// ApiKeys is [JsonIgnore] but still stored (LiteDB ignores JsonIgnore).
//
// LiteDB stores strings trimmed and "" as null (BsonMapper TrimWhitespace /
// EmptyStringToNull), so string members that can ever hold ""/whitespace are
// pointers (Config values included: a "" value reads back as null). ApiKeys
// stays []string: its only writers (CreateDeployment, CreateDeploymentApiKey,
// DashboardServer.LaunchDeployment) add GenerateApiKey/GenerateDashboardApiKey
// results, which are non-empty base64url segments joined by '.', never "" nor
// padded; and no code path reads it back.
type DeploymentDbObject struct {
	docdb.AstraDbObject
	DeploymentId   *string            `json:"deployment_id"`
	StationId      *string            `json:"station_id"`
	DeploymentName *string            `json:"deployment_name"`
	IpAddress      *string            `json:"ip"`
	Region         *string            `json:"region"`
	Version        *string            `json:"version"`
	CreatedAt      dotnet.DateTime    `json:"created"`
	Online         bool               `json:"online"`
	LastEvent      *dotnet.DateTime   `json:"last_event"`
	PlayerCount    int                `json:"player_count"`
	Config         map[string]*string `json:"config"`
	ApiKeys        []string           `json:"-"`
}

func NewDeploymentDbObject() *DeploymentDbObject {
	return &DeploymentDbObject{Config: map[string]*string{}, ApiKeys: []string{}}
}

// DeploymentRequest mirrors DeploymentRequest.
type DeploymentRequest struct {
	IpAddress      *string `json:"ip"`
	DeploymentName *string `json:"deployment_name"`
	Region         *string `json:"region"`
	Version        *string `json:"version"`
	StationId      *string `json:"station_id"`
}

func NewDeploymentRequest() *DeploymentRequest { return &DeploymentRequest{} }

// DeploymentExpanded mirrors DeploymentExpanded : DeploymentRequest. STJ
// writes the derived members first, then the DeploymentRequest ones.
type DeploymentExpanded struct {
	DeploymentRequest
	CreatedAt   dotnet.DateTime    `json:"created"`
	Online      bool               `json:"online"`
	LastEvent   dotnet.DateTime    `json:"last_event"`
	PlayerCount int                `json:"player_count"`
	Config      map[string]*string `json:"config"`
}

func NewDeploymentExpanded() *DeploymentExpanded { return &DeploymentExpanded{} }

// ServerDeploymentResponse mirrors ServerDeploymentResponse.
type ServerDeploymentResponse struct {
	DeploymentId   *string            `json:"deployment_id"`
	StationId      *string            `json:"station_id"`
	DeploymentName *string            `json:"deployment_name"`
	Region         *string            `json:"region"`
	IpAddress      *string            `json:"ip"`
	Version        *string            `json:"version"`
	CreatedAt      dotnet.DateTime    `json:"created"`
	Online         bool               `json:"online"`
	LastEventAt    dotnet.DateTime    `json:"last_event"`
	PlayerCount    int                `json:"player_count"`
	Config         map[string]*string `json:"config"`
}

func NewServerDeploymentResponse() *ServerDeploymentResponse { return &ServerDeploymentResponse{} }

// StationDeploymentResponse mirrors StationDeploymentResponse.
type StationDeploymentResponse struct {
	StationId   *string               `json:"station_id"`
	Deployments []*DeploymentExpanded `json:"deployments"`
}

func NewStationDeploymentResponse() *StationDeploymentResponse {
	return &StationDeploymentResponse{}
}

// CreateDeploymentResponse mirrors CreateDeploymentResponse.
type CreateDeploymentResponse struct {
	ApiKey       *string `json:"api_key"`
	DeploymentId *string `json:"deployment_id"`
}

func NewCreateDeploymentResponse() *CreateDeploymentResponse { return &CreateDeploymentResponse{} }

// RegisterServerRequest mirrors RegisterServerRequest (sent by a dedicated
// server, the injected DLL, when it spins up).
type RegisterServerRequest struct {
	Ip           *string `json:"ip"`
	Port         *string `json:"port"`
	DeploymentId *string `json:"deployment_id"`
	ServerName   *string `json:"server_name"`
	Region       *string `json:"region"`
	ImguiPort    *string `json:"imgui_port"`
	MaxPlayers   int     `json:"max_players"`
}

func NewRegisterServerRequest() *RegisterServerRequest { return &RegisterServerRequest{} }

// RegisterServerResponse mirrors RegisterServerResponse.
type RegisterServerResponse struct {
	Success      bool    `json:"success"`
	StationId    *string `json:"station_id"`
	DeploymentId *string `json:"deployment_id"`
}

func NewRegisterServerResponse() *RegisterServerResponse { return &RegisterServerResponse{} }

// UpdatePlayerCountRequest mirrors UpdatePlayerCountRequest.
type UpdatePlayerCountRequest struct {
	DeploymentId *string `json:"deployment_id"`
	PlayerCount  int     `json:"player_count"`
}

func NewUpdatePlayerCountRequest() *UpdatePlayerCountRequest { return &UpdatePlayerCountRequest{} }

// ServerHeartbeatRequest mirrors ServerHeartbeatRequest (POST /server_heartbeat).
type ServerHeartbeatRequest struct {
	DeploymentId  *string `json:"deployment_id"`
	Pid           int     `json:"pid"`
	Seq           uint64  `json:"seq"`
	DispatchAgeMs uint64  `json:"dispatch_age_ms"`
	TickAgeMs     uint64  `json:"tick_age_ms"`
	UptimeMs      uint64  `json:"uptime_ms"`
	Players       int     `json:"players"`
}

func NewServerHeartbeatRequest() *ServerHeartbeatRequest { return &ServerHeartbeatRequest{} }
