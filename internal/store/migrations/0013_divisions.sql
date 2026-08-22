CREATE TABLE divisions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    name TEXT NOT NULL,
    team_ids TEXT NOT NULL
);
