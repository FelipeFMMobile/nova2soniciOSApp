PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS agenda_schema_migrations (version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS agenda_events(id TEXT PRIMARY KEY,title TEXT NOT NULL,start INTEGER NOT NULL,end INTEGER NOT NULL,status TEXT NOT NULL CHECK(status IN ('active','cancelled')));
CREATE TABLE IF NOT EXISTS agenda_operations (key TEXT PRIMARY KEY, name TEXT NOT NULL, arguments TEXT NOT NULL, result TEXT NOT NULL, completed_at TEXT NOT NULL);
INSERT OR IGNORE INTO agenda_schema_migrations VALUES (1);
