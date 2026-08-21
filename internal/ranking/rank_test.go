package ranking

import "testing"

func f(v float64) *float64 { return &v }

func TestRankOrdersByTimeAscending(t *testing.T) {
	input := []TeamResult{
		{TeamID: "b", TimeSeconds: f(130.5)},
		{TeamID: "a", TimeSeconds: f(124.11)},
		{TeamID: "c", TimeSeconds: f(126.87)},
	}
	got := Rank(input)
	want := []string{"a", "c", "b"}
	for i, w := range want {
		if got[i].TeamID != w {
			t.Fatalf("position %d: expected %s, got %s", i, w, got[i].TeamID)
		}
	}
}

func TestRankPlacesStatusesAfterTimedTeamsInOrder(t *testing.T) {
	input := []TeamResult{
		{TeamID: "dns-team", Status: StatusDNS},
		{TeamID: "timed", TimeSeconds: f(120)},
		{TeamID: "dsq-team", Status: StatusDSQ},
		{TeamID: "dnf-team", Status: StatusDNF},
	}
	got := Rank(input)
	want := []string{"timed", "dnf-team", "dsq-team", "dns-team"}
	for i, w := range want {
		if got[i].TeamID != w {
			t.Fatalf("position %d: expected %s, got %s", i, w, got[i].TeamID)
		}
	}
}

func TestRankIsStableForTies(t *testing.T) {
	input := []TeamResult{
		{TeamID: "first", TimeSeconds: f(100)},
		{TeamID: "second", TimeSeconds: f(100)},
	}
	got := Rank(input)
	if got[0].TeamID != "first" || got[1].TeamID != "second" {
		t.Fatalf("expected stable order first,second; got %s,%s", got[0].TeamID, got[1].TeamID)
	}
}

func TestRankDoesNotMutateInput(t *testing.T) {
	input := []TeamResult{
		{TeamID: "b", TimeSeconds: f(2)},
		{TeamID: "a", TimeSeconds: f(1)},
	}
	_ = Rank(input)
	if input[0].TeamID != "b" {
		t.Fatalf("expected input left unmodified, got %s at position 0", input[0].TeamID)
	}
}
