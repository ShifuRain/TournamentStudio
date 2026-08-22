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

func TestRankSortsUnrecognizedStatusAfterDNS(t *testing.T) {
	input := []TeamResult{
		{TeamID: "dns-team", Status: StatusDNS},
		{TeamID: "garbage-team", Status: Status("bogus")},
		{TeamID: "dsq-team", Status: StatusDSQ},
		{TeamID: "dnf-team", Status: StatusDNF},
		{TeamID: "timed", TimeSeconds: f(120)},
		{TeamID: "empty-status-team", Status: Status("")},
	}
	got := Rank(input)
	// An unrecognized (or empty) status must sort strictly after DNS, the
	// current worst known status — never tied with "has a time" (which
	// would sort it ahead of every correctly-cased DNF/DSQ/DNS team).
	want := []string{"timed", "dnf-team", "dsq-team", "dns-team"}
	for i, w := range want {
		if got[i].TeamID != w {
			t.Fatalf("position %d: expected %s, got %s", i, w, got[i].TeamID)
		}
	}
	lastTwo := []string{got[len(got)-2].TeamID, got[len(got)-1].TeamID}
	if lastTwo[0] != "garbage-team" && lastTwo[0] != "empty-status-team" {
		t.Fatalf("expected the two unrecognized-status teams last, got %v after %v", lastTwo, want)
	}
	if lastTwo[1] != "garbage-team" && lastTwo[1] != "empty-status-team" {
		t.Fatalf("expected the two unrecognized-status teams last, got %v after %v", lastTwo, want)
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
