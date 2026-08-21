CREATE TABLE pre_phase_rounds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    round_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'open'
);
