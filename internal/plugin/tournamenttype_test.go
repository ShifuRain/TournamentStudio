package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// infiniteLoopPluginSource is a minimal tournament-type plugin whose
// next_round_groups never returns, to exercise the per-call instruction/
// time budget (pluginCallTimeout) that must stop a runaway plugin from
// hanging the caller and holding the plugin's mutex forever.
const infiniteLoopPluginSource = `
local function next_round_groups(groups)
  while true do end
  return {}
end

local function division_cuts(ranked_teams, cuts)
  return {}
end

return {
  id = "infinite-loop-test-plugin",
  compatible_sports = {"dragonboat"},
  next_round_groups = next_round_groups,
  division_cuts = division_cuts,
}
`

func TestNextRoundGroupsTimesOutOnInfiniteLoop(t *testing.T) {
	// Shrink the production timeout for the duration of this test only,
	// so it doesn't take pluginCallTimeout's full production value (5s)
	// to observe the abort.
	original := pluginCallTimeout
	pluginCallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { pluginCallTimeout = original })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "infinite-loop.lua"), []byte(infiniteLoopPluginSource), 0o644); err != nil {
		t.Fatalf("write fixture plugin: %v", err)
	}

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(e.Close)

	ttp := findTournamentType(e, "infinite-loop-test-plugin")
	if ttp == nil {
		t.Fatalf("infinite-loop-test-plugin not found")
	}

	done := make(chan error, 1)
	go func() {
		_, err := ttp.NextRoundGroups([][]ranking.TeamResult{
			{{TeamID: "t1", TimeSeconds: f(1)}},
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected an error from a plugin call that timed out, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("NextRoundGroups did not return within 2s of a %s timeout; the infinite loop was not aborted", pluginCallTimeout)
	}
}
