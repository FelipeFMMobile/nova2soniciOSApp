package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	_ "time/tzdata"
)

const AgendaTimezone = "America/Sao_Paulo"

type AgendaEvent struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Start  string `json:"start"`
	End    string `json:"end"`
	Status string `json:"status"`
}
type AgendaInput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Duration int    `json:"duration_minutes"`
}
type Agenda struct{ *Store }

func OpenAgenda(path string) (*Agenda, error) {
	s, err := Open(path)
	if err != nil {
		return nil, err
	}
	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS agenda_events(id TEXT PRIMARY KEY,title TEXT NOT NULL,start INTEGER NOT NULL,end INTEGER NOT NULL,status TEXT NOT NULL CHECK(status IN ('active','cancelled')));`)
	if err != nil {
		s.Close()
		return nil, err
	}
	return &Agenda{s}, nil
}

// Absolute instants are mandatory. Local free-text interpretation belongs to
// conversation; ambiguous dates must be clarified, never guessed by storage.
func agendaInterval(start, end string) (time.Time, time.Time, error) {
	a, e1 := time.Parse(time.RFC3339, start)
	b, e2 := time.Parse(time.RFC3339, end)
	if e1 != nil || e2 != nil || !a.Before(b) || b.Sub(a) > 31*24*time.Hour || a.Nanosecond() != 0 || b.Nanosecond() != 0 {
		return a, b, errors.New("invalid_interval")
	}
	return a.UTC(), b.UTC(), nil
}
func available(a, b time.Time) bool {
	loc, _ := time.LoadLocation(AgendaTimezone)
	x, y := a.In(loc), b.In(loc)
	return x.Weekday() != time.Saturday && x.Weekday() != time.Sunday && x.Format("2006-01-02") == y.Format("2006-01-02") && x.Hour() >= 9 && (y.Hour() < 18 || (y.Hour() == 18 && y.Minute() == 0 && y.Second() == 0)) && b.Sub(a) <= 8*time.Hour
}
func agendaConflict(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, a, b time.Time) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM agenda_events WHERE status='active' AND start<? AND end>?`, b.Unix(), a.Unix()).Scan(&n)
	return n > 0, err
}
func (s *Agenda) Events(ctx context.Context, start, end string) ([]AgendaEvent, error) {
	a, b, err := agendaInterval(start, end)
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
		events = append(events, e)
	}
	return events, rows.Err()
}
func (s *Agenda) Slots(ctx context.Context, i AgendaInput) ([]AgendaEvent, error) {
	a, b, err := agendaInterval(i.Start, i.End)
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
		if !available(x, y) {
			continue
		}
		conflict, err := agendaConflict(ctx, s.db, x, y)
		if err != nil {
			return nil, err
		}
		if !conflict {
			slots = append(slots, AgendaEvent{Start: x.Format(time.RFC3339), End: y.Format(time.RFC3339), Status: "available"})
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
	err = tx.QueryRowContext(ctx, `SELECT name,arguments,result FROM operations WHERE key=?`, key).Scan(&oldName, &oldArgs, &result)
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
		a, b, err := agendaInterval(i.Start, i.End)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(i.Title) == "" || len(i.Title) > 256 {
			return nil, errors.New("invalid_title")
		}
		if !available(a, b) {
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
	_, err = tx.ExecContext(ctx, `INSERT INTO operations VALUES(?,?,?,?,?)`, key, name, string(canonical), string(data), time.Now().UTC().Format(time.RFC3339Nano))
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
