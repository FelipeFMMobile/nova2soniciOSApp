package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestFreshDatabaseOwnsOnlyItsDomain(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "app.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, table := range []string{"agenda_events", "agenda_operations", "agenda_schema_migrations", "operations", "schema_migrations"} {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil || count != 0 {
			t.Fatal("cross-domain or legacy schema", table, count, err)
		}
	}
}

func TestLegacyNotesLedgerIsImportedWithoutOtherDomains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`CREATE TABLE operations(key TEXT PRIMARY KEY,name TEXT,arguments TEXT,result TEXT,completed_at TEXT);
INSERT INTO operations VALUES
('notes-key','notes.create','{"content":"x","title":"legacy"}','{"status":"created"}','2030-01-01T00:00:00Z'),
('agenda-key','agenda.create_event','{}','{"status":"created"}','2030-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var notes, agenda int
	if err = s.db.QueryRow(`SELECT count(*) FROM notes_operations WHERE key='notes-key'`).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM notes_operations WHERE key='agenda-key'`).Scan(&agenda); err != nil {
		t.Fatal(err)
	}
	if notes != 1 || agenda != 0 {
		t.Fatal("legacy ledger ownership mismatch", notes, agenda)
	}
}
func TestLegacyExtraTablesArePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec("CREATE TABLE legacy_extra(id TEXT); INSERT INTO legacy_extra VALUES('retained')")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var id string
	if err = s.db.QueryRow("SELECT id FROM legacy_extra").Scan(&id); err != nil || id != "retained" {
		t.Fatal(id, err)
	}
}
func TestUnicodeByteLimitAndNoLedgerOnFailure(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	title := ""
	for i := 0; i < 129; i++ {
		title += "é"
	}
	args, _ := json.Marshal(map[string]string{"title": title, "content": "x"})
	if _, err = s.Mutate(context.Background(), "bad", "notes.create", args); err == nil {
		t.Fatal("byte limit bypassed")
	}
	var n int
	if err = s.db.QueryRow("SELECT count(*) FROM notes_operations").Scan(&n); err != nil || n != 0 {
		t.Fatal("failed mutation recorded", n, err)
	}
}
