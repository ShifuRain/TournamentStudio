package plugin

import "testing"

func TestDivisionCutsFillsInOrder(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	ranked := []string{"t1", "t2", "t3", "t4", "t5", "t6"}
	cuts := []Cut{
		{Name: "Gold Final", Size: 3},
		{Name: "Silver Final", Size: 3},
	}

	got, err := ttp.DivisionCuts(ranked, cuts)
	if err != nil {
		t.Fatalf("DivisionCuts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 divisions, got %d", len(got))
	}
	if got[0].Name != "Gold Final" || len(got[0].TeamIDs) != 3 || got[0].TeamIDs[0] != "t1" {
		t.Fatalf("unexpected first division: %+v", got[0])
	}
	if got[1].Name != "Silver Final" || len(got[1].TeamIDs) != 3 || got[1].TeamIDs[0] != "t4" {
		t.Fatalf("unexpected second division: %+v", got[1])
	}
}

func TestDivisionCutsAddsImplicitFinalForRemainder(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	ranked := []string{"t1", "t2", "t3", "t4", "t5"}
	cuts := []Cut{
		{Name: "Gold Final", Size: 3},
	}

	got, err := ttp.DivisionCuts(ranked, cuts)
	if err != nil {
		t.Fatalf("DivisionCuts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 divisions (including implicit), got %d", len(got))
	}
	if got[1].Name != "Final" || len(got[1].TeamIDs) != 2 {
		t.Fatalf("unexpected implicit division: %+v", got[1])
	}
}

func TestDivisionCutsTruncatesOverflow(t *testing.T) {
	ttp := loadTimedHeatsReseeding(t)

	ranked := []string{"t1", "t2", "t3"}
	cuts := []Cut{
		{Name: "Gold Final", Size: 10},
	}

	got, err := ttp.DivisionCuts(ranked, cuts)
	if err != nil {
		t.Fatalf("DivisionCuts: %v", err)
	}
	if len(got) != 1 || len(got[0].TeamIDs) != 3 {
		t.Fatalf("expected single division truncated to 3 teams, got %+v", got)
	}
}
