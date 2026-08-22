package schedule

import (
	"database/sql"
	"errors"
	"time"
)

var ErrHeatNotFound = errors.New("heat not found")
var ErrGroupAlreadyScheduled = errors.New("group already has a scheduled heat")
var ErrDivisionAlreadyScheduled = errors.New("division already has a scheduled heat")

// GroupAssignment pairs a round's group with the course it should race
// on, for ScheduleGroupHeats.
type GroupAssignment struct {
	GroupID  int64
	CourseID int64
}

// DivisionAssignment pairs a division with the course it should race
// on, for ScheduleDivisionHeats.
type DivisionAssignment struct {
	DivisionID int64
	CourseID   int64
}

// validateCourseForSchedule confirms courseID belongs to tournamentID
// and returns its HeatIntervalSeconds, or ErrCourseNotFound if either
// check fails. Shared by ScheduleGroupHeats and ScheduleDivisionHeats.
func validateCourseForSchedule(tx *sql.Tx, tournamentID, courseID int64) (int, error) {
	var courseTournamentID int64
	var intervalSeconds int
	row := tx.QueryRow(`SELECT tournament_id, heat_interval_seconds FROM courses WHERE id = ?`, courseID)
	if err := row.Scan(&courseTournamentID, &intervalSeconds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrCourseNotFound
		}
		return 0, err
	}
	if courseTournamentID != tournamentID {
		return 0, ErrCourseNotFound
	}
	return intervalSeconds, nil
}

