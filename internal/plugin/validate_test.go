package plugin

import "testing"

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
