require "test_helper"
require "vbrain"

class ResolveLinksCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  RESOLVE = File.join(PROJECT_ROOT, "scripts", "resolve_links.rb")

  def setup
    VBrain::Paths.ensure_dirs!
    @created = []
  end

  def teardown
    @created.each { |p| File.delete(p) if File.exist?(p) }
  end

  def test_applies_llm_map_only_for_existing_slugs
    ts = Time.now.to_f.to_s.tr(".", "")
    real = "rl-empresa-#{ts}"
    @created << VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: real,
      frontmatter: { "title" => "Empresa #{ts}", "kind" => "note", "tags" => [] },
      body: "alvo real\n"
    )
    src = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: "rl-origem-#{ts}",
      frontmatter: { "title" => "Origem", "kind" => "note", "tags" => ["k"] },
      body: "cita [[Acme #{ts}]] e [[Fantasma #{ts}]].\n"
    )
    @created << src

    # LLM diz: "Acme" → página real; "Fantasma" → slug inexistente (deve ser descartado)
    map = { "Acme #{ts}" => real, "Fantasma #{ts}" => "slug-que-nao-existe-#{ts}" }
    map_path = File.join(Dir.tmpdir, "rlmap-#{ts}.json")
    File.write(map_path, JSON.generate(map))
    @created << map_path

    out, err, st = Open3.capture3("bundle", "exec", "ruby", RESOLVE, "--map", map_path, chdir: PROJECT_ROOT)
    assert st.success?, "resolve_links falhou: #{err}"
    data = JSON.parse(out)
    assert_equal 1, data["applied"], "só o slug existente é aplicado"
    assert_equal 1, data["dropped_unknown_slug"], "slug inexistente é descartado (defesa anti-alucinação)"

    parsed = VBrain::Page.parse(src)
    assert_includes parsed.body, "[Acme #{ts}](#{real}.md)", "resolução válida vira link markdown"
    assert_includes parsed.body, "[[Fantasma #{ts}]]", "slug inventado fica como wikilink intacto"
    assert_equal "Origem", parsed.frontmatter["title"], "frontmatter preservado"
  end
end
