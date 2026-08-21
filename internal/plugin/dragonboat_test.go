package plugin

import "testing"

func TestBundledDragonboatPluginRegisters(t *testing.T) {
	e, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	t.Cleanup(e.Close)

	found := findSport(e, "dragonboat")
	if found == nil {
		t.Fatalf("expected dragonboat sport plugin to register")
	}
	if found.DisplayName != "Dragonboat" {
		t.Fatalf("expected display name Dragonboat, got %s", found.DisplayName)
	}
	if len(found.CompatibleTournamentTypes) != 1 || found.CompatibleTournamentTypes[0] != "timed-heats-reseeding" {
		t.Fatalf("unexpected compatible tournament types: %v", found.CompatibleTournamentTypes)
	}
	if len(found.RosterFields) != 1 || found.RosterFields[0].Key != "boat_class" {
		t.Fatalf("unexpected roster fields: %v", found.RosterFields)
	}
}
