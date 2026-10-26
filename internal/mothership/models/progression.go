// Ports Models/MothershipProgressionTrackDbObject.cs,
// MothershipProgressionTrackBindingDbObject.cs, MothershipProgressionTreeDbObject.cs
// and MothershipProgressionTreeBindingDbObject.cs.
package models

import "solara/internal/docdb"

// MothershipProgressionTrackDbObject is a progression track.
type MothershipProgressionTrackDbObject struct {
	docdb.AstraDbObject
	TrackId      *string `json:"track_id"`
	Name         *string `json:"name"`
	LevelsJson   *string `json:"levels"`
	TriggersJson *string `json:"triggers"`
}

func NewMothershipProgressionTrackDbObject() *MothershipProgressionTrackDbObject {
	o := &MothershipProgressionTrackDbObject{LevelsJson: P("[]"), TriggersJson: P("[]")}
	initStrings(o)
	return o
}

// MothershipProgressionTrackBindingDbObject binds a track to a deployment.
type MothershipProgressionTrackBindingDbObject struct {
	docdb.AstraDbObject
	BindingId    *string `json:"binding_id"`
	TrackId      *string `json:"track_id"`
	DeploymentId *string `json:"deployment_id"`
}

func NewMothershipProgressionTrackBindingDbObject() *MothershipProgressionTrackBindingDbObject {
	o := &MothershipProgressionTrackBindingDbObject{}
	initStrings(o)
	return o
}

// MothershipProgressionTreeDbObject is a progression tree.
type MothershipProgressionTreeDbObject struct {
	docdb.AstraDbObject
	TreeId    *string `json:"tree_id"`
	Name      *string `json:"name"`
	NodesJson *string `json:"nodes"`
}

func NewMothershipProgressionTreeDbObject() *MothershipProgressionTreeDbObject {
	o := &MothershipProgressionTreeDbObject{NodesJson: P("[]")}
	initStrings(o)
	return o
}

// MothershipProgressionTreeBindingDbObject binds a tree to a deployment.
type MothershipProgressionTreeBindingDbObject struct {
	docdb.AstraDbObject
	BindingId    *string `json:"binding_id"`
	TreeId       *string `json:"tree_id"`
	DeploymentId *string `json:"deployment_id"`
}

func NewMothershipProgressionTreeBindingDbObject() *MothershipProgressionTreeBindingDbObject {
	o := &MothershipProgressionTreeBindingDbObject{}
	initStrings(o)
	return o
}
