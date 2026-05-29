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
    SQL

    def self.open(path = nil)
      path ||= Paths.db_path
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
      rebuild_pages_if_old_kind_check!(db)
    end

    def self.rebuild_pages_if_old_kind_check!(db)
      row = db.execute("SELECT sql FROM sqlite_master WHERE type='table' AND name='pages'").first
      return unless row

      sql = row.is_a?(Hash) ? row["sql"] : row[0]
      return unless sql
      return if sql.include?("'realtime'")
      return unless sql.include?("CHECK(kind IN")

      db.execute_batch(<<~SQL)
        DROP TRIGGER IF EXISTS pages_ai;
        DROP TRIGGER IF EXISTS pages_ad;
        DROP TRIGGER IF EXISTS pages_au;
        DROP TABLE IF EXISTS pages_fts;
        DROP TABLE IF EXISTS pages;
      SQL
      db.execute_batch(SCHEMA_SQL)
    end
  end
end
