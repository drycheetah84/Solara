// Solara addition: arena match results.
//
// A game server reports every game that finishes in one of its module slots (a Tackleball arena, a Driftplex...):
//
//	POST /v1/deployments/{deployment_id}/match_result   (station API :78, game-server credential only)
//	     {slot_id, started_ms_ago, duration_ms, teams:[{team, score, rounds}], players:[{name, team}]}
//
// and the dashboard serves them to anything that may read the station (an admin, or an API key with stations.read
// scoped to that station -- a Discord ranked bot, say):
//
//	GET  /api/stations/{station_id}/matches?since=<seq>&limit=<n>&slot=<slot id>
//	     -> {"matches": [...oldest first, seq > since], "latest": <highest seq stored>}
//
// Poll with since = the last seq seen: each result has a seq that only ever grows, so nothing is missed or repeated.
// Results are kept per station, newest kMatchResultsKept.
package ares

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"solara/internal/ares/models"
	"solara/internal/docdb"
	"solara/internal/hosting"
	"solara/internal/stj"
)

const (
	kMatchResultsKept    = 500 // per station
	kMatchResultsPageMax = 200
)

type MatchResultTeam struct {
	Team   int `json:"team"`
	Score  int `json:"score"`
	Rounds int `json:"rounds"`
}

type MatchResultPlayer struct {
	Name string `json:"name"`
	Team int    `json:"team"`
}

// MatchResultDbObject is one finished game.
type MatchResultDbObject struct {
	docdb.AstraDbObject
	MatchId      string              `json:"match_id"`
	Seq          int64               `json:"seq"`
	StationId    string              `json:"station_id"`
	DeploymentId string              `json:"deployment_id"`
	ServerName   string              `json:"server_name"`
	SlotId       string              `json:"slot_id"`
	StartedAt    int64               `json:"started_at"` // unix ms (server-reported start, aligned to our clock)
	EndedAt      int64               `json:"ended_at"`   // unix ms, when the result arrived
	DurationMs   int64               `json:"duration_ms"`
	Teams        []MatchResultTeam   `json:"teams"`
	Players      []MatchResultPlayer `json:"players"`
	Winner       int                 `json:"winner"` // team index; -1 = draw
}

func NewMatchResultDbObject() *MatchResultDbObject { return &MatchResultDbObject{} }

func matchResults() *docdb.Collection[MatchResultDbObject] {
	return docdb.GetCollection(Database, true, NewMatchResultDbObject)
}

var matchResultsMu sync.Mutex // seq allocation + pruning

// matchWinner: most goals; on level goals, most rounds won; else a draw (-1).
func matchWinner(teams []MatchResultTeam) int {
	if len(teams) < 2 {
		return -1
	}
	best, bestScore, bestRounds, tie := teams[0].Team, teams[0].Score, teams[0].Rounds, false
	for _, t := range teams[1:] {
		switch {
		case t.Score > bestScore || (t.Score == bestScore && t.Rounds > bestRounds):
			best, bestScore, bestRounds, tie = t.Team, t.Score, t.Rounds, false
		case t.Score == bestScore && t.Rounds == bestRounds:
			tie = true
		}
	}
	if tie {
		return -1
	}
	return best
}

