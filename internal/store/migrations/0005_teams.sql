CREATE TABLE teams (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tournament_id INTEGER NOT NULL REFERENCES tournaments(id),
    name TEXT NOT NULL,
    club TEXT NOT NULL DEFAULT '',
    extra_fields TEXT NOT NULL DEFAULT '{}'
);
