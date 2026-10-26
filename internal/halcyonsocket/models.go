// Ports the DTO classes declared at the end of
// AUnrealFeatures.HalcyonSocket/HalcyonSocketServer.cs (WatchdogBody,
// SuccessResult, WatchdogRow, WatchdogStatusResult, SpinUpBody, SpinUpResult,
// AgentInfo, AgentListResult). Property names are the JSON names (STJ default
// policy); declaration order is preserved.
package halcyonsocket

import "solara/internal/dotnet"

// WatchdogBody is the body of POST /watchdog/retire and /watchdog/resume.
type WatchdogBody struct {
	Deployment_id *string `json:"deployment_id"`
	Reason        *string `json:"reason"`
}

func NewWatchdogBody() *WatchdogBody { return &WatchdogBody{} }

// SuccessResult is { success }.
type SuccessResult struct {
	Success bool `json:"success"`
}

func NewSuccessResult() *SuccessResult { return &SuccessResult{} }

// WatchdogRow is one row of GET /watchdog.
type WatchdogRow struct {
	Deployment_id   string           `json:"deployment_id"`
	Name            string           `json:"name"`
	Box             string           `json:"box"`
	Pid             int              `json:"pid"`
	Players         int              `json:"players"`
	Uptime_minutes  int              `json:"uptime_minutes"`
	Last_heartbeat  *dotnet.DateTime `json:"last_heartbeat"`
	Dispatch_age_ms uint64           `json:"dispatch_age_ms"`
	Tick_age_ms     uint64           `json:"tick_age_ms"`
	Verdict         string           `json:"verdict"`
	Restarts        int              `json:"restarts"`
	Last_restart    *dotnet.DateTime `json:"last_restart"`
	Retired         bool             `json:"retired"`
	Retired_reason  string           `json:"retired_reason"`
}

func NewWatchdogRow() *WatchdogRow { return &WatchdogRow{} }

// WatchdogStatusResult is { servers: WatchdogRow[] } (initialized to an empty array).
type WatchdogStatusResult struct {
	Servers []*WatchdogRow `json:"servers"`
}

func NewWatchdogStatusResult() *WatchdogStatusResult {
	return &WatchdogStatusResult{Servers: []*WatchdogRow{}}
}

// SpinUpBody is the body of POST /spinup (and of the dashboard's spin-up proxy).
type SpinUpBody struct {
	Box    *string `json:"box"`
	Map    *string `json:"map"`
	Region *string `json:"region"`
	Args   *string `json:"args"`
	Name   *string `json:"name"` // dashboard-entered deployment name
}

func NewSpinUpBody() *SpinUpBody { return &SpinUpBody{} }

// SpinUpResult is { success, request_id, agent, error }.
type SpinUpResult struct {
	Success    bool   `json:"success"`
	Request_id string `json:"request_id"`
	Agent      string `json:"agent"`
	Error      string `json:"error"`
}

func NewSpinUpResult() *SpinUpResult { return &SpinUpResult{} }

// AgentInfo is one connected box as shown in the dashboard's box picker.
type AgentInfo struct {
	Box       string          `json:"box"`
	Capacity  int             `json:"capacity"`
	Running   int             `json:"running"`
	Free      int             `json:"free"`
	Last_seen dotnet.DateTime `json:"last_seen"`
}

func NewAgentInfo() *AgentInfo { return &AgentInfo{} }

// AgentListResult is { agents: AgentInfo[] } (initialized to an empty array).
type AgentListResult struct {
	Agents []*AgentInfo `json:"agents"`
}

func NewAgentListResult() *AgentListResult { return &AgentListResult{Agents: []*AgentInfo{}} }