// ScheduleGroupHeats creates one Heat per assignment, transactionally.
// Every assignment's course must belong to tournamentID and must not
// already have created a heat for that group. Heats on the same course
// within one call are auto-sequenced HeatIntervalSeconds apart, in
// assignments' order; the first heat on a given course starts at
// startAt if provided, else one interval after that course's
// currently-latest heat, else now if the course has no heats yet.
func (r *Repo) ScheduleGroupHeats(tournamentID, roundID int64, assignments []GroupAssignment, startAt *time.Time) ([]Heat, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	nextStart := make(map[int64]time.Time)
	created := make([]Heat, 0, len(assignments))

	for _, a := range assignments {
		intervalSeconds, err := validateCourseForSchedule(tx, tournamentID, a.CourseID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		var existingCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM heats WHERE group_id = ?`, a.GroupID).Scan(&existingCount); err != nil {
			tx.Rollback()
			return nil, err
		}
		if existingCount > 0 {
			tx.Rollback()
			return nil, ErrGroupAlreadyScheduled
		}

		start, ok := nextStart[a.CourseID]
		if !ok {
			start, err = courseAnchor(tx, a.CourseID, startAt)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		nextStart[a.CourseID] = start.Add(time.Duration(intervalSeconds) * time.Second)

		groupID := a.GroupID
		res, err := tx.Exec(
			`INSERT INTO heats (round_id, group_id, division_id, course_id, planned_start, status) VALUES (?, ?, NULL, ?, ?, ?)`,
			roundID, groupID, a.CourseID, start.Format(time.RFC3339), string(HeatScheduled),
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		heatID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		created = append(created, Heat{
			ID: heatID, RoundID: roundID, GroupID: &groupID, CourseID: a.CourseID,
			PlannedStart: start, Status: HeatScheduled,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// ScheduleDivisionHeats creates one Heat per assignment, transactionally,
// the division-heat counterpart to ScheduleGroupHeats. Every
// assignment's course must belong to tournamentID and must not already
// have created a heat for that division; each division's RoundID (the
// round it was cut from) becomes the created heat's RoundID. Same
// same-course auto-sequencing rules as ScheduleGroupHeats.
func (r *Repo) ScheduleDivisionHeats(tournamentID int64, assignments []DivisionAssignment, startAt *time.Time) ([]Heat, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, err
	}

	nextStart := make(map[int64]time.Time)
	created := make([]Heat, 0, len(assignments))

	for _, a := range assignments {
		var divTournamentID, divRoundID int64
		row := tx.QueryRow(`SELECT tournament_id, round_id FROM divisions WHERE id = ?`, a.DivisionID)
		if err := row.Scan(&divTournamentID, &divRoundID); err != nil {
			tx.Rollback()
			if errors.Is(err, sql.ErrNoRows) {
				return nil, ErrDivisionNotFound
			}
			return nil, err
		}
		if divTournamentID != tournamentID {
			tx.Rollback()
			return nil, ErrDivisionNotFound
		}

		intervalSeconds, err := validateCourseForSchedule(tx, tournamentID, a.CourseID)
		if err != nil {
			tx.Rollback()
			return nil, err
		}

		var existingCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM heats WHERE division_id = ?`, a.DivisionID).Scan(&existingCount); err != nil {
			tx.Rollback()
			return nil, err
		}
		if existingCount > 0 {
			tx.Rollback()
			return nil, ErrDivisionAlreadyScheduled
		}

		start, ok := nextStart[a.CourseID]
		if !ok {
			start, err = courseAnchor(tx, a.CourseID, startAt)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
		}
		nextStart[a.CourseID] = start.Add(time.Duration(intervalSeconds) * time.Second)

		divisionID := a.DivisionID
		res, err := tx.Exec(
			`INSERT INTO heats (round_id, group_id, division_id, course_id, planned_start, status) VALUES (?, NULL, ?, ?, ?, ?)`,
			divRoundID, divisionID, a.CourseID, start.Format(time.RFC3339), string(HeatScheduled),
		)
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		heatID, err := res.LastInsertId()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		created = append(created, Heat{
			ID: heatID, RoundID: divRoundID, DivisionID: &divisionID, CourseID: a.CourseID,
			PlannedStart: start, Status: HeatScheduled,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

// courseAnchor determines the base PlannedStart for the first new heat
// scheduled on courseID in one ScheduleGroupHeats/ScheduleDivisionHeats
// call: startAt if the caller supplied one, else one interval after the
// course's currently-latest scheduled heat, else now if the course has
// no heats yet.
func courseAnchor(tx *sql.Tx, courseID int64, startAt *time.Time) (time.Time, error) {
	if startAt != nil {
		return *startAt, nil
	}
	var latest sql.NullString
	if err := tx.QueryRow(`SELECT MAX(planned_start) FROM heats WHERE course_id = ?`, courseID).Scan(&latest); err != nil {
		return time.Time{}, err
	}
	if !latest.Valid {
		return time.Now().UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, latest.String)
	if err != nil {
		return time.Time{}, err
	}
	var intervalSeconds int
	if err := tx.QueryRow(`SELECT heat_interval_seconds FROM courses WHERE id = ?`, courseID).Scan(&intervalSeconds); err != nil {
		return time.Time{}, err
	}
	return t.Add(time.Duration(intervalSeconds) * time.Second), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanHeatRow(row rowScanner) (Heat, error) {
	var h Heat
	var groupID, divisionID sql.NullInt64
	var plannedStart, status string
	if err := row.Scan(&h.ID, &h.RoundID, &groupID, &divisionID, &h.CourseID, &plannedStart, &status); err != nil {
		return Heat{}, err
	}
	if groupID.Valid {
		v := groupID.Int64
		h.GroupID = &v
	}
	if divisionID.Valid {
		v := divisionID.Int64
		h.DivisionID = &v
	}
	t, err := time.Parse(time.RFC3339, plannedStart)
	if err != nil {
		return Heat{}, err
	}
	h.PlannedStart = t
	h.Status = HeatStatus(status)
	return h, nil
}

func (r *Repo) UpdateHeatPlannedStart(id int64, start time.Time) (*Heat, error) {
	res, err := r.db.Exec(`UPDATE heats SET planned_start = ? WHERE id = ?`, start.Format(time.RFC3339), id)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrHeatNotFound
	}
	return r.GetHeat(id)
}

func (r *Repo) GetHeat(id int64) (*Heat, error) {
	row := r.db.QueryRow(
		`SELECT id, round_id, group_id, division_id, course_id, planned_start, status FROM heats WHERE id = ?`,
		id,
	)
	h, err := scanHeatRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrHeatNotFound
		}
		return nil, err
	}
	return &h, nil
}
