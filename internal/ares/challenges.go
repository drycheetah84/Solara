// Solara addition: challenge leaderboards (station dashboard, :78).
//
// The 50451 client and game server call two routes the original backend never had
// (A2StationDashboardSubsystem, strings next to "/v1/fleets/global/challenges/"):
//
//	POST /v1/fleets/{fleet_id}/challenges/{challenge_id}/add          body FPostChallengeRecordBody
//	POST /v1/fleets/{fleet_id}/challenges/{challenge_id}/list?limit=N body FGetChallengeRecordsRequest
//
// "PostChallengeRecord for user: %s with time: %f" -- a challenge is a quest whose completion time is recorded
// ("Quest completed by non-VRPlayerController, not posting challenge"), so a record is a TIME: lower is better.
// Bodies go through FJsonObjectConverter, which writes UPROPERTY names with the first letter lower-cased:
//
//	FPostChallengeRecordBody      { user_id: FString, record: float }
//	FGetChallengeRecordsRequest   { user_id: FString, friends: TArray<FString> }
//	FGetChallengeRecordsResponse  { top, nearby, friends: TArray<FChallengeRecordEntry>, me: FChallengeRecordEntry }
//	FChallengeRecordEntry         { user_id: FString, userName: FString, record: float, rank: int32 }
//
// Each player keeps their best (lowest) record per challenge. The fleet in the path is stored but not used for
// ranking: the client always lists the "global" fleet.
package ares

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

// ChallengeRecordDbObject is one player's best record on one challenge.
type ChallengeRecordDbObject struct {
	docdb.AstraDbObject
	ChallengeId string  `json:"challenge_id"`
	FleetId     string  `json:"fleet_id"`
	UserId      string  `json:"user_id"`
	Record      float64 `json:"record"`
	UpdatedAt   int64   `json:"updated_at"` // unix ms of the record (ties: earlier wins)
}

func NewChallengeRecordDbObject() *ChallengeRecordDbObject { return &ChallengeRecordDbObject{} }

func a2ChallengeRecords() *docdb.Collection[ChallengeRecordDbObject] {
	return docdb.GetCollection[ChallengeRecordDbObject](Database, true, NewChallengeRecordDbObject)
}

// challengeMu serialises best-record updates (read-compare-write).
var challengeMu sync.Mutex

type challengeEntry struct {
	UserId   string  `json:"user_id"`
	UserName string  `json:"userName"`
	Record   float64 `json:"record"`
	Rank     int     `json:"rank"`
}

type challengeListResponse struct {
	Top     []challengeEntry `json:"top"`
	Nearby  []challengeEntry `json:"nearby"`
	Friends []challengeEntry `json:"friends"`
	Me      challengeEntry   `json:"me"`
}

// jsonField reads a body field case-insensitively (FJsonObjectConverter is case-insensitive on read, so be too).
func jsonField(m map[string]json.RawMessage, name string) (json.RawMessage, bool) {
	if v, ok := m[name]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, name) {
			return v, true
		}
	}
	return nil, false
}

func challengeUserName(userId string) string {
	id := userId
	if u := a2FindUser(&id); u != nil && u.Username != nil && *u.Username != "" {
		return *u.Username
	}
	return userId
}

