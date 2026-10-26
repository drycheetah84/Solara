// Ports the response/request DTOs at the end of
// AUnrealFeatures.Ares/Servers/DashboardServer.cs ("dashboard-only, no DB coupling").
//
// Field order, JSON names and nullability are the C# ones. A few properties
// declared `string X { get; set; } = string.Empty` are *string here because
// the handlers assign them straight from nullable model properties (e.g.
// DeploymentId = d.DeploymentId), so the original emitted `null` for them;
// keeping them nullable preserves that output.
package ares

import "solara/internal/dotnet"

type DashboardStats struct {
	TotalUsers        int `json:"total_users"`
	TotalStations     int `json:"total_stations"`
	TotalDeployments  int `json:"total_deployments"`
	OnlineDeployments int `json:"online_deployments"`
	TotalPlayers      int `json:"total_players"`
	TotalEvents       int `json:"total_events"`
}

func NewDashboardStats() *DashboardStats { return &DashboardStats{} }

type DashboardUser struct {
	UserId    string          `json:"user_id"`
	Username  string          `json:"username"`
	Platform  *string         `json:"platform"`
	LastLogin dotnet.DateTime `json:"last_login"`
	CreatedAt dotnet.DateTime `json:"created_at"`
	RoleCount int             `json:"role_count"`
	Banned    bool            `json:"banned"`
	IsAdmin   bool            `json:"is_admin"`
}

func NewDashboardUser() *DashboardUser { return &DashboardUser{UserId: "", Username: ""} }

type DashboardStation struct {
	StationId   *string          `json:"station_id"`
	StationName *string          `json:"station_name"`
	CreatedAt   dotnet.DateTime  `json:"created_at"`
	Online      bool             `json:"online"`
	LastOnline  *dotnet.DateTime `json:"last_online"`
	Deployments int              `json:"deployments"`
	OnlineDeps  int              `json:"online_deps"`
	PlayerCount int              `json:"player_count"`
}

func NewDashboardStation() *DashboardStation {
	return &DashboardStation{StationId: dashStrPtr(""), StationName: dashStrPtr("")}
}

type DashboardDeployment struct {
	DeploymentId   *string          `json:"deployment_id"`
	StationId      *string          `json:"station_id"`
	DeploymentName *string          `json:"deployment_name"`
	IpAddress      *string          `json:"ip_address"`
	Region         *string          `json:"region"`
	Version        *string          `json:"version"`
	Online         bool             `json:"online"`
	PlayerCount    int              `json:"player_count"`
	LastEvent      *dotnet.DateTime `json:"last_event"`
	CreatedAt      dotnet.DateTime  `json:"created_at"`
}

func NewDashboardDeployment() *DashboardDeployment {
	return &DashboardDeployment{DeploymentId: dashStrPtr(""), StationId: dashStrPtr("")}
}

type DashboardEvent struct {
	EventId      string          `json:"event_id"`
	DeploymentId *string         `json:"deployment_id"`
	EventName    *string         `json:"event_name"`
	CreatedAt    dotnet.DateTime `json:"created_at"`
}

func NewDashboardEvent() *DashboardEvent {
	return &DashboardEvent{EventId: "", DeploymentId: dashStrPtr(""), EventName: dashStrPtr("")}
}

type DashboardActionResult struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
	Id      *string `json:"id"`
}

func NewDashboardActionResult() *DashboardActionResult { return &DashboardActionResult{} }

type DashboardBanRequest struct {
	StationId *string `json:"station_id"`
	Reason    *string `json:"reason"`
	Duration  int     `json:"duration"`
}

func NewDashboardBanRequest() *DashboardBanRequest { return &DashboardBanRequest{} }

type DashboardBan struct {
	UserId     *string          `json:"user_id"`
	Username   *string          `json:"username"`
	StationId  *string          `json:"station_id"`
	Reason     *string          `json:"reason"`
	Expiration *dotnet.DateTime `json:"expiration"`
	Revoked    bool             `json:"revoked"`
}

