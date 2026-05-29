require "test_helper"
require "set"
require "vbrain/links"

class LinksTest < Minitest::Test
  def slugs(body)
    VBrain::Links.extract(body).map(&:slug)
  end

  def test_extracts_single_wikilink
    links = VBrain::Links.extract("texto com [[Foo Bar]] no meio")
    assert_equal 1, links.size
    assert_equal "foo-bar", links.first.slug
    assert_equal "Foo Bar", links.first.title
  end

  def test_extracts_multiple_in_order
    assert_equal %w[alpha beta gamma], slugs("[[Alpha]] e [[Beta]] e [[Gamma]].")
  end

  def test_wikilink_alias_keeps_target_for_slug_and_title
    links = VBrain::Links.extract("ver [[Postgres Logical|replicação]]")
    assert_equal "postgres-logical", links.first.slug
    assert_equal "Postgres Logical", links.first.title
  end

  def test_dedups_by_slug
    assert_equal ["x"], slugs("[[X]] e de novo [[X]] e [[X|outro]]")
  end

  def test_parses_markdown_links_to_md_files
    links = VBrain::Links.extract("veja [Família de Victor](familia-de-victor.md) aqui")
    assert_equal "familia-de-victor", links.first.slug
    assert_equal "Família de Victor", links.first.title
  end

  def test_both_forms_dedup_by_slug
    # mesma página em wikilink e em markdown link → uma aresta só
    body = "[[Família de Victor]] e depois [Família de Victor](familia-de-victor.md)"
    assert_equal ["familia-de-victor"], slugs(body)
  end

  def test_ignores_external_and_non_md_markdown_links
    body = "[Google](https://google.com) e [foto](pic.png)"
    assert_equal [], slugs(body)
  end

  def test_empty_and_nil
    assert_equal [], VBrain::Links.extract("sem link [x] (y)")
    assert_equal [], VBrain::Links.extract(nil)
    assert_equal [], VBrain::Links.extract("[[]] e [[   ]]")
  end

  # --- linkify ---

  def test_linkify_converts_resolvable_wikilink
    body = "liga pra [[Família de Victor]]."
    out = VBrain::Links.linkify(body, Set["familia-de-victor"])
    assert_equal "liga pra [Família de Victor](familia-de-victor.md).", out
  end

  def test_linkify_leaves_unresolvable_untouched
    body = "liga pra [[Página Inexistente]]."
    assert_equal body, VBrain::Links.linkify(body, Set["familia-de-victor"])
  end

  def test_linkify_alias_uses_display_text_and_target_slug
    body = "ver [[Postgres Logical|replicação lógica]]"
    out = VBrain::Links.linkify(body, Set["postgres-logical"])
    assert_equal "ver [replicação lógica](postgres-logical.md)", out
  end

  def test_linkify_is_idempotent
    body = "liga pra [[Família de Victor]]."
    once = VBrain::Links.linkify(body, Set["familia-de-victor"])
    twice = VBrain::Links.linkify(once, Set["familia-de-victor"])
    assert_equal once, twice
  end

  def test_linkify_accepts_array_of_slugs
    out = VBrain::Links.linkify("[[A]]", ["a"])
    assert_equal "[A](a.md)", out
  end

  # --- apply_resolution (camada LLM aplicada) ---

  def test_apply_resolution_maps_title_to_chosen_slug
    body = "trabalha na [[V360]]."
    out = VBrain::Links.apply_resolution(body, { "V360" => "carreira-de-victor" })
    assert_equal "trabalha na [V360](carreira-de-victor.md).", out
  end

  def test_apply_resolution_alias_uses_display
    body = "ver [[Empresa V360|a V360]]"
    out = VBrain::Links.apply_resolution(body, { "Empresa V360" => "v360" })
    assert_equal "ver [a V360](v360.md)", out
  end

  def test_apply_resolution_leaves_null_or_absent_untouched
    body = "[[UFRJ]] e [[Outra]]"
    out = VBrain::Links.apply_resolution(body, { "UFRJ" => nil })
    assert_equal body, out, "slug nil (LLM não achou) e título ausente do mapa ficam intactos"
  end

  def test_apply_resolution_empty_map_noop
    body = "[[X]]"
    assert_equal body, VBrain::Links.apply_resolution(body, {})
  end
end
