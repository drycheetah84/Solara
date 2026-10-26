// Ports AUnrealFeatures.Ares/Models: ServerEventDbObject.cs,
// ServerEventRequest.cs, ServerEventResponse.cs, StationEventDbObject.cs,
// StationEvents.cs (StationEvent, StationEvents), EventSignupDbObject.cs,
// EventSignupResponse.cs, StationEventSignupInfo.cs, StationEventSignups.cs,
// RoleData.cs (RoleData, RoleResponse, RolesResponse), AssignPermsBody.cs,
// CreateRoleRequest.cs, ClientLoginDeployment.cs, ClientLoginRequest.cs,
// ClientLoginResponse.cs, ClientLoginV2Response.cs, LogInServerRequest.cs,
// LogInServerResponse.cs (UserPermission, LogInServerResponse).
package models

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

// ServerEventDbObject mirrors ServerEventDbObject : AstraDbObject.
type ServerEventDbObject struct {
	docdb.AstraDbObject
	DeploymentId *string         `json:"deployment_id"`
	EventType    *string         `json:"event_type"`
	EventData    *string         `json:"event_data"`
	Timestamp    dotnet.DateTime `json:"timestamp"`
}

func NewServerEventDbObject() *ServerEventDbObject { return &ServerEventDbObject{} }

// ServerEventRequest mirrors ServerEventRequest.
type ServerEventRequest struct {
	EventType *string `json:"event_type"`
	EventData *string `json:"event_data"`
}

func NewServerEventRequest() *ServerEventRequest { return &ServerEventRequest{} }

// ServerEventResponse mirrors ServerEventResponse.
type ServerEventResponse struct {
	Index        int             `json:"idx"`
	EventType    *string         `json:"event_type"`
	DeploymentId *string         `json:"deployment_id"`
	EventData    *string         `json:"event_data"`
	Timestamp    dotnet.DateTime `json:"timestamp"`
}

func NewServerEventResponse() *ServerEventResponse { return &ServerEventResponse{} }

// StationEventDbObject mirrors StationEventDbObject : AstraDbObject.
type StationEventDbObject struct {
	docdb.AstraDbObject
	EventId      *string            `json:"event_id"`
	StationId    *string            `json:"station_id"`
	DeploymentId *string            `json:"deployment_id"`
	Title        *string            `json:"title"`
	Description  *string            `json:"description"`
	StartTime    dotnet.DateTime    `json:"start_time"`
	Duration     int                `json:"duration"`
	Public       bool               `json:"public"`
	SignupsOpen  bool               `json:"signups_open"`
	Config       map[string]*string `json:"config"`
}

func NewStationEventDbObject() *StationEventDbObject {
	return &StationEventDbObject{Config: map[string]*string{}}
}

// StationEvent mirrors StationEvent (StationEvents.cs).
type StationEvent struct {
	EventId      *string         `json:"event_id"`
	Title        *string         `json:"title"`
	Description  *string         `json:"description"`
	StartTime    dotnet.DateTime `json:"start_time"`
	Duration     int             `json:"duration"`
	Public       bool            `json:"public"`
	SignupsOpen  bool            `json:"signups_open"`
	StationId    *string         `json:"station_id"`
	DeploymentId *string         `json:"deployment_id"`
}

func NewStationEvent() *StationEvent { return &StationEvent{} }

// StationEvents mirrors StationEvents.
type StationEvents struct {
	Events []*StationEvent `json:"events"`
}

func NewStationEvents() *StationEvents { return &StationEvents{} }

// EventSignupDbObject mirrors EventSignupDbObject : AstraDbObject.
type EventSignupDbObject struct {
	docdb.AstraDbObject
	SignupId  *string         `json:"signup_id"`
	EventId   *string         `json:"event_id"`
	UserId    *string         `json:"user_id"`
	Timestamp dotnet.DateTime `json:"timestamp"`
	Data      *string         `json:"data"`
}

func NewEventSignupDbObject() *EventSignupDbObject { return &EventSignupDbObject{} }

// EventSignupResponse mirrors EventSignupResponse.
type EventSignupResponse struct {
	UserSignedUp bool    `json:"user_signed_up"`
	SignupId     *string `json:"signup_id"`
}

func NewEventSignupResponse() *EventSignupResponse { return &EventSignupResponse{} }

// StationEventSignupInfo mirrors StationEventSignupInfo.
type StationEventSignupInfo struct {
	SignupId  *string         `json:"signup_id"`
	UserId    *string         `json:"user_id"`
	Timestamp dotnet.DateTime `json:"timestamp"`
	Data      *string         `json:"data"`
}

func NewStationEventSignupInfo() *StationEventSignupInfo { return &StationEventSignupInfo{} }

