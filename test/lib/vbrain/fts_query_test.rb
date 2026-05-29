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

  def test_drops_stopwords_keeping_content_terms
    # "quais empregos eu já tive" deve virar só os termos de conteúdo — as
    # stopwords (quais/eu/já/tive) inflavam o OR e afogavam o sinal no BM25,
    # que foi a causa raiz do bug de "não acha empregos".
    assert_equal %("empregos"), VBrain::FtsQuery.normalize("quais empregos eu já tive")
  end

  def test_drops_stopwords_case_insensitive_and_unaccented
    assert_equal %("carreira"), VBrain::FtsQuery.normalize("Quais foram Minhas carreira")
    assert_equal %("carreira"), VBrain::FtsQuery.normalize("o que eu ja tive de carreira")
  end

  def test_keeps_multiple_content_terms
    assert_equal %("cargo" OR "visagio" OR "consultor"),
                 VBrain::FtsQuery.normalize("qual foi meu cargo na visagio como consultor")
  end

  def test_falls_back_to_original_when_all_stopwords
    # Só stopwords: melhor buscar com elas do que devolver vazio (zero hits).
    refute_empty VBrain::FtsQuery.normalize("quem é você")
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
