package schedule

import (
	"path/filepath"
	"testing"

	"tournamentstudio/internal/store"
)

func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return NewRepo(s)
}

// seedTournament inserts a real tournaments row and returns its ID, so
// tests can satisfy courses.tournament_id's foreign-key constraint
// (foreign_keys is on) without going through the HTTP layer.
func seedTournament(t *testing.T, r *Repo) int64 {
	t.Helper()
	res, err := r.db.Exec(`INSERT INTO tournaments (name, sport_plugin_id, tournament_type_id, language, status) VALUES ('Test', 'dragonboat', 'timed-heats-reseeding', 'en', 'draft')`)
	if err != nil {
		t.Fatalf("seed tournament: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seed tournament id: %v", err)
	}
	return id
}

func TestCreateAndListCourses(t *testing.T) {
	r := newTestRepo(t)
	tournamentA := seedTournament(t, r)
	tournamentB := seedTournament(t, r)

	c1, err := r.CreateCourse(tournamentA, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if c1.ID == 0 {
		t.Fatalf("expected a non-zero ID")
	}
	if c1.DelayOffsetSeconds != 0 {
		t.Fatalf("expected new course to default to 0 delay offset, got %d", c1.DelayOffsetSeconds)
	}

	if _, err := r.CreateCourse(tournamentA, "Course B", 240); err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}
	if _, err := r.CreateCourse(tournamentB, "Other Tournament's Course", 300); err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	courses, err := r.ListCourses(tournamentA)
	if err != nil {
		t.Fatalf("ListCourses: %v", err)
	}
	if len(courses) != 2 {
		t.Fatalf("expected 2 courses for tournamentA, got %d", len(courses))
	}
}

func TestGetCourseNotFound(t *testing.T) {
	r := newTestRepo(t)
	if _, err := r.GetCourse(999); err != ErrCourseNotFound {
		t.Fatalf("expected ErrCourseNotFound, got %v", err)
	}
}

func TestUpdateCourse(t *testing.T) {
	r := newTestRepo(t)
	tournamentID := seedTournament(t, r)
	c, err := r.CreateCourse(tournamentID, "Course A", 300)
	if err != nil {
		t.Fatalf("CreateCourse: %v", err)
	}

	newOffset := 900
	updated, err := r.UpdateCourse(c.ID, CourseUpdate{DelayOffsetSeconds: &newOffset})
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if updated.DelayOffsetSeconds != 900 {
		t.Fatalf("expected delay offset 900, got %d", updated.DelayOffsetSeconds)
	}
	if updated.Name != "Course A" || updated.HeatIntervalSeconds != 300 {
		t.Fatalf("expected other fields unchanged, got %+v", updated)
	}

	newName := "Course A (renamed)"
	updated2, err := r.UpdateCourse(c.ID, CourseUpdate{Name: &newName})
	if err != nil {
		t.Fatalf("UpdateCourse: %v", err)
	}
	if updated2.Name != "Course A (renamed)" || updated2.DelayOffsetSeconds != 900 {
		t.Fatalf("expected name updated and prior offset preserved, got %+v", updated2)
	}
}