// StationEventSignups mirrors StationEventSignups (no [JsonPropertyName]:
// the JSON name is the C# property name "Signups").
type StationEventSignups struct {
	Signups []*StationEventSignupInfo `json:"Signups"`
}

func NewStationEventSignups() *StationEventSignups { return &StationEventSignups{} }

// RoleData mirrors RoleData : AstraDbObject.
type RoleData struct {
	docdb.AstraDbObject
	RoleId          *string `json:"role_id"`
	StationId       *string `json:"station_id"`
	RoleName        *string `json:"role_name"`
	RoleDescription *string `json:"role_description"`
}

func NewRoleData() *RoleData { return &RoleData{} }

// RoleResponse mirrors RoleResponse : RoleData (stored collection
// "RoleResponse"). STJ writes "permissions" first, then the RoleData members.
type RoleResponse struct {
	RoleData
	Permissions []*string `json:"permissions"`
}

func NewRoleResponse() *RoleResponse { return &RoleResponse{} }

// RolesResponse mirrors RolesResponse.
type RolesResponse struct {
	Roles []*RoleResponse `json:"roles"`
}

func NewRolesResponse() *RolesResponse { return &RolesResponse{} }

// AssignPermsBody mirrors AssignPermsBody.
type AssignPermsBody struct {
	Permissions []*string `json:"permissions"`
}

func NewAssignPermsBody() *AssignPermsBody { return &AssignPermsBody{} }

// CreateRoleRequest mirrors CreateRoleRequest.
type CreateRoleRequest struct {
	RoleName        *string `json:"role_name"`
	RoleDescription *string `json:"role_description"`
}

func NewCreateRoleRequest() *CreateRoleRequest { return &CreateRoleRequest{} }

// ClientLoginDeployment mirrors ClientLoginDeployment.
type ClientLoginDeployment struct {
	DeploymentId   *string `json:"deployment_id"`
	StationId      *string `json:"station_id"`
	StationName    *string `json:"station_name"`
	DeploymentName *string `json:"deployment_name"`
	IpAddress      *string `json:"ip"`
	IsWhitelist    bool    `json:"is_whitelist"`
	IsPublic       bool    `json:"is_public"`
	IsJoinable     bool    `json:"is_joinable"`
}

func NewClientLoginDeployment() *ClientLoginDeployment { return &ClientLoginDeployment{} }

// ClientLoginRequest mirrors ClientLoginRequest (note the "userName" JSON name).
type ClientLoginRequest struct {
	Nonce      *string `json:"nonce"`
	PlatformId *string `json:"platform_id"`
	Username   *string `json:"userName"`
	Platform   *string `json:"platform"`
	Device     *string `json:"device"`
}

func NewClientLoginRequest() *ClientLoginRequest { return &ClientLoginRequest{} }

// ClientLoginResponse mirrors ClientLoginResponse.
type ClientLoginResponse struct {
	Success           bool                     `json:"success"`
	ApiKey            *string                  `json:"api_key"`
	OrgScopedId       *string                  `json:"org_scoped_id"`
	ServerDeployments []*ClientLoginDeployment `json:"server_deployments"`
}

func NewClientLoginResponse() *ClientLoginResponse { return &ClientLoginResponse{} }

// ClientLoginV2Response mirrors ClientLoginV2Response (A2 build 29932).
type ClientLoginV2Response struct {
	Success     bool   `json:"success"`
	ApiKey      string `json:"api_key"`
	OrgScopedId string `json:"org_scoped_id"`
}

func NewClientLoginV2Response() *ClientLoginV2Response {
	return &ClientLoginV2Response{ApiKey: "", OrgScopedId: ""}
}

// LogInServerRequest mirrors LogInServerRequest.
type LogInServerRequest struct {
	UserId   *string `json:"user_id"`
	Username *string `json:"username"`
	Platform *string `json:"platform"`
	Device   *string `json:"device"`
}

func NewLogInServerRequest() *LogInServerRequest { return &LogInServerRequest{} }

// UserPermission mirrors UserPermission (LogInServerResponse.cs).
type UserPermission struct {
	StationId  *string `json:"station_id"`
	Permission *string `json:"permission"`
}

func NewUserPermission() *UserPermission { return &UserPermission{} }

// LogInServerResponse mirrors LogInServerResponse. User is declared as
// UserData: STJ serializes the declared type, so a UserDataResponse assigned
// to it is written without its roles/ban members (assign &x.UserData).
type LogInServerResponse struct {
	ApiKey      *string           `json:"api_key"`
	User        *UserData         `json:"user"`
	Permissions []*UserPermission `json:"permissions"`
}

func NewLogInServerResponse() *LogInServerResponse { return &LogInServerResponse{} }
