CREATE TABLE heats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    round_id INTEGER NOT NULL REFERENCES pre_phase_rounds(id),
    group_id INTEGER REFERENCES groups(id),
    division_id INTEGER,
    course_id INTEGER NOT NULL REFERENCES courses(id),
    planned_start TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled'
);
