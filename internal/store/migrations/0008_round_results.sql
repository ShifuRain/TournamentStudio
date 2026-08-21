CREATE TABLE round_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    team_id TEXT NOT NULL,
    time_seconds REAL,
    status TEXT,
    UNIQUE(round_id, team_id)
);
