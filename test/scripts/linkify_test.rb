require "test_helper"
require "vbrain"

class LinkifyCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  LINKIFY = File.join(PROJECT_ROOT, "scripts", "linkify.rb")

  def setup
    VBrain::Paths.ensure_dirs!
    @created = []
  end

  def teardown
    @created.each { |p| File.delete(p) if File.exist?(p) }
  end

  def test_converts_resolvable_wikilinks_and_preserves_frontmatter
    ts = Time.now.to_f.to_s.tr(".", "")
    target = "linkify-alvo-#{ts}"
    # alvo precisa existir como arquivo pro slug resolver
    tgt = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: target,
      frontmatter: { "title" => "Linkify Alvo #{ts}", "kind" => "note", "tags" => [] },
      body: "sou o alvo\n"
    )
    @created << tgt

    src = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: "linkify-origem-#{ts}",
      frontmatter: { "title" => "Origem", "kind" => "note", "tags" => ["t"] },
      body: "liga pra [[Linkify Alvo #{ts}]] e pra [[Nao Existe #{ts}]].\n"
    )
    @created << src

    _, err, st = Open3.capture3("bundle", "exec", "ruby", LINKIFY, chdir: PROJECT_ROOT)
    assert st.success?, "linkify falhou: #{err}"

    parsed = VBrain::Page.parse(src)
    assert_includes parsed.body, "[Linkify Alvo #{ts}](#{target}.md)",
      "wikilink resolvível deve virar link markdown navegável"
    assert_includes parsed.body, "[[Nao Existe #{ts}]]",
      "wikilink não-resolvível deve ficar intacto"
    # frontmatter preservado
    assert_equal "Origem", parsed.frontmatter["title"]
    assert_equal %w[t], parsed.frontmatter["tags"]
  end

  def test_idempotent_no_change_on_second_run
    ts = Time.now.to_f.to_s.tr(".", "")
    target = "lk-idem-alvo-#{ts}"
    @created << VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: target,
      frontmatter: { "title" => "LK Idem Alvo #{ts}", "kind" => "note", "tags" => [] },
      body: "alvo\n"
    )
    @created << VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: "lk-idem-origem-#{ts}",
      frontmatter: { "title" => "O", "kind" => "note", "tags" => [] },
      body: "ver [[LK Idem Alvo #{ts}]]\n"
    )

    out1, _, _ = Open3.capture3("bundle", "exec", "ruby", LINKIFY, chdir: PROJECT_ROOT)
    out2, _, _ = Open3.capture3("bundle", "exec", "ruby", LINKIFY, chdir: PROJECT_ROOT)
    assert JSON.parse(out1)["changed"] >= 1, "primeira passada converte ao menos 1"
    # segunda passada não deve reconverter o que já é markdown link
    assert_equal 0, JSON.parse(out2)["changed"], "segunda passada é no-op (idempotente)"
  end
end
