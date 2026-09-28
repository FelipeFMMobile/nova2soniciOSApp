package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path/filepath"
	"strings"
	"stsmodel.local/mcp-agenda/internal/scheduling"
	"stsmodel.local/mcp-agenda/migrations"
	"time"
)

const AgendaTimezone = scheduling.AgendaTimezone

type AgendaEvent = scheduling.AgendaEvent
type AgendaInput = scheduling.AgendaInput
type Agenda struct{ db *sql.DB }

func OpenAgenda(path string) (*Agenda, error) {
	if path == "" {
		return nil, errors.New("database path required")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		f.Close()
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(migrations.Schema)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err = migrateLegacyLedger(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Agenda{db: db}, nil
}

func migrateLegacyLedger(db *sql.DB) error {
	var exists int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='operations'`).Scan(&exists); err != nil || exists == 0 {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO agenda_operations(key,name,arguments,result,completed_at)
SELECT key,name,arguments,result,completed_at FROM operations WHERE name LIKE 'agenda.%'`); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT OR IGNORE INTO agenda_schema_migrations VALUES (2)`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Agenda) Close() error { return s.db.Close() }

func agendaConflict(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, a, b time.Time) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM agenda_events WHERE status='active' AND start<? AND end>?`, b.Unix(), a.Unix()).Scan(&n)
	return n > 0, err
}
func (s *Agenda) Events(ctx context.Context, start, end string) ([]AgendaEvent, error) {
	a, b, err := scheduling.Interval(start, end)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,title,start,end,status FROM agenda_events WHERE start<? AND end>? ORDER BY start,id LIMIT 100`, b.Unix(), a.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []AgendaEvent{}
	for rows.Next() {
		var e AgendaEvent
		var x, y int64
		if err = rows.Scan(&e.ID, &e.Title, &x, &y, &e.Status); err != nil {
			return nil, err
		}
		e.Start = time.Unix(x, 0).UTC().Format(time.RFC3339)
		e.End = time.Unix(y, 0).UTC().Format(time.RFC3339)
		e.LocalTimes()
		events = append(events, e)
	}
	return events, rows.Err()
}
func (s *Agenda) Slots(ctx context.Context, i AgendaInput) ([]AgendaEvent, error) {
	a, b, err := scheduling.Interval(i.Start, i.End)
	if err != nil {
		return nil, err
	}
	if i.Duration < 15 || i.Duration > 240 || i.Duration%15 != 0 {
		return nil, errors.New("invalid_duration")
	}
	slots := []AgendaEvent{}
	duration := time.Duration(i.Duration) * time.Minute
	// Grid starts at the requested instant; bounded 31-day range, max 100 slots.
	for x := a; !x.Add(duration).After(b) && len(slots) < 100; x = x.Add(15 * time.Minute) {
		y := x.Add(duration)
		if !scheduling.Available(x, y) {
			continue
		}
		conflict, err := agendaConflict(ctx, s.db, x, y)
		if err != nil {
			return nil, err
		}
		if !conflict {
			slot := AgendaEvent{Start: x.Format(time.RFC3339), End: y.Format(time.RFC3339), Status: "available"}
			slot.LocalTimes()
			slots = append(slots, slot)
		}
	}
	return slots, nil
}
func (s *Agenda) MutateAgenda(ctx context.Context, key, name string, args json.RawMessage, confirmed bool) (json.RawMessage, error) {
	if key == "" || len(key) > 256 {
		return nil, errors.New("invalid_key")
	}
	var i AgendaInput
	if json.Unmarshal(args, &i) != nil {
		return nil, errors.New("invalid_arguments")
	}
	if name == "agenda.cancel_event" && !confirmed {
		return nil, errors.New("confirmation_required")
	}
	var obj any
	_ = json.Unmarshal(args, &obj)
	canonical, _ := json.Marshal(obj)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var oldName, oldArgs, result string
	err = tx.QueryRowContext(ctx, `SELECT name,arguments,result FROM agenda_operations WHERE key=?`, key).Scan(&oldName, &oldArgs, &result)
	if err == nil {
		if oldName != name || oldArgs != string(canonical) {
			return nil, errors.New("idempotency_conflict")
		}
		return json.RawMessage(result), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var value any
	switch name {
	case "agenda.create_event":
		a, b, err := scheduling.Interval(i.Start, i.End)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(i.Title) == "" || len(i.Title) > 256 {
			return nil, errors.New("invalid_title")
		}
		if !scheduling.Available(a, b) {
			return nil, errors.New("outside_availability")
		}
		conflict, err := agendaConflict(ctx, tx, a, b)
		if err != nil {
			return nil, err
		}
		if conflict {
			return nil, errors.New("slot_conflict")
		}
		event := AgendaEvent{ID: rand.Text(), Title: i.Title, Start: a.Format(time.RFC3339), End: b.Format(time.RFC3339), Status: "active"}
		event.LocalTimes()
		if _, err = tx.ExecContext(ctx, `INSERT INTO agenda_events VALUES(?,?,?,?,?)`, event.ID, event.Title, a.Unix(), b.Unix(), event.Status); err != nil {
			return nil, err
		}
		value = map[string]any{"status": "created", "event": event, "timezone": AgendaTimezone}
	case "agenda.cancel_event":
		change, err := tx.ExecContext(ctx, `UPDATE agenda_events SET status='cancelled' WHERE id=? AND status='active'`, i.ID)
		if err != nil {
			return nil, err
		}
		n, _ := change.RowsAffected()
		if n != 1 {
			return nil, errors.New("event_not_found")
		}
		value = map[string]any{"status": "cancelled", "id": i.ID}
	default:
		return nil, errors.New("unknown_mutation")
	}
	data, _ := json.Marshal(value)
	_, err = tx.ExecContext(ctx, `INSERT INTO agenda_operations VALUES(?,?,?,?,?)`, key, name, string(canonical), string(data), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return data, nil
}

// Fixtures are opt-in and deterministic; production never seeds appointments.
func (s *Agenda) SeedFixtures(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO agenda_events VALUES('fixture-2030','Reunião fictícia',1905166800,1905170400,'active')`)
	return err
}
