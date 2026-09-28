PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS notes_schema_migrations (version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS notes (id TEXT PRIMARY KEY, title TEXT NOT NULL, content TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS notes_operations (key TEXT PRIMARY KEY, name TEXT NOT NULL, arguments TEXT NOT NULL, result TEXT NOT NULL, completed_at TEXT NOT NULL);
INSERT OR IGNORE INTO notes_schema_migrations VALUES (1);
