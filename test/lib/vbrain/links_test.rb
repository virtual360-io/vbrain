require "test_helper"
require "vbrain/links"

class LinksTest < Minitest::Test
  def test_extracts_single_link
    assert_equal ["Foo Bar"], VBrain::Links.extract("texto com [[Foo Bar]] no meio")
  end

  def test_extracts_multiple_in_order
    body = "Veja [[Alpha]] e depois [[Beta]] e [[Gamma]]."
    assert_equal %w[Alpha Beta Gamma], VBrain::Links.extract(body)
  end

  def test_supports_alias_keeps_target_only
    assert_equal ["Postgres Logical"], VBrain::Links.extract("ver [[Postgres Logical|replicação]]")
  end

  def test_dedups_repeated_targets
    assert_equal ["X"], VBrain::Links.extract("[[X]] e de novo [[X]] e [[X|outro texto]]")
  end

  def test_strips_whitespace
    assert_equal ["Espaçado"], VBrain::Links.extract("[[  Espaçado  ]]")
  end

  def test_returns_empty_for_body_without_links
    assert_equal [], VBrain::Links.extract("sem nenhum link aqui [x] (y) {z}")
  end

  def test_ignores_empty_or_whitespace_only_links
    assert_equal [], VBrain::Links.extract("[[]] e [[   ]] e [[|só alias]]")
  end

  def test_returns_empty_for_nil
    assert_equal [], VBrain::Links.extract(nil)
  end

  def test_target_slug_matches_write_pages_slug
    # Mesma normalização que VBrain::Slug.from, pra resolução casar.
    assert_equal "foo-bar", VBrain::Links.target_slug("Foo Bar")
    assert_equal "replicacao-logica", VBrain::Links.target_slug("Replicação Lógica")
  end

  def test_target_slug_returns_nil_when_no_valid_slug
    assert_nil VBrain::Links.target_slug("!!!")
    assert_nil VBrain::Links.target_slug("   ")
  end
end
