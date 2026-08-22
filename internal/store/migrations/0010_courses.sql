CREATE TABLE courses (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    name TEXT NOT NULL,
    heat_interval_seconds INTEGER NOT NULL,
    delay_offset_seconds INTEGER NOT NULL DEFAULT 0
);
