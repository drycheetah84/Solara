package ares

import "testing"

func TestMatchWinner(t *testing.T) {
	cases := []struct {
		name  string
		teams []MatchResultTeam
		want  int
	}{
		{"more goals", []MatchResultTeam{{Team: 0, Score: 2}, {Team: 1, Score: 5}}, 1},
		{"level goals, more rounds", []MatchResultTeam{{Team: 0, Score: 3, Rounds: 2}, {Team: 1, Score: 3, Rounds: 1}}, 0},
		{"draw", []MatchResultTeam{{Team: 0, Score: 1, Rounds: 1}, {Team: 1, Score: 1, Rounds: 1}}, -1},
		{"one team", []MatchResultTeam{{Team: 0, Score: 4}}, -1},
	}
	for _, c := range cases {
		if got := matchWinner(c.teams); got != c.want {
			t.Errorf("%s: got %d want %d", c.name, got, c.want)
		}
	}
}