// PostMatchResult is POST /v1/deployments/{deployment_id}/match_result (station API).
func (s *A2StationDbServer) PostMatchResult(c *hosting.Context) hosting.Result {
	// Only a game server may report: a player key could otherwise forge ranked results.
	cred := a2Credential(c, "A2StationDb Solara/PostMatchResult", a2ServerToken(c))
	if !cred.trusted {
		return a2Forbidden()
	}
	depId := c.Param("deployment_id")
	var body struct {
		SlotId       string              `json:"slot_id"`
		StartedMsAgo int64               `json:"started_ms_ago"`
		DurationMs   int64               `json:"duration_ms"`
		Teams        []MatchResultTeam   `json:"teams"`
		Players      []MatchResultPlayer `json:"players"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		return hosting.BadRequest(stj.O("error", "body must be {slot_id, started_ms_ago, duration_ms, teams, players}"))
	}
	body.SlotId = strings.TrimSpace(body.SlotId)
	if body.SlotId == "" || len(body.SlotId) > 128 || len(body.Teams) < 2 || len(body.Teams) > 8 || len(body.Players) > 64 {
		return hosting.BadRequest(stj.O("error", "slot_id and 2-8 teams required (at most 64 players)"))
	}
	stationId, serverName := "", ""
	if deps := a2Deployments(); deps != nil {
		if d := deps.FindOne(func(x *models.DeploymentDbObject) bool { return a2Deref(x.DeploymentId) == depId }); d != nil {
			stationId, serverName = a2Deref(d.StationId), a2Deref(d.DeploymentName)
		}
	}
	if stationId == "" {
		return hosting.NotFound(stj.O("error", "unknown deployment", "deployment_id", depId))
	}
	players := make([]MatchResultPlayer, 0, len(body.Players))
	for _, p := range body.Players {
		if n := strings.TrimSpace(p.Name); n != "" && len(n) <= 64 {
			players = append(players, MatchResultPlayer{Name: n, Team: p.Team})
		}
	}
	col := matchResults()
	if col == nil {
		return hosting.Ok(stj.O("success", false, "error", "match results store unavailable"))
	}
	now := time.Now().UnixMilli()
	started := now - body.StartedMsAgo
	if body.StartedMsAgo <= 0 || body.StartedMsAgo > 24*3600*1000 {
		started = now - body.DurationMs
	}

	matchResultsMu.Lock()
	defer matchResultsMu.Unlock()
	seq := int64(0)
	for _, r := range col.FindAll() {
		if r.Seq > seq {
			seq = r.Seq
		}
	}
	seq++
	rec := &MatchResultDbObject{
		MatchId: strconv.FormatInt(now, 36) + "-" + strconv.FormatInt(seq, 36), Seq: seq,
		StationId: stationId, DeploymentId: depId, ServerName: serverName, SlotId: body.SlotId,
		StartedAt: started, EndedAt: now, DurationMs: body.DurationMs,
		Teams: body.Teams, Players: players, Winner: matchWinner(body.Teams),
	}
	col.Insert(rec)

	// Keep the newest kMatchResultsKept of this station.
	mine := col.Find(func(r *MatchResultDbObject) bool { return r.StationId == stationId })
	if len(mine) > kMatchResultsKept {
		sort.Slice(mine, func(i, j int) bool { return mine[i].Seq > mine[j].Seq })
		cut := mine[kMatchResultsKept-1].Seq
		col.DeleteMany(func(r *MatchResultDbObject) bool { return r.StationId == stationId && r.Seq < cut })
	}
	return hosting.Ok(stj.O("success", true, "match_id", rec.MatchId, "seq", rec.Seq, "winner", rec.Winner))
}

func matchResultView(r *MatchResultDbObject) stj.Obj {
	iso := func(ms int64) any {
		if ms == 0 {
			return nil
		}
		return time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	teams := r.Teams
	if teams == nil {
		teams = []MatchResultTeam{}
	}
	players := r.Players
	if players == nil {
		players = []MatchResultPlayer{}
	}
	return stj.O("match_id", r.MatchId, "seq", r.Seq, "station_id", r.StationId, "deployment_id", r.DeploymentId,
		"server_name", r.ServerName, "slot_id", r.SlotId, "started_at", iso(r.StartedAt), "ended_at", iso(r.EndedAt),
		"duration_ms", r.DurationMs, "teams", teams, "players", players, "winner", r.Winner)
}

// GetStationMatches is GET /api/stations/{station_id}/matches (dashboard).
func (s *AresDashboardServer) GetStationMatches(c *hosting.Context) hosting.Result {
	stationId := c.Param("station_id")
	since, _ := strconv.ParseInt(c.Request.GetQueryParameter("since", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.Request.GetQueryParameter("limit", "50"))
	if limit < 1 {
		limit = 50
	}
	if limit > kMatchResultsPageMax {
		limit = kMatchResultsPageMax
	}
	slot := strings.TrimSpace(c.Request.GetQueryParameter("slot", ""))
	out := []any{}
	latest := int64(0)
	if col := matchResults(); col != nil {
		rows := col.Find(func(r *MatchResultDbObject) bool { return strings.EqualFold(r.StationId, stationId) })
		sort.Slice(rows, func(i, j int) bool { return rows[i].Seq < rows[j].Seq })
		for _, r := range rows {
			if r.Seq > latest {
				latest = r.Seq
			}
			if r.Seq <= since || (slot != "" && !strings.EqualFold(r.SlotId, slot)) {
				continue
			}
			if len(out) < limit {
				out = append(out, matchResultView(r))
			}
		}
	}
	return dashJson(200, stj.O("station_id", stationId, "matches", out, "latest", latest))
}
