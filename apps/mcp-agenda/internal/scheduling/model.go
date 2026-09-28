package scheduling

import (
	"errors"
	"time"
	_ "time/tzdata"
)

const AgendaTimezone = "America/Sao_Paulo"

type AgendaEvent struct {
	StartLocal string `json:"start_local"`
	EndLocal   string `json:"end_local"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Status     string `json:"status"`
}
type AgendaInput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Duration int    `json:"duration_minutes"`
}

// Absolute instants are mandatory. Local free-text interpretation belongs to
// conversation; ambiguous dates must be clarified, never guessed by storage.
func Interval(start, end string) (time.Time, time.Time, error) {
	a, e1 := time.Parse(time.RFC3339, start)
	b, e2 := time.Parse(time.RFC3339, end)
	if e1 != nil || e2 != nil || !a.Before(b) || b.Sub(a) > 31*24*time.Hour || a.Nanosecond() != 0 || b.Nanosecond() != 0 {
		return a, b, errors.New("invalid_interval")
	}
	return a.UTC(), b.UTC(), nil
}
func Available(a, b time.Time) bool {
	loc, _ := time.LoadLocation(AgendaTimezone)
	x, y := a.In(loc), b.In(loc)
	return x.Weekday() != time.Saturday && x.Weekday() != time.Sunday && x.Format("2006-01-02") == y.Format("2006-01-02") && x.Hour() >= 9 && (y.Hour() < 18 || (y.Hour() == 18 && y.Minute() == 0 && y.Second() == 0)) && b.Sub(a) <= 8*time.Hour
}

// UTC is authoritative in storage; local presentation is computed by Go so
// the model need not infer an offset from the timezone label.
func (e *AgendaEvent) LocalTimes() {
	loc, _ := time.LoadLocation(AgendaTimezone)
	start, _ := time.Parse(time.RFC3339, e.Start)
	end, _ := time.Parse(time.RFC3339, e.End)
	e.StartLocal = start.In(loc).Format(time.RFC3339)
	e.EndLocal = end.In(loc).Format(time.RFC3339)
}
