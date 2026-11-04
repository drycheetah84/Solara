// Ports AUnrealFeatures.Ares/Servers/A2StationDbServer.cs (class shell,
// constants and route table). The handlers live in a2station_*.go;
// ServerDrafters (declared in DrafterPermissions.cs) in drafter_permissions.go.
//
// A2StationDbServer (:78) is the station / deployment API the game-server DLL
// and the client talk to: register_server, update_player_count,
// /v1/deployments/{id}?include_station_config=true, users, roles, events, ...
package ares

import (
	"solara/internal/hosting"
)

const (
	a2Hostname = "localhost"
	a2Port     = 78
)

// IA2StationDbServer mirrors the (empty) marker interface.
type IA2StationDbServer interface{}

// A2StationDbServer mirrors the C# server class. The original constructor
// only called base(HOSTNAME, PORT): no background loops are started.
type A2StationDbServer struct {
	HTTP *hosting.Server
}

// NewA2StationDbServer builds the server, registers every endpoint in the
// original reflection order (docs/original_routes.tsv) and starts listening,
// as the C# constructor did. The bind error is returned.
func NewA2StationDbServer() (*A2StationDbServer, error) {
	s := newA2StationDbServer()
	if err := s.HTTP.Start(); err != nil {
		return s, err
	}
	return s, nil
}

// newA2StationDbServer builds and registers without binding (tests).
func newA2StationDbServer() *A2StationDbServer {
	s := &A2StationDbServer{HTTP: hosting.NewServer("A2StationDbServer", a2Hostname, a2Port)}
	s.registerEndpoints()
	return s
}

