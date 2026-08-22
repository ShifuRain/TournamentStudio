package schedule

import "testing"

// mustCreateTestHeat creates a tournament, round, group, and a
// group-heat scheduled onto a freshly created course, for tests in this
// file that only need a valid heat ID to submit results against.
func (r *Repo) mustCreateTestHeat(t *testing.T) int64 {
	t.Helper()
	tournamentID := seedTournament(t, r)
	roundID := seedRound(t, r, tournamentID, 1)
	groupID := seedGroup(t, r, roundID)
	course, err := r.CreateCourse(tournamentID, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	heats, err := r.ScheduleGroupHeats(tournamentID, roundID, []GroupAssignment{{GroupID: groupID, CourseID: course.ID}}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}
	return heats[0].ID
}

func TestSubmitAndListHeatResults(t *testing.T) {
	r := newTestRepo(t)
	heatID := r.mustCreateTestHeat(t)

	time1 := 100.5
	if err := r.SubmitHeatResults(heatID, []HeatResult{
		{TeamID: "t1", TimeSeconds: &time1},
		{TeamID: "t2", Status: "DNF"},
	}); err != nil {
		t.Fatalf("SubmitHeatResults: %v", err)
	}

	results, err := r.ListHeatResults(heatID)
	if err != nil {
		t.Fatalf("ListHeatResults: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSubmitHeatResultsUpsertsOnResubmission(t *testing.T) {
	r := newTestRepo(t)
	heatID := r.mustCreateTestHeat(t)

	first := 130.0
	if err := r.SubmitHeatResults(heatID, []HeatResult{{TeamID: "t1", TimeSeconds: &first}}); err != nil {
		t.Fatalf("SubmitHeatResults: %v", err)
	}
	corrected := 124.11
	if err := r.SubmitHeatResults(heatID, []HeatResult{{TeamID: "t1", TimeSeconds: &corrected}}); err != nil {
		t.Fatalf("SubmitHeatResults (correction): %v", err)
	}

	results, err := r.ListHeatResults(heatID)
	if err != nil {
		t.Fatalf("ListHeatResults: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 row after resubmission, got %d", len(results))
	}
	if *results[0].TimeSeconds != 124.11 {
		t.Fatalf("expected corrected time 124.11, got %v", *results[0].TimeSeconds)
	}
}

func TestListResultsForRoundAggregatesAcrossHeats(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	round10 := seedRound(t, r, tournamentID, 1)
	group1 := seedGroup(t, r, round10)
	group2 := seedGroup(t, r, round10)
	course, err := r.CreateCourse(tournamentID, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	heats, err := r.ScheduleGroupHeats(tournamentID, round10, []GroupAssignment{
		{GroupID: group1, CourseID: course.ID},
		{GroupID: group2, CourseID: course.ID},
	}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}

	t1 := 100.0
	t2 := 110.0
	if err := r.SubmitHeatResults(heats[0].ID, []HeatResult{{TeamID: "t1", TimeSeconds: &t1}}); err != nil {
		t.Fatalf("SubmitHeatResults heat 0: %v", err)
	}
	if err := r.SubmitHeatResults(heats[1].ID, []HeatResult{{TeamID: "t2", TimeSeconds: &t2}}); err != nil {
		t.Fatalf("SubmitHeatResults heat 1: %v", err)
	}

	// A result submitted to a different round's heat must not leak in.
	round11 := seedRound(t, r, tournamentID, 2)
	group3 := seedGroup(t, r, round11)
	otherHeats, err := r.ScheduleGroupHeats(tournamentID, round11, []GroupAssignment{{GroupID: group3, CourseID: course.ID}}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats (other round): %v", err)
	}
	t3 := 999.0
	if err := r.SubmitHeatResults(otherHeats[0].ID, []HeatResult{{TeamID: "t3", TimeSeconds: &t3}}); err != nil {
		t.Fatalf("SubmitHeatResults other round: %v", err)
	}

	results, err := r.ListResultsForRound(round10)
	if err != nil {
		t.Fatalf("ListResultsForRound: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results for round10, got %d: %+v", len(results), results)
	}
}

func TestSetHeatStatus(t *testing.T) {
	r := newTestRepo(t)
	heatID := r.mustCreateTestHeat(t)

	if err := r.SetHeatStatus(heatID, HeatClosed); err != nil {
		t.Fatalf("SetHeatStatus: %v", err)
	}
	h, err := r.GetHeat(heatID)
	if err != nil {
		t.Fatalf("GetHeat: %v", err)
	}
	if h.Status != HeatClosed {
		t.Fatalf("expected status closed, got %q", h.Status)
	}
}

func TestListHeatsForRound(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	round10 := seedRound(t, r, tournamentID, 1)
	group1 := seedGroup(t, r, round10)
	group2 := seedGroup(t, r, round10)
	course, err := r.CreateCourse(tournamentID, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if _, err := r.ScheduleGroupHeats(tournamentID, round10, []GroupAssignment{
		{GroupID: group1, CourseID: course.ID},
		{GroupID: group2, CourseID: course.ID},
	}, nil); err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}
	round11 := seedRound(t, r, tournamentID, 2)
	group3 := seedGroup(t, r, round11)
	if _, err := r.ScheduleGroupHeats(tournamentID, round11, []GroupAssignment{{GroupID: group3, CourseID: course.ID}}, nil); err != nil {
		t.Fatalf("ScheduleGroupHeats (round11): %v", err)
	}

	heats, err := r.ListHeatsForRound(round10)
	if err != nil {
		t.Fatalf("ListHeatsForRound: %v", err)
	}
	if len(heats) != 2 {
		t.Fatalf("expected 2 heats for round10, got %d", len(heats))
	}
}
