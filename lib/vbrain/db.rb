require "sqlite3"
require "fileutils"
require_relative "paths"

module VBrain
  module DB
    SCHEMA_SQL = <<~SQL.freeze
      PRAGMA journal_mode=WAL;
      PRAGMA foreign_keys=ON;

      CREATE TABLE IF NOT EXISTS raw_sources (
        id                INTEGER PRIMARY KEY,
        path              TEXT NOT NULL UNIQUE,
        original_filename TEXT NOT NULL,
        source_type       TEXT NOT NULL CHECK(source_type IN ('text','transcript','epub','repo','spreadsheet','oneshot')),
        sha256            TEXT NOT NULL UNIQUE,
        ingested_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
      );

      CREATE TABLE IF NOT EXISTS pages (
        id          INTEGER PRIMARY KEY,
        path        TEXT NOT NULL UNIQUE,
        title       TEXT NOT NULL,
        body        TEXT NOT NULL,
        kind        TEXT NOT NULL CHECK(kind IN ('concept','decision','gotcha','note','rule')),
        tags        TEXT NOT NULL DEFAULT '',
        sha256      TEXT NOT NULL,
        raw_id      INTEGER REFERENCES raw_sources(id) ON DELETE SET NULL,
        created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
        updated_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
      );

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
    SQL

    def self.open(path = Paths::DB_PATH)
      FileUtils.mkdir_p(File.dirname(path)) unless path.to_s == ":memory:"
      db = SQLite3::Database.new(path.to_s)
      db.results_as_hash = true
      migrate!(db)
      if block_given?
        begin
          yield db
        ensure
          db.close
        end
      else
        db
      end
    end

    def self.migrate!(db)
      db.execute_batch(SCHEMA_SQL)
    end
  end
end
