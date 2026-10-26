// Ports Models/MothershipFriendEntryDbObject.cs, MothershipPrivacyStateDbObject.cs,
// MothershipSharedGroupDbObject.cs and MothershipNotificationDbObject.cs.
package models

import (
	"solara/internal/docdb"
	"solara/internal/stj"
)

// MothershipFriendEntryDbObject is one friend of a player.
type MothershipFriendEntryDbObject struct {
	docdb.AstraDbObject
	OwnerId      *string `json:"owner_id"`
	FriendLinkId *string `json:"friend_link_id"`
	UserName     *string `json:"username"`
	RoomId       *string `json:"room_id"`
	Zone         *string `json:"zone"`
	Region       *string `json:"region"`
	IsPublic     bool    `json:"is_public"`
	Created      *string `json:"created"`
}

func NewMothershipFriendEntryDbObject() *MothershipFriendEntryDbObject {
	o := &MothershipFriendEntryDbObject{IsPublic: true}
	initStrings(o)
	return o
}

// MothershipPrivacyStateDbObject is a player's privacy state.
type MothershipPrivacyStateDbObject struct {
	docdb.AstraDbObject
	PlayFabId    *string `json:"playfab_id"`
	PrivacyState int     `json:"privacy_state"`
}

func NewMothershipPrivacyStateDbObject() *MothershipPrivacyStateDbObject {
	o := &MothershipPrivacyStateDbObject{}
	initStrings(o)
	return o
}

// MothershipSharedGroupDbObject is a shared group. Data is a
// Dictionary<string, string>; it is kept as an insertion-ordered stj.Obj
// because its JSON key order is observable. Values are Go strings;
// NormalizeData converts values decoded from storage.
type MothershipSharedGroupDbObject struct {
	docdb.AstraDbObject
	SharedGroupId *string   `json:"SharedGroupId"`
	Data          stj.Obj   `json:"data"`
	Members       []*string `json:"members"`
}

func NewMothershipSharedGroupDbObject() *MothershipSharedGroupDbObject {
	o := &MothershipSharedGroupDbObject{Data: stj.Obj{}, Members: []*string{}}
	initStrings(o)
	return o
}

// NormalizeData turns stored JsonElement values back into plain strings so
// the group serializes exactly as Dictionary<string, string> did.
func (g *MothershipSharedGroupDbObject) NormalizeData() {
	for i, kv := range g.Data {
		el, ok := kv.V.(stj.Element)
		if !ok {
			continue
		}
		// A stored null stays a Null element: it serializes as null and
		// re-stores as null (docdb cannot encode a nil interface value).
		if el.ValueKind() == stj.String {
			g.Data[i].V = el.Str()
		}
	}
}

// Remove mirrors Dictionary.Remove on Data.
func (g *MothershipSharedGroupDbObject) Remove(key string) {
	for i, kv := range g.Data {
		if kv.K == key {
			g.Data = append(g.Data[:i], g.Data[i+1:]...)
			return
		}
	}
}

// MothershipNotificationDbObject is a notification sent to a player.
type MothershipNotificationDbObject struct {
	docdb.AstraDbObject
	NotificationId *string `json:"notification_id"`
	PlayerId       *string `json:"player_id"`
	Message        *string `json:"message"`
	Type           *string `json:"type"`
	SentAt         *string `json:"sent_at"`
}

func NewMothershipNotificationDbObject() *MothershipNotificationDbObject {
	o := &MothershipNotificationDbObject{}
	initStrings(o)
	return o
}
