package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

func OpenStore(path string) (*Store, error) {
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
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL;
 CREATE TABLE IF NOT EXISTS captures(id TEXT PRIMARY KEY, source TEXT NOT NULL, format TEXT NOT NULL, created TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS entries(id TEXT PRIMARY KEY, capture_id TEXT NOT NULL REFERENCES captures(id) ON DELETE CASCADE, sequence INTEGER NOT NULL, payload BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS entries_capture_sequence ON entries(capture_id, sequence);
 CREATE TABLE IF NOT EXISTS artifacts(id TEXT PRIMARY KEY, kind TEXT NOT NULL, subject_id TEXT NOT NULL, created TEXT NOT NULL, payload BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS artifacts_subject ON artifacts(kind,subject_id,created);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) SaveCapture(ctx context.Context, c Capture, entries []Entry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if c.Created.IsZero() {
		c.Created = time.Now().UTC()
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO captures VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING`, c.ID, c.Source, c.Format, c.Created.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO entries VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err = stmt.ExecContext(ctx, e.ID, c.ID, e.Sequence, b); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) ListCaptures(ctx context.Context, limit, offset int) ([]Capture, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,c.source,c.format,c.created,COUNT(e.id) FROM captures c LEFT JOIN entries e ON c.id=e.capture_id GROUP BY c.id ORDER BY c.created DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Capture{}
	for rows.Next() {
		var c Capture
		var t string
		if err = rows.Scan(&c.ID, &c.Source, &c.Format, &t, &c.Count); err != nil {
			return nil, err
		}
		c.Created, _ = time.Parse(time.RFC3339Nano, t)
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) Entries(ctx context.Context, id string) ([]Entry, error) {
	var found int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM captures WHERE id=?`, id).Scan(&found); err != nil {
		return nil, fmt.Errorf("capture %q: %w", id, err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM entries WHERE capture_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var b []byte
		var e Entry
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) Entry(ctx context.Context, id string) (Entry, error) {
	var b []byte
	var e Entry
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM entries WHERE id=?`, id).Scan(&b)
	if err != nil {
		return e, fmt.Errorf("entry %q: %w", id, err)
	}
	err = json.Unmarshal(b, &e)
	return e, err
}
func (s *Store) DeleteCapture(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM artifacts WHERE subject_id=? OR subject_id IN (SELECT id FROM entries WHERE capture_id=?)`, id, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM captures WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("capture %q not found", id)
	}
	return tx.Commit()
}
func (s *Store) SaveArtifact(ctx context.Context, kind, subject string, value any) (string, error) {
	id := NewID(kind + "_")
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO artifacts VALUES(?,?,?,?,?)`, id, kind, subject, time.Now().UTC().Format(time.RFC3339Nano), b)
	return id, err
}
func (s *Store) Artifacts(ctx context.Context, kind, subject string, limit, offset int) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,subject_id,created,payload FROM artifacts WHERE (?='' OR kind=?) AND (?='' OR subject_id=?) ORDER BY created DESC LIMIT ? OFFSET ?`, kind, kind, subject, subject, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, k, sub, t string
		var b []byte
		if err = rows.Scan(&id, &k, &sub, &t, &b); err != nil {
			return nil, err
		}
		var v any
		if err = json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "kind": k, "subject_id": sub, "created_at": t, "result": v})
	}
	return out, rows.Err()
}