func (s *A2StationDbServer) registerEndpoints() {
	s.HTTP.Handle("GET", "/v1/region", "GetRegion", a2Guard("GetRegion", s.GetRegion))
	s.HTTP.Handle("POST", "/v2/users/log_in", "UserPlayerLoginV2", a2Guard("UserPlayerLoginV2", s.UserPlayerLoginV2))
	s.HTTP.Handle("POST", "/v1/users/log_in_with_key", "LogInWithKeyV1", a2Guard("LogInWithKeyV1", s.LogInWithKeyV1))
	s.HTTP.Handle("POST", "/stations/{station_id}/deployments", "CreateDeployment", a2Guard("CreateDeployment", s.CreateDeployment))
	s.HTTP.Handle("GET", "/stations/{station_id}/deployments", "GetStationDeploymentsV0", a2Guard("GetStationDeploymentsV0", s.GetStationDeploymentsV0))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/deployments", "GetStationDeploymentsV1", a2Guard("GetStationDeploymentsV1", s.GetStationDeploymentsV1))
	s.HTTP.Handle("POST", "/deployments/{deployment_id}/api_key", "CreateDeploymentApiKey", a2Guard("CreateDeploymentApiKey", s.CreateDeploymentApiKey))
	s.HTTP.Handle("POST", "/deployments/{deployment_id}/server_events", "AddServerEvent", a2Guard("AddServerEvent", s.AddServerEvent))
	s.HTTP.Handle("PATCH", "/deployments/{deployment_id}", "UpdateDeployment", a2Guard("UpdateDeployment", s.UpdateDeployment))
	s.HTTP.Handle("DELETE", "/deployments/{deployment_id}", "DeleteDeployment", a2Guard("DeleteDeployment", s.DeleteDeployment))
	s.HTTP.Handle("GET", "/deployments/{deployment_id}/config", "GetDeploymentConfigV0", a2Guard("GetDeploymentConfigV0", s.GetDeploymentConfigV0))
	s.HTTP.Handle("GET", "/v1/deployments/{deployment_id}", "GetDeploymentV1", a2Guard("GetDeploymentV1", s.GetDeploymentV1))
	s.HTTP.Handle("GET", "/v1/deployments/{deployment_id}/netvar_overrides", "GetNetvarOverrides", a2Guard("GetNetvarOverrides", s.GetNetvarOverrides))
	s.HTTP.Handle("GET", "/v1/spec/levels", "SpecListLevels", a2Guard("SpecListLevels", s.SpecListLevels))
	s.HTTP.Handle("GET", "/v1/spec/levels/{name}", "SpecGetLevel", a2Guard("SpecGetLevel", s.SpecGetLevel))
	s.HTTP.Handle("PUT", "/v1/spec/levels/{name}", "SpecPutLevel", a2Guard("SpecPutLevel", s.SpecPutLevel))
	s.HTTP.Handle("POST", "/v1/spec/levels/{name}/loaded", "SpecSetLoaded", a2Guard("SpecSetLoaded", s.SpecSetLoaded))
	s.HTTP.Handle("POST", "/v1/spec/boot", "SpecBoot", a2Guard("SpecBoot", s.SpecBoot))
	s.HTTP.Handle("GET", "/v1/spec/desired", "SpecDesired", a2Guard("SpecDesired", s.SpecDesired))
	s.HTTP.Handle("POST", "/v1/spec/status", "SpecStatus", a2Guard("SpecStatus", s.SpecStatus))
	s.HTTP.Handle("GET", "/v1/deployments/{deployment_id}/config", "GetDeploymentConfigV1", a2Guard("GetDeploymentConfigV1", s.GetDeploymentConfigV1))
	s.HTTP.Handle("POST", "/v1/deployments/{deployment_id}/config", "SetDeploymentConfigV1", a2Guard("SetDeploymentConfigV1", s.SetDeploymentConfigV1))
	s.HTTP.Handle("DELETE", "/v1/deployments/{deployment_id}/config", "DeleteDeploymentConfigV1", a2Guard("DeleteDeploymentConfigV1", s.DeleteDeploymentConfigV1))
	s.HTTP.Handle("GET", "/v1/deployments/{deployment_id}/server_events", "GetDeploymentServerEvents", a2Guard("GetDeploymentServerEvents", s.GetDeploymentServerEvents))
	s.HTTP.Handle("GET", "/v1/spec/drafters/{deployment_id}", "ServerDrafters", a2Guard("ServerDrafters", s.ServerDrafters))
	s.HTTP.Handle("POST", "/register_server", "RegisterServer", a2Guard("RegisterServer", s.RegisterServer))
	s.HTTP.Handle("POST", "/update_player_count", "UpdatePlayerCount", a2Guard("UpdatePlayerCount", s.UpdatePlayerCount))
	s.HTTP.Handle("POST", "/server_heartbeat", "ServerHeartbeat", a2Guard("ServerHeartbeat", s.ServerHeartbeat))
	s.HTTP.Handle("POST", "/v1/server/admin/purge_stations", "PurgeStations", a2Guard("PurgeStations", s.PurgeStations))
	s.HTTP.Handle("POST", "/v1/server/admin/prune_sessions", "PruneSessions", a2Guard("PruneSessions", s.PruneSessions))
	s.HTTP.Handle("GET", "/users", "ListAllUsersV0", a2Guard("ListAllUsersV0", s.ListAllUsersV0))
	s.HTTP.Handle("POST", "/users", "CreateOrUpdateUser", a2Guard("CreateOrUpdateUser", s.CreateOrUpdateUser))
	s.HTTP.Handle("GET", "/v1/users", "ListAllUsersV1", a2Guard("ListAllUsersV1", s.ListAllUsersV1))
	s.HTTP.Handle("DELETE", "/users/{user_id}", "DeleteUser", a2Guard("DeleteUser", s.DeleteUser))
	s.HTTP.Handle("GET", "/users/{user_id}", "GetUserV0", a2Guard("GetUserV0", s.GetUserV0))
	s.HTTP.Handle("GET", "/v1/users/{user_id}", "GetUserV1", a2Guard("GetUserV1", s.GetUserV1))
	s.HTTP.Handle("POST", "/users/log_in_server", "UserServerLogin", a2Guard("UserServerLogin", s.UserServerLogin))
	s.HTTP.Handle("POST", "/users/log_in_with_key", "LogInWithKey", a2Guard("LogInWithKey", s.LogInWithKey))
	s.HTTP.Handle("POST", "/users/log_in", "UserPlayerLogin", a2Guard("UserPlayerLogin", s.UserPlayerLogin))
	s.HTTP.Handle("POST", "/users/{user_id}/api_key", "CreateUserApiKey", a2Guard("CreateUserApiKey", s.CreateUserApiKey))
	s.HTTP.Handle("POST", "/stations/create", "CreateStation", a2Guard("CreateStation", s.CreateStation))
	s.HTTP.Handle("GET", "/stations", "FetchAllStationsV0", a2Guard("FetchAllStationsV0", s.FetchAllStationsV0))
	s.HTTP.Handle("GET", "/v1/stations", "FetchAllStationsV1", a2Guard("FetchAllStationsV1", s.FetchAllStationsV1))
	s.HTTP.Handle("GET", "/stations/{station_id}", "GetStation", a2Guard("GetStation", s.GetStation))
	s.HTTP.Handle("GET", "/stations/player_count", "GetPlayerCount", a2Guard("GetPlayerCount", s.GetPlayerCount))
	s.HTTP.Handle("PATCH", "/stations/{station_id}", "UpdateStation", a2Guard("UpdateStation", s.UpdateStation))
	s.HTTP.Handle("DELETE", "/stations/{station_id}", "DeleteStation", a2Guard("DeleteStation", s.DeleteStation))
	s.HTTP.Handle("POST", "/v1/board/upload", "UploadBoardImage", a2Guard("UploadBoardImage", s.UploadBoardImage))
	s.HTTP.Handle("GET", "/board/{file}", "GetBoardImage", a2Guard("GetBoardImage", s.GetBoardImage))
	s.HTTP.Handle("GET", "/v1/board/keys", "GetBoardKeys", a2Guard("GetBoardKeys", s.GetBoardKeys))
	s.HTTP.Handle("GET", "/stations/{station_id}/config", "GetStationConfig", a2Guard("GetStationConfig", s.GetStationConfig))
	s.HTTP.Handle("POST", "/stations/{station_id}/config", "SetStationConfig", a2Guard("SetStationConfig", s.SetStationConfig))
	s.HTTP.Handle("DELETE", "/stations/{station_id}/config", "DeleteStationConfig", a2Guard("DeleteStationConfig", s.DeleteStationConfig))
	s.HTTP.Handle("GET", "/stations/{station_id}/online", "GetStationOnline", a2Guard("GetStationOnline", s.GetStationOnline))
	s.HTTP.Handle("GET", "/stations/{station_id}/users", "GetAllUsersInStationV0", a2Guard("GetAllUsersInStationV0", s.GetAllUsersInStationV0))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/users", "GetAllUsersInStationV1", a2Guard("GetAllUsersInStationV1", s.GetAllUsersInStationV1))
	s.HTTP.Handle("GET", "/stations/{station_id}/users/{user_id}/roles", "GetStationUserRoles", a2Guard("GetStationUserRoles", s.GetStationUserRoles))
	s.HTTP.Handle("POST", "/stations/{station_id}/users/{user_id}/roles/{role_id}", "AddStationUserRole", a2Guard("AddStationUserRole", s.AddStationUserRole))
	s.HTTP.Handle("POST", "/stations/{station_id}/users/{user_id}/grant/{role_name}", "GrantStationUserRoleByName", a2Guard("GrantStationUserRoleByName", s.GrantStationUserRoleByName))
	s.HTTP.Handle("DELETE", "/stations/{station_id}/users/{user_id}/role/{role_id}", "DeleteStationUserRole", a2Guard("DeleteStationUserRole", s.DeleteStationUserRole))
	s.HTTP.Handle("POST", "/v1/stations/{station_id}/user_roles", "AddStationUserRoleByName", a2Guard("AddStationUserRoleByName", s.AddStationUserRoleByName))
	s.HTTP.Handle("GET", "/stations/{station_id}/roles", "GetStationRoles", a2Guard("GetStationRoles", s.GetStationRoles))
	s.HTTP.Handle("POST", "/stations/{station_id}/roles", "CreateStationRole", a2Guard("CreateStationRole", s.CreateStationRole))
	s.HTTP.Handle("DELETE", "/stations/{station_id}/roles/{role_id}", "DeleteStationRole", a2Guard("DeleteStationRole", s.DeleteStationRole))
	s.HTTP.Handle("PATCH", "/stations/{station_id}/roles/{role_id}", "UpdateStationRole", a2Guard("UpdateStationRole", s.UpdateStationRole))
	s.HTTP.Handle("PATCH", "/stations/{station_id}/roles/{role_id}/permissions", "AssignPerms", a2Guard("AssignPerms", s.AssignPerms))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/roles/{role_id}/users", "GetRoleUsers", a2Guard("GetRoleUsers", s.GetRoleUsers))
	s.HTTP.Handle("GET", "/stations/{station_id}/server_events", "GetServerEventsV0", a2Guard("GetServerEventsV0", s.GetServerEventsV0))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/server_events", "GetServerEventsV1", a2Guard("GetServerEventsV1", s.GetServerEventsV1))
	s.HTTP.Handle("POST", "/v1/stations/{station_id}/users/{user_id}/ban", "BanStationUser", a2Guard("BanStationUser", s.BanStationUser))
	s.HTTP.Handle("PATCH", "/v1/stations/{station_id}/users/{user_id}/unban", "UnbanStationUser", a2Guard("UnbanStationUser", s.UnbanStationUser))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/bans", "GetStationBans", a2Guard("GetStationBans", s.GetStationBans))
	s.HTTP.Handle("POST", "/v1/stations/{station_id}/event", "CreateStationEvent", a2Guard("CreateStationEvent", s.CreateStationEvent))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/events", "GetStationEvents", a2Guard("GetStationEvents", s.GetStationEvents))
	s.HTTP.Handle("GET", "/v1/events/{event_id}", "GetStationEvent", a2Guard("GetStationEvent", s.GetStationEvent))
	s.HTTP.Handle("DELETE", "/v1/stations/{station_id}/event/{event_id}", "DeleteStationEvent", a2Guard("DeleteStationEvent", s.DeleteStationEvent))
	s.HTTP.Handle("PATCH", "/v1/stations/{station_id}/event/{event_id}", "UpdateStationEvent", a2Guard("UpdateStationEvent", s.UpdateStationEvent))
	s.HTTP.Handle("POST", "/v1/stations/{station_id}/event/{event_id}/users/{user_id}/signup", "SignupStationEvent", a2Guard("SignupStationEvent", s.SignupStationEvent))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/event/{event_id}/signups", "GetStationEventSignups", a2Guard("GetStationEventSignups", s.GetStationEventSignups))
	s.HTTP.Handle("DELETE", "/v1/stations/{station_id}/event/{event_id}/signup", "DeleteStationEventSignup", a2Guard("DeleteStationEventSignup", s.DeleteStationEventSignup))
	s.HTTP.Handle("GET", "/v1/stations/{station_id}/event/{event_id}/config", "GetStationEventConfig", a2Guard("GetStationEventConfig", s.GetStationEventConfig))
	s.HTTP.Handle("POST", "/v1/stations/{station_id}/event/{event_id}/config", "SetStationEventConfig", a2Guard("SetStationEventConfig", s.SetStationEventConfig))
	s.HTTP.Handle("DELETE", "/v1/stations/{station_id}/event/{event_id}/config", "DeleteStationEventConfig", a2Guard("DeleteStationEventConfig", s.DeleteStationEventConfig))
	s.HTTP.Handle("GET", "/v2/fleets", "FetchAllFleetsV2", a2Guard("FetchAllFleetsV2", s.FetchAllFleetsV2))
	s.HTTP.Handle("GET", "/v2/fleets/{fleet_id}", "GetFleetV2", a2Guard("GetFleetV2", s.GetFleetV2))
	s.HTTP.Handle("GET", "/v2/stations", "FetchAllStationsV2", a2Guard("FetchAllStationsV2", s.FetchAllStationsV2))
	// -- Solara additions, registered after every original route --
	s.HTTP.Handle("POST", "/v1/deployments/{deployment_id}/gaia/events", "Solara/PostGaiaEvents", a2Guard("Solara/PostGaiaEvents", s.PostGaiaEvents))
	s.HTTP.Handle("GET", "/v1/deployments/{deployment_id}/gaia/next", "Solara/GetGaiaNext", a2Guard("Solara/GetGaiaNext", s.GetGaiaNext))
	s.HTTP.Handle("POST", "/v1/deployments/{deployment_id}/notify/players", "Solara/PostNotifyPlayers", a2Guard("Solara/PostNotifyPlayers", s.PostNotifyPlayers))
	s.HTTP.Handle("GET", "/v1/deployments/{deployment_id}/notify/next", "Solara/GetNotifyNext", a2Guard("Solara/GetNotifyNext", s.GetNotifyNext))
	s.HTTP.Handle("POST", "/v1/deployments/{deployment_id}/quest_reward", "Solara/PostQuestReward", a2Guard("Solara/PostQuestReward", s.PostQuestReward))
	s.HTTP.Handle("POST", "/v1/fleets/{fleet_id}/challenges/{challenge_id}/add", "Solara/AddChallengeRecord", a2Guard("Solara/AddChallengeRecord", s.AddChallengeRecord))
	s.HTTP.Handle("POST", "/v1/fleets/{fleet_id}/challenges/{challenge_id}/list", "Solara/ListChallengeRecords", a2Guard("Solara/ListChallengeRecords", s.ListChallengeRecords))
	// Anonymous-mode name tag (Server_SetAnonymousMode asks the station dashboard; the original had no route).
	s.HTTP.Handle("GET", "/v1/anonymous_name", "Solara/GetAnonymousName", a2Guard("Solara/GetAnonymousName", s.GetAnonymousName))
}
