package schedule

import (
	"database/sql"
	"errors"
)

var ErrCourseNotFound = errors.New("course not found")

func (r *Repo) CreateCourse(tournamentID int64, name string, heatIntervalSeconds int) (*Course, error) {
	res, err := r.db.Exec(
		`INSERT INTO courses (tournament_id, name, heat_interval_seconds, delay_offset_seconds) VALUES (?, ?, ?, 0)`,
		tournamentID, name, heatIntervalSeconds,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Course{ID: id, TournamentID: tournamentID, Name: name, HeatIntervalSeconds: heatIntervalSeconds, DelayOffsetSeconds: 0}, nil
}

func (r *Repo) ListCourses(tournamentID int64) ([]Course, error) {
	rows, err := r.db.Query(
		`SELECT id, tournament_id, name, heat_interval_seconds, delay_offset_seconds FROM courses WHERE tournament_id = ? ORDER BY id`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var courses []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.TournamentID, &c.Name, &c.HeatIntervalSeconds, &c.DelayOffsetSeconds); err != nil {
			return nil, err
		}
		courses = append(courses, c)
	}
	return courses, rows.Err()
}

func (r *Repo) GetCourse(id int64) (*Course, error) {
	row := r.db.QueryRow(
		`SELECT id, tournament_id, name, heat_interval_seconds, delay_offset_seconds FROM courses WHERE id = ?`,
		id,
	)
	var c Course
	if err := row.Scan(&c.ID, &c.TournamentID, &c.Name, &c.HeatIntervalSeconds, &c.DelayOffsetSeconds); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCourseNotFound
		}
		return nil, err
	}
	return &c, nil
}

type CourseUpdate struct {
	Name                *string
	HeatIntervalSeconds *int
	DelayOffsetSeconds  *int
}

func (r *Repo) UpdateCourse(id int64, upd CourseUpdate) (*Course, error) {
	c, err := r.GetCourse(id)
	if err != nil {
		return nil, err
	}
	if upd.Name != nil {
		c.Name = *upd.Name
	}
	if upd.HeatIntervalSeconds != nil {
		c.HeatIntervalSeconds = *upd.HeatIntervalSeconds
	}
	if upd.DelayOffsetSeconds != nil {
		c.DelayOffsetSeconds = *upd.DelayOffsetSeconds
	}
	if _, err := r.db.Exec(
		`UPDATE courses SET name = ?, heat_interval_seconds = ?, delay_offset_seconds = ? WHERE id = ?`,
		c.Name, c.HeatIntervalSeconds, c.DelayOffsetSeconds, id,
	); err != nil {
		return nil, err
	}
	return c, nil
}
