package boardcache

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

type SQLite struct{ db *sql.DB }

func Open(path string) (*SQLite, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建 SQLite 目录: %w", err)
	}
	db, err := sql.Open("sqlite", path+"?_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &SQLite{db: db}
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS board_cache_entries (kind TEXT NOT NULL, resource_key TEXT NOT NULL, data BLOB NOT NULL, loaded INTEGER NOT NULL, updated_at TEXT NOT NULL, generation INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(kind, resource_key, generation));
CREATE TABLE IF NOT EXISTS board_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT OR IGNORE INTO board_meta(key, value) VALUES ('active_generation', '0');
DELETE FROM board_cache_entries WHERE generation != 0 AND generation != CAST((SELECT value FROM board_meta WHERE key='active_generation') AS INTEGER);`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化 SQLite: %w", err)
	}
	return s, nil
}

func (s *SQLite) read(ctx context.Context, kind, key string, limit uint32) (any, bool, string, error) {
	var raw []byte
	var loaded int
	var updated string
	err := s.db.QueryRowContext(ctx, `SELECT data, loaded, updated_at FROM board_cache_entries WHERE kind=? AND resource_key=? AND generation IN (0, CAST((SELECT value FROM board_meta WHERE key='active_generation') AS INTEGER)) ORDER BY generation DESC LIMIT 1`, kind, key).Scan(&raw, &loaded, &updated)
	if err == sql.ErrNoRows {
		return nil, false, "", nil
	}
	if err != nil {
		return nil, false, "", err
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false, "", err
	}
	if limit > 0 {
		if list, ok := value.([]any); ok && uint32(len(list)) > limit {
			value = list[:limit]
		}
	}
	return value, loaded != 0, updated, nil
}
func (s *SQLite) Boards(ctx context.Context, limit uint32) (any, bool, string, error) {
	return s.read(ctx, "boards", "_", limit)
}
func (s *SQLite) Members(ctx context.Context, board string, limit uint32) (any, bool, string, error) {
	return s.read(ctx, "members", board, limit)
}

func (s *SQLite) replace(ctx context.Context, kind, key string, value any, generation int64) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO board_cache_entries(kind,resource_key,data,loaded,updated_at,generation) VALUES(?,?,?,1,?,?) ON CONFLICT(kind,resource_key,generation) DO UPDATE SET data=excluded.data,loaded=1,updated_at=excluded.updated_at`, kind, key, raw, time.Now().UTC().Format(time.RFC3339Nano), generation)
	return err
}
func (s *SQLite) ReplaceBoards(ctx context.Context, value any) error {
	return s.replace(ctx, "boards", "_", value, 0)
}
func (s *SQLite) ReplaceMembers(ctx context.Context, board string, value any) error {
	return s.replace(ctx, "members", board, value, 0)
}

func (s *SQLite) Publish(ctx context.Context, boards any, members map[string]any) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	gen := time.Now().UnixNano()
	raw, err := json.Marshal(boards)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, `DELETE FROM board_cache_entries WHERE generation != 0`); err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO board_cache_entries(kind,resource_key,data,loaded,updated_at,generation) VALUES('boards','_',?,1,?,?)`, raw, stamp, gen)
	}
	for key, value := range members {
		if err != nil {
			break
		}
		raw, err = json.Marshal(value)
		if err == nil {
			_, err = tx.ExecContext(ctx, `INSERT INTO board_cache_entries(kind,resource_key,data,loaded,updated_at,generation) VALUES('members',?,?,1,?,?)`, key, raw, stamp, gen)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO board_meta(key,value) VALUES('active_generation', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprint(gen))
	}
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
