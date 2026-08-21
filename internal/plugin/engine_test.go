package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestPlugin(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("write test plugin: %v", err)
	}
}

func findSport(e *Engine, id string) *SportPlugin {
	for _, sp := range e.Sports() {
		if sp.ID == id {
			return sp
		}
	}
	return nil
}

func findTournamentType(e *Engine, id string) *TournamentTypePlugin {
	for _, ttp := range e.TournamentTypes() {
		if ttp.ID == id {
			return ttp
		}
	}
	return nil
}

func TestLoadRegistersSportPlugin(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "sport.lua", `
return {
  id = "test-sport",
  display_name = "Test Sport",
  compatible_tournament_types = {"test-format"},
  roster_fields = {
    {key = "boat_class", label = "Boat class", required = false},
  },
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	sp := findSport(e, "test-sport")
	if sp == nil {
		t.Fatalf("expected test-sport to register")
	}
	if sp.DisplayName != "Test Sport" {
		t.Fatalf("expected display name Test Sport, got %s", sp.DisplayName)
	}
	if len(sp.CompatibleTournamentTypes) != 1 || sp.CompatibleTournamentTypes[0] != "test-format" {
		t.Fatalf("unexpected compatible tournament types: %v", sp.CompatibleTournamentTypes)
	}
	if len(sp.RosterFields) != 1 || sp.RosterFields[0].Key != "boat_class" {
		t.Fatalf("unexpected roster fields: %v", sp.RosterFields)
	}
}

func TestLoadRegistersTournamentTypePlugin(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "format.lua", `
return {
  id = "test-format",
  compatible_sports = {"test-sport"},
  next_round_groups = function(groups) return {} end,
  division_cuts = function(ranked, cuts) return {} end,
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	ttp := findTournamentType(e, "test-format")
	if ttp == nil {
		t.Fatalf("expected test-format to register")
	}
	if len(ttp.CompatibleSports) != 1 || ttp.CompatibleSports[0] != "test-sport" {
		t.Fatalf("unexpected compatible sports: %v", ttp.CompatibleSports)
	}
}

func TestLoadSkipsMalformedPluginWithoutFailing(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "broken.lua", `this is not valid lua {{{`)
	writeTestPlugin(t, dir, "good.lua", `
return {
  id = "good-sport",
  compatible_tournament_types = {},
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	if findSport(e, "good-sport") == nil {
		t.Fatalf("expected good-sport to still register despite broken.lua")
	}
}

func TestLoadTournamentTypeRequiresFunctions(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "incomplete.lua", `
return {
  id = "incomplete-format",
  compatible_sports = {"test-sport"},
  next_round_groups = function(groups) return {} end,
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	if findTournamentType(e, "incomplete-format") != nil {
		t.Fatalf("expected incomplete-format to be skipped (missing division_cuts)")
	}
}

func TestLoadNonexistentExternalDirDoesNotError(t *testing.T) {
	e, err := Load(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()
}

func TestExternalPluginOverridesBundledOnSameID(t *testing.T) {
	dir := t.TempDir()
	writeTestPlugin(t, dir, "dragonboat.lua", `
return {
  id = "dragonboat",
  display_name = "Overridden Dragonboat",
  compatible_tournament_types = {"timed-heats-reseeding"},
}
`)

	e, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	defer e.Close()

	sp := findSport(e, "dragonboat")
	if sp == nil {
		t.Fatalf("expected dragonboat to still be registered")
	}
	if sp.DisplayName != "Overridden Dragonboat" {
		t.Fatalf("expected external plugin to override bundled one, got display name %q", sp.DisplayName)
	}
}

// TestSandboxDeniesDangerousGlobals verifies the per-plugin VM never exposes
// os, io, require, load, dofile, loadstring, loadfile, or module — the
// filesystem/process/module-loading escape hatches gopher-lua's base
// library installs by default. Each script only calls error(...) if the
// capability under test turned out to be reachable or callable, so a nil
// DoString error means the sandbox held.
func TestSandboxDeniesDangerousGlobals(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"os table absent", `if os ~= nil then error("os is reachable") end`},
		{"io table absent", `if io ~= nil then error("io is reachable") end`},
		{"require absent", `if require ~= nil then error("require is reachable") end`},
		{"load absent", `if load ~= nil then error("load is reachable") end`},
		{"loadstring absent", `if loadstring ~= nil then error("loadstring is reachable") end`},
		{"loadfile absent", `if loadfile ~= nil then error("loadfile is reachable") end`},
		{"dofile absent", `if dofile ~= nil then error("dofile is reachable") end`},
		{"module absent", `if module ~= nil then error("module is reachable") end`},
		{"os.execute call fails", `local ok = pcall(function() return os.execute("echo pwned") end); if ok then error("os.execute succeeded") end`},
		{"io.open call fails", `local ok = pcall(function() return io.open("/etc/passwd") end); if ok then error("io.open succeeded") end`},
		// Sanity: the libraries plugins are meant to have still work.
		{"string still works", `if string.upper("a") ~= "A" then error("string lib broken") end`},
		{"table still works", `local t = {1,2,3}; table.insert(t, 4); if #t ~= 4 then error("table lib broken") end`},
		{"math still works", `if math.floor(1.9) ~= 1 then error("math lib broken") end`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			L := newSandboxedState()
			defer L.Close()
			if err := L.DoString(c.src); err != nil {
				t.Fatalf("sandbox leak detected: %v", err)
			}
		})
	}
}
