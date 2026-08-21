package plugin

import (
	"testing"

	"tournamentstudio/internal/ranking"
)

// f returns a pointer to v, for building ranking.TeamResult literals inline.
func f(v float64) *float64 { return &v }

func loadTimedHeatsReseeding(t *testing.T) *TournamentTypePlugin {
	t.Helper()
	e, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(e.Close)

	ttp := findTournamentType(e, "timed-heats-reseeding")
	if ttp == nil {
		t.Fatalf("timed-heats-reseeding plugin not found")
	}
	return ttp
}

func assertSameMembers(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	seen := make(map[string]bool, len(got))
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Fatalf("expected %v to contain %s, got %v", want, w, got)
		}
	}
}

func TestNextRoundGroupsCrossSwapHalves(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	groups := [][]ranking.TeamResult{
		{
			{TeamID: "A1", TimeSeconds: f(120)},
			{TeamID: "A2", TimeSeconds: f(121)},
			{TeamID: "A3", TimeSeconds: f(122)},
			{TeamID: "A4", TimeSeconds: f(123)},
		},
		{
			{TeamID: "B1", TimeSeconds: f(120)},
			{TeamID: "B2", TimeSeconds: f(121)},
			{TeamID: "B3", TimeSeconds: f(122)},
			{TeamID: "B4", TimeSeconds: f(123)},
		},
	}

	got, err := ttp.NextRoundGroups(groups)
	if err != nil {
		t.Fatalf("NextRoundGroups: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 new groups, got %d", len(got))
	}

	assertSameMembers(t, got[0], []string{"A1", "A2", "B3", "B4"})
	assertSameMembers(t, got[1], []string{"A3", "A4", "B1", "B2"})
}

func TestNextRoundGroupsThreeGroupsConservesAllTeams(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	groups := [][]ranking.TeamResult{
		{{TeamID: "A1", TimeSeconds: f(1)}, {TeamID: "A2", TimeSeconds: f(2)}, {TeamID: "A3", TimeSeconds: f(3)}},
		{{TeamID: "B1", TimeSeconds: f(1)}, {TeamID: "B2", TimeSeconds: f(2)}, {TeamID: "B3", TimeSeconds: f(3)}},
		{{TeamID: "C1", TimeSeconds: f(1)}, {TeamID: "C2", TimeSeconds: f(2)}, {TeamID: "C3", TimeSeconds: f(3)}},
	}

	got, err := ttp.NextRoundGroups(groups)
	if err != nil {
		t.Fatalf("NextRoundGroups: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 new groups, got %d", len(got))
	}

	all := make(map[string]bool)
	for _, group := range got {
		for _, id := range group {
			if all[id] {
				t.Fatalf("team %s appeared in more than one new group", id)
			}
			all[id] = true
		}
	}
	for _, id := range []string{"A1", "A2", "A3", "B1", "B2", "B3", "C1", "C2", "C3"} {
		if !all[id] {
			t.Fatalf("team %s missing from reseeded groups", id)
		}
	}
}

func TestNextRoundGroupsPlacesNonFinishersInSlowestTier(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	groups := [][]ranking.TeamResult{
		{
			{TeamID: "A1", TimeSeconds: f(120)},
			{TeamID: "A2", TimeSeconds: f(121)},
			{TeamID: "A-dnf", Status: ranking.StatusDNF},
			{TeamID: "A-dns", Status: ranking.StatusDNS},
		},
		{
			{TeamID: "B1", TimeSeconds: f(120)},
			{TeamID: "B2", TimeSeconds: f(121)},
			{TeamID: "B3", TimeSeconds: f(122)},
			{TeamID: "B4", TimeSeconds: f(123)},
		},
	}

	got, err := ttp.NextRoundGroups(groups)
	if err != nil {
		t.Fatalf("NextRoundGroups: %v", err)
	}

	assertSameMembers(t, got[1], []string{"A-dnf", "A-dns", "B1", "B2"})
}
