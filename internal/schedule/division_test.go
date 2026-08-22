package schedule

import "testing"

func TestCreateAndListDivisionsForRound(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	round10 := seedRound(t, r, tournamentID, 1)

	created, err := r.CreateDivisions(tournamentID, round10, []NewDivision{
		{Name: "Gold Final", TeamIDs: []string{"t1", "t2"}},
		{Name: "Final", TeamIDs: []string{"t3", "t4", "t5"}},
	})
	if err != nil {
		t.Fatalf("CreateDivisions: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 divisions, got %d", len(created))
	}
	if created[0].ID == 0 || created[1].ID == 0 {
		t.Fatalf("expected non-zero IDs, got %+v", created)
	}
	if created[0].RoundID != round10 || created[0].TournamentID != tournamentID {
		t.Fatalf("unexpected division: %+v", created[0])
	}

	listed, err := r.ListDivisionsForRound(round10)
	if err != nil {
		t.Fatalf("ListDivisionsForRound: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 divisions listed, got %d", len(listed))
	}
	if len(listed[0].TeamIDs) != 2 || len(listed[1].TeamIDs) != 3 {
		t.Fatalf("unexpected team_ids round trip: %+v", listed)
	}
}

func TestGetDivisionNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetDivision(999); err != ErrDivisionNotFound {
		t.Fatalf("expected ErrDivisionNotFound, got %v", err)
	}
}

func TestListDivisionsForRoundScopesToRound(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	round10 := seedRound(t, r, tournamentID, 1)
	round11 := seedRound(t, r, tournamentID, 2)

	if _, err := r.CreateDivisions(tournamentID, round10, []NewDivision{{Name: "Final", TeamIDs: []string{"t1"}}}); err != nil {
		t.Fatalf("CreateDivisions round10: %v", err)
	}
	if _, err := r.CreateDivisions(tournamentID, round11, []NewDivision{{Name: "Final", TeamIDs: []string{"t2"}}}); err != nil {
		t.Fatalf("CreateDivisions round11: %v", err)
	}

	listed, err := r.ListDivisionsForRound(round10)
	if err != nil {
		t.Fatalf("ListDivisionsForRound: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 division for round10, got %d", len(listed))
	}
}
