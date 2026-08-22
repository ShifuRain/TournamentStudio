package schedule

import (
	"testing"
	"time"
)

// seedRound inserts a real pre_phase_rounds row and returns its ID, so
// tests can satisfy heats.round_id's foreign-key constraint without
// going through the round package (which schedule deliberately does not
// depend on).
func seedRound(t *testing.T, r *Repo, tournamentID int64, roundNumber int) int64 {
	t.Helper()
	res, err := r.db.Exec(`INSERT INTO pre_phase_rounds (tournament_id, round_number, status) VALUES (?, ?, 'open')`, tournamentID, roundNumber)
	if err != nil {
		t.Fatalf("seed round: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed round id: %v", err)
	}
	return id
}

// seedGroup inserts a real groups row and returns its ID, so tests can
// satisfy heats.group_id's foreign-key constraint.
func seedGroup(t *testing.T, r *Repo, roundID int64) int64 {
	t.Helper()
	res, err := r.db.Exec(`INSERT INTO groups (round_id, team_ids) VALUES (?, '[]')`, roundID)
	if err != nil {
		t.Fatalf("seed group: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed group id: %v", err)
	}
	return id
}

func TestScheduleGroupHeatsAutoSequencesOnSameCourse(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	roundID := seedRound(t, r, tournamentID, 1)
	group1 := seedGroup(t, r, roundID)
	group2 := seedGroup(t, r, roundID)
	course, err := r.CreateCourse(tournamentID, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	heats, err := r.ScheduleGroupHeats(tournamentID, roundID, []GroupAssignment{
		{GroupID: group1, CourseID: course.ID},
		{GroupID: group2, CourseID: course.ID},
	}, &start)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}
	if len(heats) != 2 {
		t.Fatalf("expected 2 heats, got %d", len(heats))
	}
	if !heats[0].PlannedStart.Equal(start) {
		t.Fatalf("expected first heat at %v, got %v", start, heats[0].PlannedStart)
	}
	wantSecond := start.Add(300 * time.Second)
	if !heats[1].PlannedStart.Equal(wantSecond) {
		t.Fatalf("expected second heat at %v, got %v", wantSecond, heats[1].PlannedStart)
	}
	if heats[0].RoundID != roundID || *heats[0].GroupID != group1 || heats[0].DivisionID != nil {
		t.Fatalf("unexpected heat shape: %+v", heats[0])
	}
	if heats[0].Status != HeatScheduled {
		t.Fatalf("expected status %q, got %q", HeatScheduled, heats[0].Status)
	}
}

func TestScheduleGroupHeatsContinuesAfterExistingHeatsOnSameCourse(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	round1 := seedRound(t, r, tournamentID, 1)
	round2 := seedRound(t, r, tournamentID, 2)
	group1 := seedGroup(t, r, round1)
	group2 := seedGroup(t, r, round2)
	course, err := r.CreateCourse(tournamentID, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	firstBatch, err := r.ScheduleGroupHeats(tournamentID, round1, []GroupAssignment{{GroupID: group1, CourseID: course.ID}}, &start)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats (round 1): %v", err)
	}

	secondBatch, err := r.ScheduleGroupHeats(tournamentID, round2, []GroupAssignment{{GroupID: group2, CourseID: course.ID}}, nil)
	if err != nil {
		t.Fatalf("ScheduleGroupHeats (round 2): %v", err)
	}
	wantSecond := firstBatch[0].PlannedStart.Add(300 * time.Second)
	if !secondBatch[0].PlannedStart.Equal(wantSecond) {
		t.Fatalf("expected round 2's heat to continue the sequence at %v, got %v", wantSecond, secondBatch[0].PlannedStart)
	}
}

func TestScheduleGroupHeatsRejectsAlreadyScheduledGroup(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	roundID := seedRound(t, r, tournamentID, 1)
	group1 := seedGroup(t, r, roundID)
	course, err := r.CreateCourse(tournamentID, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleGroupHeats(tournamentID, roundID, []GroupAssignment{{GroupID: group1, CourseID: course.ID}}, &start); err != nil {
		t.Fatalf("ScheduleGroupHeats: %v", err)
	}

	if _, err := r.ScheduleGroupHeats(tournamentID, roundID, []GroupAssignment{{GroupID: group1, CourseID: course.ID}}, &start); err != ErrGroupAlreadyScheduled {
		t.Fatalf("expected ErrGroupAlreadyScheduled, got %v", err)
	}
}

func TestScheduleGroupHeatsRejectsUnknownCourse(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	roundID := seedRound(t, r, tournamentID, 1)
	group1 := seedGroup(t, r, roundID)
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleGroupHeats(tournamentID, roundID, []GroupAssignment{{GroupID: group1, CourseID: 999}}, &start); err != ErrCourseNotFound {
		t.Fatalf("expected ErrCourseNotFound, got %v", err)
	}
}

func TestScheduleGroupHeatsRejectsCourseFromAnotherTournament(t *testing.T) {
	r := newTestRepo(t)
	tournamentA := seedTournament(t, r)
	tournamentB := seedTournament(t, r)
	roundID := seedRound(t, r, tournamentA, 1)
	group1 := seedGroup(t, r, roundID)
	course, err := r.CreateCourse(tournamentB, "Someone Else's Course", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	if _, err := r.ScheduleGroupHeats(tournamentA, roundID, []GroupAssignment{{GroupID: group1, CourseID: course.ID}}, &start); err != ErrCourseNotFound {
		t.Fatalf("expected ErrCourseNotFound for a cross-tournament course, got %v", err)
	}
}

func TestGetHeatNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetHeat(999); err != ErrHeatNotFound {
		t.Fatalf("expected ErrHeatNotFound, got %v", err)
	}
}
