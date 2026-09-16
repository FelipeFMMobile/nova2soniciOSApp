package storage

import (
	"context"
	"time"
)

// Audit stores lifecycle metadata only: no tokens, audio or transcripts.
func (s *Store) StartSession(ctx context.Context, id, requestID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions(id,request_id,started_at) VALUES (?,?,?)`, id, requestID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) EndSession(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET ended_at=? WHERE id=?`, now, id); err != nil {
		return err
	}
	// A disconnected host cannot infer whether an in-flight remote call took
	// effect. The atomic notes ledger is authoritative on retry.
	if _, err = tx.ExecContext(ctx, `UPDATE tool_operations SET state=CASE WHEN state='running' THEN 'outcome_unknown' ELSE 'cancelled' END,updated_at=? WHERE session_id=? AND state IN ('running','confirmation_required')`, now, id); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) RecordTurn(ctx context.Context, sessionID, id, state string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO turns VALUES (?,?,?,?) ON CONFLICT(session_id,id) DO UPDATE SET state=excluded.state,updated_at=excluded.updated_at`, sessionID, id, state, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) RecordTool(ctx context.Context, sessionID, id, turn, name, key, state string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tool_operations VALUES (?,?,?,?,?,?,?) ON CONFLICT(session_id,id) DO UPDATE SET state=excluded.state,updated_at=excluded.updated_at`, sessionID, id, turn, name, key, state, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
