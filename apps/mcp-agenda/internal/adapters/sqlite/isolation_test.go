package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestFreshDatabaseOwnsOnlyItsDomain(t *testing.T) {
	s, err := OpenAgenda(filepath.Join(t.TempDir(), "app.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, table := range []string{"notes", "notes_operations", "notes_schema_migrations", "operations", "schema_migrations"} {
		var count int
		if err = s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil || count != 0 {
			t.Fatal("cross-domain or legacy schema", table, count, err)
		}
	}
}

func TestLegacyAgendaLedgerIsImportedWithoutOtherDomains(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite")
	s, err := OpenAgenda(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`CREATE TABLE operations(key TEXT PRIMARY KEY,name TEXT,arguments TEXT,result TEXT,completed_at TEXT);
INSERT INTO operations VALUES
('agenda-key','agenda.create_event','{}','{"status":"created"}','2030-01-01T00:00:00Z'),
('notes-key','notes.create','{}','{"status":"created"}','2030-01-01T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenAgenda(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var agenda, notes int
	if err = s.db.QueryRow(`SELECT count(*) FROM agenda_operations WHERE key='agenda-key'`).Scan(&agenda); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM agenda_operations WHERE key='notes-key'`).Scan(&notes); err != nil {
		t.Fatal(err)
	}
	if agenda != 1 || notes != 0 {
		t.Fatal("legacy ledger ownership mismatch", agenda, notes)
	}
}
func TestLegacyExtraTablesArePreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.sqlite")
	s, err := OpenAgenda(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec("CREATE TABLE legacy_extra(id TEXT); INSERT INTO legacy_extra VALUES('retained')")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = OpenAgenda(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var id string
	if err = s.db.QueryRow("SELECT id FROM legacy_extra").Scan(&id); err != nil || id != "retained" {
		t.Fatal(id, err)
	}
}
func TestRejectedMutationHasNoLedger(t *testing.T) {
	s, err := OpenAgenda(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.MutateAgenda(context.Background(), "bad", "agenda.create_event", json.RawMessage(`{"title":"x","start":"ambiguous"}`), false); err == nil {
		t.Fatal("invalid dates accepted")
	}
	var n int
	if err = s.db.QueryRow("SELECT count(*) FROM agenda_operations").Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}
