package app

import "testing"

func TestRankWords(t *testing.T) {
	dict := map[string][]string{"a": {"a kill", "a safe", "a trap", "a kill"}, "safe": {"safe mid"}, "mid": {"mid exit"}, "trap": {"trap end"}, "kill": {"kill bad"}}
	observed := map[string]bool{"kill bad": false, "a safe": true}
	r := rankWords("a", dict, observed, map[string]bool{})
	if len(r) != 3 || r[0].Word != "a kill" || r[0].Assessment != "possible_kill" || r[1].Word != "a safe" || !r[1].Verified || r[1].WorstNextMoves != 1 || r[2].Word != "a trap" || r[2].WorstNextMoves != 0 {
		t.Fatalf("unexpected rank: %+v", r)
	}
	r = rankWords("a", dict, observed, map[string]bool{"a kill": true, "mid exit": true})
	if len(r) != 2 || r[0].Assessment != "risk" || r[1].Assessment != "risk" {
		t.Fatalf("used words not excluded: %+v", r)
	}
}
func TestRankWordsSelfLoops(t *testing.T) {
	r := rankWords("a", map[string][]string{"a": {"a a"}}, nil, nil)
	if len(r) != 1 || r[0].OpponentReplies != 0 {
		t.Fatalf("self-loop reused: %+v", r)
	}
}