func NewDashboardBan() *DashboardBan {
	return &DashboardBan{UserId: dashStrPtr(""), Username: dashStrPtr(""), StationId: dashStrPtr(""), Reason: dashStrPtr("")}
}

type DashboardRole struct {
	RoleId          *string   `json:"role_id"`
	StationId       *string   `json:"station_id"`
	RoleName        *string   `json:"role_name"`
	RoleDescription *string   `json:"role_description"`
	Permissions     []*string `json:"permissions"`
}

func NewDashboardRole() *DashboardRole {
	return &DashboardRole{RoleId: dashStrPtr(""), StationId: dashStrPtr(""), RoleName: dashStrPtr(""),
		RoleDescription: dashStrPtr(""), Permissions: []*string{}}
}

type DashboardCreateRole struct {
	RoleName        *string `json:"role_name"`
	RoleDescription *string `json:"role_description"`
}

func NewDashboardCreateRole() *DashboardCreateRole { return &DashboardCreateRole{} }

type DashboardSetPermissions struct {
	Permissions []*string `json:"permissions"`
}

func NewDashboardSetPermissions() *DashboardSetPermissions { return &DashboardSetPermissions{} }

type DashboardCreateStation struct {
	StationName *string `json:"station_name"`
}

func NewDashboardCreateStation() *DashboardCreateStation { return &DashboardCreateStation{} }

type DashboardCreateDeployment struct {
	DeploymentName *string `json:"deployment_name"`
	IpAddress      *string `json:"ip"`
	Region         *string `json:"region"`
	Version        *string `json:"version"`
}

func NewDashboardCreateDeployment() *DashboardCreateDeployment { return &DashboardCreateDeployment{} }

type DashboardLaunchResult struct {
	Success bool    `json:"success"`
	Id      *string `json:"id"`
	ApiKey  *string `json:"api_key"`
}

func NewDashboardLaunchResult() *DashboardLaunchResult { return &DashboardLaunchResult{} }

type DashboardCreateEvent struct {
	Title        *string         `json:"title"`
	Description  *string         `json:"description"`
	StartTime    dotnet.DateTime `json:"start_time"`
	Duration     int             `json:"duration"`
	DeploymentId *string         `json:"deployment_id"`
	Public       bool            `json:"public"`
	SignupsOpen  bool            `json:"signups_open"`
}

func NewDashboardCreateEvent() *DashboardCreateEvent { return &DashboardCreateEvent{} }

type DashboardStationEvent struct {
	EventId      *string         `json:"event_id"`
	StationId    *string         `json:"station_id"`
	DeploymentId *string         `json:"deployment_id"`
	Title        *string         `json:"title"`
	Description  *string         `json:"description"`
	StartTime    dotnet.DateTime `json:"start_time"`
	Duration     int             `json:"duration"`
	Public       bool            `json:"public"`
	SignupsOpen  bool            `json:"signups_open"`
}

func NewDashboardStationEvent() *DashboardStationEvent {
	return &DashboardStationEvent{EventId: dashStrPtr(""), StationId: dashStrPtr(""), Title: dashStrPtr(""), Description: dashStrPtr("")}
}

type DashboardMember struct {
	UserId   *string   `json:"user_id"`
	Username *string   `json:"username"`
	Platform *string   `json:"platform"`
	Roles    []*string `json:"roles"`
}

func NewDashboardMember() *DashboardMember {
	return &DashboardMember{UserId: dashStrPtr(""), Username: dashStrPtr(""), Roles: []*string{}}
}

// dashboardSpecLevelPatchBody mirrors the private nested SpecLevelPatchBody.
type dashboardSpecLevelPatchBody struct {
	Autoload *bool `json:"autoload"`
	Loaded   *bool `json:"loaded"`
}

// dashboardWhitelistBody mirrors the private nested WhitelistBody.
type dashboardWhitelistBody struct {
	Usernames []*string `json:"usernames"`
}
