// Package db abre e migra o índice SQLite (FTS5) do vbrain. Porta
// determinística de lib/vbrain/db.rb usando o driver puro-Go modernc.org/sqlite.
package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"

	"github.com/virtual360-io/vbrain/internal/paths"
	_ "modernc.org/sqlite"
)

// SchemaSQL é o schema idempotente do índice: raw_sources, pages, links,
// query_log, a tabela FTS5 externa e os triggers de sincronização.
const SchemaSQL = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS raw_sources (
  id                INTEGER PRIMARY KEY,
  path              TEXT NOT NULL UNIQUE,
  original_filename TEXT NOT NULL,
  source_type       TEXT NOT NULL CHECK(source_type IN ('text','url','tweet','oneshot')),
  sha256            TEXT NOT NULL UNIQUE,
  ingested_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS pages (
  id          INTEGER PRIMARY KEY,
  path        TEXT NOT NULL UNIQUE,
  title       TEXT NOT NULL,
  body        TEXT NOT NULL,
  kind        TEXT NOT NULL CHECK(kind IN ('concept','decision','gotcha','note','rule','realtime')),
  tags        TEXT NOT NULL DEFAULT '',
  sha256      TEXT NOT NULL,
  raw_id      INTEGER REFERENCES raw_sources(id) ON DELETE SET NULL,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS links (
  id           INTEGER PRIMARY KEY,
  from_page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
  target_slug  TEXT NOT NULL,
  target_title TEXT NOT NULL,
  to_page_id   INTEGER REFERENCES pages(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS links_from ON links(from_page_id);
CREATE INDEX IF NOT EXISTS links_to   ON links(to_page_id);

CREATE TABLE IF NOT EXISTS query_log (
  id            INTEGER PRIMARY KEY,
  query         TEXT NOT NULL,
  source_query  TEXT,
  normalized    TEXT NOT NULL DEFAULT '',
  results_count INTEGER NOT NULL DEFAULT 0,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS query_log_created ON query_log(created_at);

CREATE VIRTUAL TABLE IF NOT EXISTS pages_fts USING fts5(
  title, body, tags,
  content='pages', content_rowid='id',
  tokenize="unicode61 tokenchars '/_-'"
);

CREATE TRIGGER IF NOT EXISTS pages_ai AFTER INSERT ON pages BEGIN
  INSERT INTO pages_fts(rowid, title, body, tags) VALUES (new.id, new.title, new.body, new.tags);
END;
CREATE TRIGGER IF NOT EXISTS pages_ad AFTER DELETE ON pages BEGIN
  INSERT INTO pages_fts(pages_fts, rowid, title, body, tags) VALUES('delete', old.id, old.title, old.body, old.tags);
END;
CREATE TRIGGER IF NOT EXISTS pages_au AFTER UPDATE ON pages BEGIN
  INSERT INTO pages_fts(pages_fts, rowid, title, body, tags) VALUES('delete', old.id, old.title, old.body, old.tags);
  INSERT INTO pages_fts(rowid, title, body, tags) VALUES (new.id, new.title, new.body, new.tags);
END;
`

// Open abre o banco em path (ou Paths.DBPath() se vazio), garante o diretório,
// força foreign_keys e roda a migração. Usa uma única conexão: o índice é
// single-user e bancos :memory: exigem conexão única.
func Open(path string) (*sql.DB, error) {
	if path == "" {
		path = paths.DBPath()
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}

	dsn := path + "?_pragma=foreign_keys(1)"
	if path == ":memory:" {
		dsn = path
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)

	if err := Migrate(sqlDB); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return sqlDB, nil
}

// Migrate aplica o schema (idempotente) e reconstrói pages se o CHECK antigo
// (sem 'realtime') estiver presente.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(SchemaSQL); err != nil {
		return err
	}
	return rebuildPagesIfOldKindCheck(db)
}

// rebuildPagesIfOldKindCheck derruba pages/fts/triggers e recria o schema se a
// tabela pages ainda tiver o CHECK de kind sem 'realtime' (migração legada).
func rebuildPagesIfOldKindCheck(db *sql.DB) error {
	var ddl sql.NullString
	err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='pages'").Scan(&ddl)
	if err != nil || !ddl.Valid {
		return nil // sem tabela ainda ou erro benigno: nada a reconstruir
	}
	sqlText := ddl.String
	if strings.Contains(sqlText, "'realtime'") || !strings.Contains(sqlText, "CHECK(kind IN") {
		return nil
	}

	const drop = `
DROP TRIGGER IF EXISTS pages_ai;
DROP TRIGGER IF EXISTS pages_ad;
DROP TRIGGER IF EXISTS pages_au;
DROP TABLE IF EXISTS pages_fts;
DROP TABLE IF EXISTS pages;
`
	if _, err := db.Exec(drop); err != nil {
		return err
	}
	_, err = db.Exec(SchemaSQL)
	return err
}
