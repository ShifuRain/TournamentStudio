CREATE TABLE sessions (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    role TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
