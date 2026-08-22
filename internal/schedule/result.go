package schedule

import "database/sql"

const submitHeatResultSQL = `INSERT INTO heat_results (heat_id, team_id, time_seconds, status) VALUES (?, ?, ?, ?)
	 ON CONFLICT(heat_id, team_id) DO UPDATE SET time_seconds = excluded.time_seconds, status = excluded.status`

// SubmitHeatResults writes every result in one transaction, rolling
// back on any error so a batch submission never partially commits.
func (r *Repo) SubmitHeatResults(heatID int64, results []HeatResult) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	for _, res := range results {
		if _, err := tx.Exec(submitHeatResultSQL, heatID, res.TeamID, res.TimeSeconds, nullIfEmptyResult(res.Status)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func nullIfEmptyResult(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *Repo) ListHeatResults(heatID int64) ([]HeatResult, error) {
	rows, err := r.db.Query(`SELECT heat_id, team_id, time_seconds, status FROM heat_results WHERE heat_id = ?`, heatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHeatResults(rows)
}

// ListResultsForRound aggregates every result across every heat
// belonging to roundID (via heats.round_id) -- the source of a round's
// ranking input from Task 5 onward.
func (r *Repo) ListResultsForRound(roundID int64) ([]HeatResult, error) {
	rows, err := r.db.Query(
		`SELECT hr.heat_id, hr.team_id, hr.time_seconds, hr.status
		 FROM heat_results hr
		 JOIN heats h ON h.id = hr.heat_id
		 WHERE h.round_id = ?`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHeatResults(rows)
}

func scanHeatResults(rows *sql.Rows) ([]HeatResult, error) {
	var results []HeatResult
	for rows.Next() {
		var res HeatResult
		var timeSeconds sql.NullFloat64
		var status sql.NullString
		if err := rows.Scan(&res.HeatID, &res.TeamID, &timeSeconds, &status); err != nil {
			return nil, err
		}
		if timeSeconds.Valid {
			v := timeSeconds.Float64
			res.TimeSeconds = &v
		}
		if status.Valid {
			res.Status = status.String
		}
		results = append(results, res)
	}
	return results, rows.Err()
}

func (r *Repo) SetHeatStatus(id int64, status HeatStatus) error {
	_, err := r.db.Exec(`UPDATE heats SET status = ? WHERE id = ?`, string(status), id)
	return err
}

func (r *Repo) ListHeatsForRound(roundID int64) ([]Heat, error) {
	rows, err := r.db.Query(
		`SELECT id, round_id, group_id, division_id, course_id, planned_start, status FROM heats WHERE round_id = ? ORDER BY id`,
		roundID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heats []Heat
	for rows.Next() {
		h, err := scanHeatRow(rows)
		if err != nil {
			return nil, err
		}
		heats = append(heats, h)
	}
	return heats, rows.Err()
}
