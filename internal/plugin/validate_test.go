package plugin

import (
	"testing"
	"time"
)

func TestValidateAcceptsWellFormedSportPlugin(t *testing.T) {
	source := []byte(`
return {
  id = "test-sport",
  display_name = "Test Sport",
  compatible_tournament_types = {"single-elim"},
}
`)
	if err := Validate("test-sport.lua", source); err != nil {
		t.Fatalf("expected valid source to pass, got: %v", err)
	}
}

func TestValidateAcceptsWellFormedTournamentTypePlugin(t *testing.T) {
	source := []byte(`
return {
  id = "test-type",
  compatible_sports = {"test-sport"},
  next_round_groups = function(ranked_groups) return {} end,
  division_cuts = function(ranked_teams, cuts) return {} end,
}
`)
	if err := Validate("test-type.lua", source); err != nil {
		t.Fatalf("expected valid source to pass, got: %v", err)
	}
}

func TestValidateRejectsMalformedLua(t *testing.T) {
	if err := Validate("broken.lua", []byte("this is not valid lua {{{")); err == nil {
		t.Fatal("expected an error for malformed Lua source")
	}
}

func TestValidateRejectsMissingID(t *testing.T) {
	source := []byte(`return { compatible_tournament_types = {"single-elim"} }`)
	if err := Validate("no-id.lua", source); err == nil {
		t.Fatal("expected an error for a plugin table with no id")
	}
}

func TestValidateRejectsUnrecognizedShape(t *testing.T) {
	source := []byte(`return { id = "mystery" }`)
	if err := Validate("mystery.lua", source); err == nil {
		t.Fatal("expected an error for a plugin table with neither known field")
	}
}

// TestValidateTimesOutOnInfiniteLoop exercises the top-level-chunk time
// budget added to loadSource: an uploaded plugin whose body itself never
// returns (as opposed to one whose next_round_groups/division_cuts
// function never returns, covered by
// TestNextRoundGroupsTimesOutOnInfiniteLoop in tournamenttype_test.go)
// must not be able to hang the plugin-upload HTTP handler that calls
// Validate forever.
func TestValidateTimesOutOnInfiniteLoop(t *testing.T) {
	// Shrink the production timeout for the duration of this test only, so
	// it doesn't take pluginCallTimeout's full production value (5s) to
	// observe the abort.
	original := pluginCallTimeout
	pluginCallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { pluginCallTimeout = original })

	source := []byte(`
while true do end
return { id = "never-returns" }
`)

	done := make(chan error, 1)
	go func() {
		done <- Validate("infinite-loop.lua", source)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error for a plugin whose top-level body never returns, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Validate did not return within 2s of a %s timeout; the infinite loop was not aborted", pluginCallTimeout)
	}
}

func TestValidateDoesNotRegisterOnAnyLiveEngine(t *testing.T) {
	dir := t.TempDir()
	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	source := []byte(`
return {
  id = "not-registered",
  display_name = "Not Registered",
  compatible_tournament_types = {"single-elim"},
}
`)
	if err := Validate("not-registered.lua", source); err != nil {
		t.Fatalf("expected valid source to pass, got: %v", err)
	}

	if findSport(e, "not-registered") != nil {
		t.Fatal("Validate must not register the plugin on any pre-existing Engine")
	}
}
