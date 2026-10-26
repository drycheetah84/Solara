// Ports AUnrealFeatures.Ares/Models: StationDbObject.cs, StationResponse.cs,
// CreateStationRequest.cs, CreateStationResponse.cs, UpdateStationRequest.cs,
// UpdateStationResponse.cs, StationsPageV2.cs, FleetResponse.cs,
// FleetStationResponse.cs, RegionResponse.cs, SuccessBoolean.cs,
// HTTPValidationError.cs (ValidationError, HTTPValidationError).
package models

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

// StationDbObject mirrors StationDbObject : AstraDbObject.
// Config is Dictionary<string, string> (= new()); Go maps serialize with
// sorted keys (see the Ares port notes on dictionary key order).
type StationDbObject struct {
	docdb.AstraDbObject
	StationId   *string            `json:"station_id"`
	StationName *string            `json:"station_name"`
	CreatedAt   dotnet.DateTime    `json:"created"`
	Online      bool               `json:"online"`
	LastOnline  *dotnet.DateTime   `json:"last_online"`
	Config      map[string]*string `json:"config"`
}

func NewStationDbObject() *StationDbObject {
	return &StationDbObject{Config: map[string]*string{}}
}

// StationResponse mirrors StationResponse.
type StationResponse struct {
	StationId   *string                     `json:"station_id"`
	StationName *string                     `json:"station_name"`
	CreatedAt   dotnet.DateTime             `json:"created"`
	Deployments []*ServerDeploymentResponse `json:"deployments"`
	Config      map[string]*string          `json:"config"`
}

func NewStationResponse() *StationResponse { return &StationResponse{} }

// CreateStationRequest mirrors CreateStationRequest.
type CreateStationRequest struct {
	StationName *string `json:"station_name"`
	StationId   *string `json:"station_id"`
}

func NewCreateStationRequest() *CreateStationRequest { return &CreateStationRequest{} }

// CreateStationResponse mirrors CreateStationResponse.
type CreateStationResponse struct {
	StationId *string `json:"station_id"`
}

func NewCreateStationResponse() *CreateStationResponse { return &CreateStationResponse{} }

// UpdateStationRequest mirrors UpdateStationRequest.
type UpdateStationRequest struct {
	StationName *string `json:"station_name"`
}

func NewUpdateStationRequest() *UpdateStationRequest { return &UpdateStationRequest{} }

// UpdateStationResponse mirrors UpdateStationResponse.
type UpdateStationResponse struct {
	StationName *string `json:"station_name"`
}

func NewUpdateStationResponse() *UpdateStationResponse { return &UpdateStationResponse{} }

// StationsPageV2 mirrors StationsPageV2 (A2 build 29932 GET /v2/stations).
type StationsPageV2 struct {
	FleetId string                  `json:"fleet_id"`
	Items   []*FleetStationResponse `json:"items"`
}

func NewStationsPageV2() *StationsPageV2 {
	return &StationsPageV2{FleetId: "", Items: []*FleetStationResponse{}}
}

// FleetResponse mirrors FleetResponse. Config is Dictionary<string, object>?
// holding bool / double / string values.
type FleetResponse struct {
	FleetId   *string                 `json:"fleet_id"`
	FleetName *string                 `json:"fleet_name"`
	Created   dotnet.DateTime         `json:"created"`
	Online    bool                    `json:"online"`
	Stations  []*FleetStationResponse `json:"stations"`
	Config    map[string]any          `json:"config"`
}

func NewFleetResponse() *FleetResponse { return &FleetResponse{} }

// FleetStationResponse mirrors FleetStationResponse.
type FleetStationResponse struct {
	StationId           *string          `json:"station_id"`
	FleetId             *string          `json:"fleet_id"`
	SessionId           *string          `json:"session_id"`
	StationName         *string          `json:"station_name"`
	Region              *string          `json:"region"`
	Ip                  *string          `json:"ip"`
	Version             *string          `json:"version"`
	DeploymentCl        *string          `json:"deployment_cl"`
	Created             dotnet.DateTime  `json:"created"`
	Online              bool             `json:"online"`
	LastEvent           *dotnet.DateTime `json:"last_event"`
	PlayerCount         int              `json:"player_count"`
	Disabled            bool             `json:"disabled"`
	Config              map[string]any   `json:"config"`
	DistrictPopulations any              `json:"district_populations"`
}

func NewFleetStationResponse() *FleetStationResponse { return &FleetStationResponse{} }

// RegionResponse mirrors RegionResponse (A2 build 29932 GET /v1/region).
type RegionResponse struct {
	ContinentCode string `json:"continent_code"`
	CountryCode   string `json:"country_code"`
}

func NewRegionResponse() *RegionResponse {
	return &RegionResponse{ContinentCode: "NA", CountryCode: "US"}
}

// SuccessBoolean mirrors SuccessBoolean.
type SuccessBoolean struct {
	Success bool `json:"success"`
}

func NewSuccessBoolean() *SuccessBoolean { return &SuccessBoolean{} }

// ValidationError mirrors ValidationError (HTTPValidationError.cs).
type ValidationError struct {
	Locations []string `json:"loc"`
	Message   *string  `json:"msg"`
	Type      *string  `json:"type"`
}

func NewValidationError() *ValidationError { return &ValidationError{} }

// HTTPValidationError mirrors HTTPValidationError.
type HTTPValidationError struct {
	Details []*ValidationError `json:"detail"`
}

func NewHTTPValidationError() *HTTPValidationError { return &HTTPValidationError{} }
