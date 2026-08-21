CREATE TABLE groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    team_ids TEXT NOT NULL
);