// AddChallengeRecord stores a record if it beats the player's best.
func (s *A2StationDbServer) AddChallengeRecord(c *hosting.Context) hosting.Result {
	challenge := strings.TrimSpace(c.Param("challenge_id"))
	fleet := strings.TrimSpace(c.Param("fleet_id"))
	if challenge == "" || len(challenge) > 128 {
		return hosting.BadRequest(stj.O("error", "challenge_id required"))
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		return hosting.BadRequest(stj.O("error", "body must be a JSON object"))
	}
	var userId string
	var record float64
	if raw, ok := jsonField(body, "user_id"); ok {
		_ = json.Unmarshal(raw, &userId)
	}
	if raw, ok := jsonField(body, "record"); ok {
		_ = json.Unmarshal(raw, &record)
	}
	userId = strings.TrimSpace(userId)
	if userId == "" || len(userId) > 128 {
		return hosting.BadRequest(stj.O("error", "user_id required"))
	}
	if math.IsNaN(record) || math.IsInf(record, 0) || record <= 0 || record > 1e7 {
		return hosting.BadRequest(stj.O("error", "record must be a positive time"))
	}
	// Game servers (trusted) may post for anyone. A player's own key always posts for that player: the record is
	// stored under the authenticated user, whatever id the body carried (so a format mismatch can't drop records,
	// and a player can't post for someone else). Any other issued key (e.g. a deployment key) is refused.
	cred := a2Credential(c, "A2StationDb Solara/AddChallengeRecord", a2ServerToken(c))
	if !cred.trusted {
		if cred.user == nil || cred.user.UserId == nil || *cred.user.UserId == "" {
			return a2Forbidden()
		}
		userId = *cred.user.UserId
	}
	col := a2ChallengeRecords()
	if col == nil {
		return hosting.Ok(stj.O("stored", false))
	}
	challengeMu.Lock()
	defer challengeMu.Unlock()
	cur := col.FindOne(func(r *ChallengeRecordDbObject) bool {
		return strings.EqualFold(r.ChallengeId, challenge) && strings.EqualFold(r.UserId, userId)
	})
	now := time.Now().UnixMilli()
	switch {
	case cur == nil:
		col.Insert(&ChallengeRecordDbObject{ChallengeId: challenge, FleetId: fleet, UserId: userId, Record: record, UpdatedAt: now})
	case record < cur.Record:
		cur.Record, cur.FleetId, cur.UpdatedAt = record, fleet, now
		col.Update(cur)
	default:
		return hosting.Ok(stj.O("stored", false, "best", cur.Record))
	}
	return hosting.Ok(stj.O("stored", true, "best", record))
}

// ListChallengeRecords returns the top N, the caller's entry, the entries around the caller, and the caller's friends.
func (s *A2StationDbServer) ListChallengeRecords(c *hosting.Context) hosting.Result {
	challenge := strings.TrimSpace(c.Param("challenge_id"))
	limit := 10
	if n, err := strconv.Atoi(c.Request.GetQueryParameter("limit", "10")); err == nil && n > 0 {
		limit = n
	}
	if limit > 100 {
		limit = 100
	}
	var userId string
	friends := map[string]bool{}
	var body map[string]json.RawMessage
	if len(c.Request.Body()) > 0 && json.Unmarshal(c.Request.Body(), &body) == nil {
		if raw, ok := jsonField(body, "user_id"); ok {
			_ = json.Unmarshal(raw, &userId)
		}
		if raw, ok := jsonField(body, "friends"); ok {
			var fs []string
			if json.Unmarshal(raw, &fs) == nil {
				for _, f := range fs {
					if f = strings.TrimSpace(f); f != "" {
						friends[strings.ToLower(f)] = true
					}
				}
			}
		}
	}
	userId = strings.TrimSpace(userId)

	resp := challengeListResponse{Top: []challengeEntry{}, Nearby: []challengeEntry{}, Friends: []challengeEntry{},
		Me: challengeEntry{UserId: userId, UserName: challengeUserName(userId)}}
	col := a2ChallengeRecords()
	if col == nil || challenge == "" {
		return hosting.Ok(resp)
	}
	recs := col.Find(func(r *ChallengeRecordDbObject) bool { return strings.EqualFold(r.ChallengeId, challenge) })
	sort.SliceStable(recs, func(i, j int) bool {
		if recs[i].Record != recs[j].Record {
			return recs[i].Record < recs[j].Record
		}
		return recs[i].UpdatedAt < recs[j].UpdatedAt
	})
	entries := make([]challengeEntry, len(recs))
	meIdx := -1
	for i, r := range recs {
		entries[i] = challengeEntry{UserId: r.UserId, UserName: challengeUserName(r.UserId), Record: r.Record, Rank: i + 1}
		if userId != "" && strings.EqualFold(r.UserId, userId) {
			meIdx = i
		}
		if friends[strings.ToLower(r.UserId)] {
			resp.Friends = append(resp.Friends, entries[i])
		}
	}
	for i := 0; i < len(entries) && i < limit; i++ {
		resp.Top = append(resp.Top, entries[i])
	}
	if meIdx >= 0 {
		resp.Me = entries[meIdx]
		half := limit / 2
		if half < 2 {
			half = 2
		}
		lo, hi := meIdx-half, meIdx+half
		if lo < 0 {
			lo = 0
		}
		if hi >= len(entries) {
			hi = len(entries) - 1
		}
		for i := lo; i <= hi; i++ {
			resp.Nearby = append(resp.Nearby, entries[i])
		}
	}
	return hosting.Ok(resp)
}
