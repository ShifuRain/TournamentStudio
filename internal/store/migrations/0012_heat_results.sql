CREATE TABLE heat_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    heat_id INTEGER NOT NULL REFERENCES heats(id),
    team_id TEXT NOT NULL,
    time_seconds REAL,
    status TEXT,
    UNIQUE(heat_id, team_id)
);
