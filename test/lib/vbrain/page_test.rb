require "test_helper"
require "vbrain/page"

class PageTest < Minitest::Test
  def test_parse_string_with_frontmatter
    content = <<~MD
      ---
      title: Foo
      tags:
        - a
        - b
      ---
      # Foo

      Body here.
    MD

    parsed = VBrain::Page.parse_string(content)
    assert_equal "Foo", parsed.frontmatter["title"]
    assert_equal %w[a b], parsed.frontmatter["tags"]
    assert_includes parsed.body, "Body here."
    refute_empty parsed.sha256
  end

  def test_parse_string_without_frontmatter
    content = "# Plain markdown\n\nNo frontmatter."
    parsed = VBrain::Page.parse_string(content)
    assert_empty parsed.frontmatter
    assert_equal content, parsed.body
  end

  def test_write_creates_file_atomically
    with_tmpdir do |dir|
      path = VBrain::Page.write(
        dir: dir,
        slug: "foo-page",
        frontmatter: { "title" => "Foo Page", "kind" => "note", "tags" => %w[a b] },
        body: "# Foo Page\n\nHello.\n"
      )
      assert File.exist?(path)
      assert_equal File.join(dir, "foo-page.md"), path
      assert_empty Dir.glob(File.join(dir, "*.tmp.*"))
    end
  end

  def test_write_then_parse_roundtrip_keeps_sha256_stable
    with_tmpdir do |dir|
      body = "## Conteúdo\n\nLinha 1\nLinha 2\n"
      fm = { "title" => "Round Trip", "kind" => "concept", "tags" => ["x"] }
      path = VBrain::Page.write(dir: dir, slug: "rt", frontmatter: fm, body: body)
      parsed = VBrain::Page.parse(path)
      assert_equal "Round Trip", parsed.frontmatter["title"]
      assert_equal body, parsed.body
      assert_equal Digest::SHA256.hexdigest(body), parsed.sha256
    end
  end

  def test_write_raises_on_empty_slug
    with_tmpdir do |dir|
      assert_raises(VBrain::Page::Error) do
        VBrain::Page.write(dir: dir, slug: "", frontmatter: {}, body: "x")
      end
    end
  end

  def test_render_uses_string_keys
    rendered = VBrain::Page.render({ title: "Sym", tags: [:a] }, "body")
    assert_includes rendered, "title:"
    assert_includes rendered, "Sym"
  end
end
