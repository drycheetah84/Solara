// Ports Models/MothershipTitleDbObject.cs, MothershipEnvironmentDbObject.cs,
// MothershipDeploymentDbObject.cs, MothershipTitleDataDbObject.cs,
// MothershipUserDataDbObject.cs, MothershipUserDataMetadataDbObject.cs and
// MothershipAnalyticsEventDbObject.cs.
package models

import "solara/internal/docdb"

// MothershipTitleDbObject is a title.
type MothershipTitleDbObject struct {
	docdb.AstraDbObject
	TitleId   *string `json:"title_id"`
	TitleName *string `json:"title_name"`
}

func NewMothershipTitleDbObject() *MothershipTitleDbObject {
	o := &MothershipTitleDbObject{}
	initStrings(o)
	return o
}

// MothershipEnvironmentDbObject is an environment of a title.
type MothershipEnvironmentDbObject struct {
	docdb.AstraDbObject
	EnvId        *string   `json:"envId"`
	TitleId      *string   `json:"titleId"`
	EnvName      *string   `json:"envName"`
	RequiredTags []*string `json:"requiredTags"`
}

func NewMothershipEnvironmentDbObject() *MothershipEnvironmentDbObject {
	o := &MothershipEnvironmentDbObject{RequiredTags: []*string{}}
	initStrings(o)
	return o
}

// MothershipDeploymentDbObject is a deployment.
type MothershipDeploymentDbObject struct {
	docdb.AstraDbObject
	DeploymentId *string   `json:"deployment_id"`
	TitleId      *string   `json:"title_id"`
	EnvId        *string   `json:"env_id"`
	Name         *string   `json:"name"`
	RequiredTags []*string `json:"requiredTags"`
}

func NewMothershipDeploymentDbObject() *MothershipDeploymentDbObject {
	o := &MothershipDeploymentDbObject{RequiredTags: []*string{}}
	initStrings(o)
	return o
}

// MothershipTitleDataDbObject is a title-data key/value pair.
type MothershipTitleDataDbObject struct {
	docdb.AstraDbObject
	Key       *string `json:"key"`
	Value     *string `json:"value"`
	UpdatedAt *string `json:"updated_at"`
}

func NewMothershipTitleDataDbObject() *MothershipTitleDataDbObject {
	o := &MothershipTitleDataDbObject{}
	initStrings(o)
	return o
}

// MothershipUserDataDbObject is one user-data key of a player.
type MothershipUserDataDbObject struct {
	docdb.AstraDbObject
	DataId     *string `json:"id"`
	UserId     *string `json:"user_id"`
	KeyName    *string `json:"key_name"`
	Value      *string `json:"value"`
	Generation int     `json:"generation"`
}

func NewMothershipUserDataDbObject() *MothershipUserDataDbObject {
	o := &MothershipUserDataDbObject{Generation: 1}
	initStrings(o)
	return o
}

// MothershipUserDataMetadataDbObject describes a user-data key.
type MothershipUserDataMetadataDbObject struct {
	docdb.AstraDbObject
	MetadataId     *string `json:"metadata_id"`
	TitleId        *string `json:"title_id"`
	EnvId          *string `json:"env_id"`
	KeyName        *string `json:"key_name"`
	KeyPermissions *string `json:"key_permissions"`
	PrivacyNotes   *string `json:"privacy_notes"`
}

func NewMothershipUserDataMetadataDbObject() *MothershipUserDataMetadataDbObject {
	o := &MothershipUserDataMetadataDbObject{}
	initStrings(o)
	return o
}

// MothershipAnalyticsEventDbObject is one stored analytics event.
type MothershipAnalyticsEventDbObject struct {
	docdb.AstraDbObject
	EventId   *string `json:"event_id"`
	Platform  *string `json:"platform"`
	Timestamp *string `json:"timestamp"`
	DataJson  *string `json:"data"`
}

func NewMothershipAnalyticsEventDbObject() *MothershipAnalyticsEventDbObject {
	o := &MothershipAnalyticsEventDbObject{DataJson: P("{}")}
	initStrings(o)
	return o
}
