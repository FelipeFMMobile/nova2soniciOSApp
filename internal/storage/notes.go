// Package storage owns atomic notes mutations and their durable retry ledger.
package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
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
	_, err = db.Exec(`PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY);
CREATE TABLE IF NOT EXISTS notes (id TEXT PRIMARY KEY, title TEXT NOT NULL, content TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS operations (key TEXT PRIMARY KEY, name TEXT NOT NULL, arguments TEXT NOT NULL, result TEXT NOT NULL, completed_at TEXT NOT NULL);
INSERT OR IGNORE INTO schema_migrations VALUES (1);
INSERT OR IGNORE INTO schema_migrations VALUES (2);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) List(ctx context.Context, query string) ([]Note, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,title,content,created_at FROM notes WHERE instr(lower(title || ' ' || content),lower(?))>0 ORDER BY created_at,id LIMIT 100`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []Note{}
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Title, &n.Content, &n.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// Mutate commits the effect and replay result together. A reused key with
// different arguments is an error, never a second mutation.
func (s *Store) Mutate(ctx context.Context, key, name string, args json.RawMessage) (json.RawMessage, error) {
	if key == "" || len(key) > 256 {
		return nil, errors.New("invalid idempotency key")
	}
	var input struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return nil, errors.New("invalid arguments")
	}
	var canonical any
	if err := json.Unmarshal(args, &canonical); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(canonical)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var oldName, oldArgs, result string
	err = tx.QueryRowContext(ctx, `SELECT name,arguments,result FROM operations WHERE key=?`, key).Scan(&oldName, &oldArgs, &result)
	if err == nil {
		if oldName != name || oldArgs != string(encoded) {
			return nil, errors.New("idempotency conflict")
		}
		return json.RawMessage(result), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var value any
	switch name {
	case "notes.create":
		if strings.TrimSpace(input.Title) == "" || len(input.Title) > 256 || strings.TrimSpace(input.Content) == "" || len(input.Content) > 8192 {
			return nil, errors.New("invalid note")
		}
		n := Note{ID: rand.Text(), Title: input.Title, Content: input.Content, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		_, err = tx.ExecContext(ctx, `INSERT INTO notes VALUES (?,?,?,?)`, n.ID, n.Title, n.Content, n.CreatedAt)
		value = map[string]any{"status": "created", "note": n}
	case "notes.delete":
		if input.ID == "" {
			return nil, errors.New("note id required")
		}
		var change sql.Result
		change, err = tx.ExecContext(ctx, `DELETE FROM notes WHERE id=?`, input.ID)
		if err == nil {
			count, _ := change.RowsAffected()
			if count != 1 {
				return nil, errors.New("note not found")
			}
		}
		value = map[string]any{"status": "deleted", "id": input.ID}
	default:
		return nil, errors.New("unknown mutation")
	}
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(value)
	_, err = tx.ExecContext(ctx, `INSERT INTO operations VALUES (?,?,?,?,?)`, key, name, string(encoded), string(data), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return data, nil
}
