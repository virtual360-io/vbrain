require "test_helper"
require "vbrain"

class ReindexAndQueryCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  REINDEX = File.join(PROJECT_ROOT, "scripts", "reindex.rb")
  QUERY   = File.join(PROJECT_ROOT, "scripts", "query.rb")

  def setup
    VBrain::Paths.ensure_dirs!
    @created_paths = []
  end

  def teardown
    @created_paths.each { |p| File.delete(p) if File.exist?(p) }
    return if @created_paths.empty?

    VBrain::DB.open do |db|
      @created_paths.each do |abs|
        rel = abs.sub(VBrain::Paths.wiki_dir + "/", "")
        db.execute("DELETE FROM pages WHERE path = ?", [rel])
      end
    end
  end

  def test_reindex_inserts_then_updates_then_deletes
    marker = "marker-#{Time.now.to_f.to_s.tr('.', '')}"
    dir = VBrain::Paths.wiki_dir
    slug = "reindex-test-#{Time.now.to_f.to_s.tr('.', '')}"
    abs = VBrain::Page.write(
      dir: dir,
      slug: slug,
      frontmatter: { "title" => "Reindex Test", "kind" => "note", "tags" => ["test"] },
      body: "## Marker\n\nLine with #{marker}.\n"
    )
    @created_paths << abs

    _, _, st = Open3.capture3("bundle", "exec", "ruby", REINDEX, chdir: PROJECT_ROOT)
    assert st.success?

    stdout, _, _ = Open3.capture3("bundle", "exec", "ruby", QUERY, marker, "--format", "json",
                                  chdir: PROJECT_ROOT)
    data = JSON.parse(stdout)
    assert data["results"].any? { |r| r["path"].include?(slug) }, "expected #{slug} in results, got #{data['results']}"

    File.delete(abs)
    @created_paths.delete(abs)
    _, _, st2 = Open3.capture3("bundle", "exec", "ruby", REINDEX, chdir: PROJECT_ROOT)
    assert st2.success?

    stdout2, _, _ = Open3.capture3("bundle", "exec", "ruby", QUERY, marker, "--format", "json",
                                   chdir: PROJECT_ROOT)
    data2 = JSON.parse(stdout2)
    assert data2["results"].none? { |r| r["path"].include?(slug) }, "expected #{slug} gone after delete"
  end

  def test_reindex_builds_link_graph_and_resolves_forward_links
    ts = Time.now.to_f.to_s.tr(".", "")
    btarget = "btarget-#{ts}"
    ghost   = "ghost-#{ts}"

    # Página A linka B (ainda não existe) e um alvo fantasma.
    a = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: "atarget-#{ts}",
      frontmatter: { "title" => "A", "kind" => "note", "tags" => [] },
      body: "Liga pra [[#{btarget}]] e pra [[#{ghost}]].\n"
    )
    @created_paths << a

    Open3.capture3("bundle", "exec", "ruby", REINDEX, chdir: PROJECT_ROOT)

    a_rel = a.sub(VBrain::Paths.wiki_dir + "/", "")
    edges = VBrain::DB.open do |db|
      a_id = db.execute("SELECT id FROM pages WHERE path = ?", [a_rel]).first["id"]
      db.execute("SELECT target_slug, to_page_id FROM links WHERE from_page_id = ? ORDER BY target_slug", [a_id])
    end
    assert_equal 2, edges.size, "duas arestas a partir de A"
    assert edges.all? { |e| e["to_page_id"].nil? }, "ambos forward links começam não-resolvidos (NULL)"

    # Agora B aparece — reindex deve resolver a aresta A->B, mas não a fantasma.
    b = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: btarget,
      frontmatter: { "title" => "B", "kind" => "note", "tags" => [] },
      body: "Sou o alvo.\n"
    )
    @created_paths << b
    Open3.capture3("bundle", "exec", "ruby", REINDEX, chdir: PROJECT_ROOT)

    resolved = VBrain::DB.open do |db|
      a_id = db.execute("SELECT id FROM pages WHERE path = ?", [a_rel]).first["id"]
      b_id = db.execute("SELECT id FROM pages WHERE path = ?", [b.sub(VBrain::Paths.wiki_dir + "/", "")]).first["id"]
      rows = db.execute("SELECT target_slug, to_page_id FROM links WHERE from_page_id = ?", [a_id])
      { rows: rows, b_id: b_id }
    end
    b_edge = resolved[:rows].find { |e| e["target_slug"] == btarget }
    ghost_edge = resolved[:rows].find { |e| e["target_slug"] == ghost }
    assert_equal resolved[:b_id], b_edge["to_page_id"], "A->B resolve quando B passa a existir"
    assert_nil ghost_edge["to_page_id"], "aresta fantasma continua não-resolvida"
  end

  def test_query_expands_to_linked_neighbors
    ts = Time.now.to_f.to_s.tr(".", "")
    marker = "neighmarker#{ts}"
    bslug = "neighbor-b-#{ts}"

    # A casa o marker e linka B; B não casa o marker mas deve vir em related.
    a = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: "neighbor-a-#{ts}",
      frontmatter: { "title" => "Neighbor A", "kind" => "note", "tags" => [] },
      body: "Conteúdo com #{marker} e link pra [[#{bslug}]].\n"
    )
    b = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: bslug,
      frontmatter: { "title" => "Neighbor B", "kind" => "note", "tags" => [] },
      body: "Página alvo sem o termo buscado.\n"
    )
    @created_paths << a
    @created_paths << b

    Open3.capture3("bundle", "exec", "ruby", REINDEX, chdir: PROJECT_ROOT)
    stdout, _, st = Open3.capture3("bundle", "exec", "ruby", QUERY, marker, "--format", "json",
                                   chdir: PROJECT_ROOT)
    assert st.success?
    data = JSON.parse(stdout)
    assert data["results"].any? { |r| r["path"].include?("neighbor-a-#{ts}") }, "A casa o FTS"
    assert data["related"].any? { |r| r["path"].include?(bslug) },
      "B (linkado por A) deve aparecer em related, mesmo sem casar o termo: #{data['related']}"
  end

  def test_query_handles_colon_without_error
    _stdout, stderr, status = Open3.capture3("bundle", "exec", "ruby", QUERY, "postgres:logical", "--format", "json",
                                             chdir: PROJECT_ROOT)
    assert status.success?, "query failed: #{stderr}"
  end
end
