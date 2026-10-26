// Ports Models/MothershipBanDbObject.cs, MothershipMuteDbObject.cs and
// MothershipReportDbObject.cs.
package models

import (
	"solara/internal/docdb"
	"solara/internal/dotnet"
)

// MothershipBanDbObject is a player ban.
type MothershipBanDbObject struct {
	docdb.AstraDbObject
	BanId           *string         `json:"ban_id"`
	TitleId         *string         `json:"title_id"`
	EnvId           *string         `json:"env_id"`
	PlayerId        *string         `json:"player_id"`
	Category        int             `json:"category"`
	Reason          *string         `json:"reason"`
	DurationMinutes int             `json:"duration_minutes"`
	OrgWide         bool            `json:"org_wide"`
	Metadata        *string         `json:"metadata"`
	CreatedAt       dotnet.DateTime `json:"created_at"`
	ExpiresAt       dotnet.DateTime `json:"expires_at"`
}

func NewMothershipBanDbObject() *MothershipBanDbObject {
	o := &MothershipBanDbObject{}
	initStrings(o)
	return o
}

// MothershipMuteDbObject is a player mute.
type MothershipMuteDbObject struct {
	docdb.AstraDbObject
	MuteId          *string         `json:"mute_id"`
	PlayerId        *string         `json:"player_id"`
	DurationMinutes int             `json:"duration_minutes"`
	CreatedAt       dotnet.DateTime `json:"created_at"`
}

func NewMothershipMuteDbObject() *MothershipMuteDbObject {
	o := &MothershipMuteDbObject{}
	initStrings(o)
	return o
}

// MothershipReportDbObject is a moderation report.
type MothershipReportDbObject struct {
	docdb.AstraDbObject
	ReportId       *string         `json:"report_id"`
	ReportedUserId *string         `json:"reported_user_id"`
	ReporterUserId *string         `json:"reporter_user_id"`
	Category       int             `json:"category"`
	Reason         *string         `json:"reason"`
	Platform       *string         `json:"platform"`
	ModdedClient   bool            `json:"modded_client"`
	Metadata       *string         `json:"metadata"`
	CreatedAt      dotnet.DateTime `json:"created_at"`
}

func NewMothershipReportDbObject() *MothershipReportDbObject {
	o := &MothershipReportDbObject{}
	initStrings(o)
	return o
}
