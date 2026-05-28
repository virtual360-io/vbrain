require "test_helper"
require "vbrain/fts_query"

class FtsQueryTest < Minitest::Test
  def test_normalizes_simple_query
    assert_equal %("foo" OR "bar"), VBrain::FtsQuery.normalize("foo bar")
  end

  def test_neutralizes_colon
    assert_equal %("postgres" OR "logical"), VBrain::FtsQuery.normalize("postgres:logical")
  end

  def test_neutralizes_quotes_and_parens
    assert_equal %("foo" OR "bar" OR "baz"), VBrain::FtsQuery.normalize('"foo" (bar) baz')
  end

  def test_empty_input_returns_empty
    assert_equal "", VBrain::FtsQuery.normalize("")
    assert_equal "", VBrain::FtsQuery.normalize(nil)
    assert_equal "", VBrain::FtsQuery.normalize("   ")
    assert_equal "", VBrain::FtsQuery.normalize(":::")
  end

  def test_prefix_mode_appends_star
    assert_equal %("foo"* OR "bar"*), VBrain::FtsQuery.normalize("foo bar", prefix: true)
  end

  def test_single_token_no_or
    assert_equal %("foo"), VBrain::FtsQuery.normalize("foo")
  end

  def test_handles_trailing_dash_without_fts_error
    require "sqlite3"
    require "vbrain/db"

    db = SQLite3::Database.new(":memory:")
    VBrain::DB.migrate!(db)
    db.execute("INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
               ["a.md", "T", "body has marker-12345 inside", "concept", "x"])
    normalized = VBrain::FtsQuery.normalize("marker-")
    refute_empty normalized
    db.execute("SELECT title FROM pages_fts WHERE pages_fts MATCH ?", [normalized])
  end

  def test_actually_queries_fts_without_error
    require "sqlite3"
    require "vbrain/db"

    db = SQLite3::Database.new(":memory:")
    VBrain::DB.migrate!(db)
    db.execute("INSERT INTO pages (path, title, body, kind, sha256) VALUES (?, ?, ?, ?, ?)",
               ["a.md", "Postgres Logical", "replica identity full", "concept", "x"])

    normalized = VBrain::FtsQuery.normalize("postgres:logical")
    refute_empty normalized
    rows = db.execute("SELECT title FROM pages_fts WHERE pages_fts MATCH ?", [normalized])
    assert_equal 1, rows.size
  end
end
