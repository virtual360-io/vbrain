require "test_helper"
require "vbrain/db"

class DBTest < Minitest::Test
  def test_migrate_creates_tables_and_triggers
    db = SQLite3::Database.new(":memory:")
    db.results_as_hash = true
    VBrain::DB.migrate!(db)

    tables = db.execute("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name").flat_map(&:values).uniq
    assert_includes tables, "pages"
    assert_includes tables, "raw_sources"

    fts = db.execute("SELECT name FROM sqlite_master WHERE name='pages_fts'").first
    assert fts, "pages_fts virtual table should exist"

    triggers = db.execute("SELECT name FROM sqlite_master WHERE type='trigger' ORDER BY name").flat_map(&:values)
    %w[pages_ai pages_ad pages_au].each do |t|
      assert_includes triggers, t
    end
  end

  def test_migrate_is_idempotent
    db = SQLite3::Database.new(":memory:")
    VBrain::DB.migrate!(db)
    VBrain::DB.migrate!(db)
    VBrain::DB.migrate!(db)
    count = db.execute("SELECT COUNT(*) FROM sqlite_master WHERE type='table'").first.first
    assert count >= 2
  end

  def test_fts_trigger_syncs_on_insert_update_delete
    db = SQLite3::Database.new(":memory:")
    db.results_as_hash = true
    VBrain::DB.migrate!(db)

    db.execute(
      "INSERT INTO pages (path, title, body, kind, tags, sha256) VALUES (?, ?, ?, ?, ?, ?)",
      ["concepts/foo.md", "Foo Concept", "Discussion of foobar and baz", "concept", "foo,bar", "abc"]
    )
    page_id = db.last_insert_row_id

    hits = db.execute(
      "SELECT p.title FROM pages_fts JOIN pages p ON p.id = pages_fts.rowid WHERE pages_fts MATCH ?",
      ["foobar"]
    )
    assert_equal 1, hits.size

    db.execute("UPDATE pages SET body = ? WHERE id = ?", ["renamed body about widgets", page_id])
    hits2 = db.execute("SELECT p.title FROM pages_fts JOIN pages p ON p.id = pages_fts.rowid WHERE pages_fts MATCH ?", ["widgets"])
    assert_equal 1, hits2.size
    hits3 = db.execute("SELECT p.title FROM pages_fts JOIN pages p ON p.id = pages_fts.rowid WHERE pages_fts MATCH ?", ["foobar"])
    assert_equal 0, hits3.size

    db.execute("DELETE FROM pages WHERE id = ?", [page_id])
    hits4 = db.execute("SELECT p.title FROM pages_fts JOIN pages p ON p.id = pages_fts.rowid WHERE pages_fts MATCH ?", ["widgets"])
    assert_equal 0, hits4.size
  end

  def test_check_constraint_rejects_unknown_kind
    db = SQLite3::Database.new(":memory:")
    VBrain::DB.migrate!(db)
    assert_raises(SQLite3::ConstraintException) do
      db.execute(
        "INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
        ["x/y.md", "T", "B", "garbage", "sha"]
      )
    end
  end
end
