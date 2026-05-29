require "test_helper"
require "vbrain"

class TagsCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  TAGS = File.join(PROJECT_ROOT, "scripts", "tags.rb")

  def setup
    VBrain::Paths.ensure_dirs!
    @paths = []
  end

  def teardown
    @paths.each { |p| File.delete(p) if File.exist?(p) }
    return if @paths.empty?

    VBrain::DB.open do |db|
      @paths.each { |abs| db.execute("DELETE FROM pages WHERE path = ?", [abs.sub(VBrain::Paths.wiki_dir + "/", "")]) }
    end
  end

  def page(slug, tags)
    abs = VBrain::Page.write(
      dir: VBrain::Paths.wiki_dir, slug: slug,
      frontmatter: { "title" => slug, "kind" => "note", "tags" => tags },
      body: "corpo\n"
    )
    @paths << abs
    abs
  end

  def run_tags(*args)
    out, _, st = Open3.capture3("bundle", "exec", "ruby", TAGS, *args, chdir: PROJECT_ROOT)
    [JSON.parse(out), st]
  end

  def test_aggregates_and_ranks_tags_by_count
    ts = Time.now.to_f.to_s.tr(".", "")
    page("tg-a-#{ts}", %W[carreira#{ts} bpm#{ts}])
    page("tg-b-#{ts}", %W[carreira#{ts}])
    Open3.capture3("bundle", "exec", "ruby", File.join(PROJECT_ROOT, "scripts", "reindex.rb"), chdir: PROJECT_ROOT)

    data, st = run_tags
    assert st.success?
    by_tag = data["tags"].to_h { |h| [h["tag"], h["count"]] }
    assert_equal 2, by_tag["carreira#{ts}"], "tag em duas páginas conta 2"
    assert_equal 1, by_tag["bpm#{ts}"]
    # carreira (2) vem antes de bpm (1) — ordenado por frequência desc.
    idx = data["tags"].map { |h| h["tag"] }
    assert idx.index("carreira#{ts}") < idx.index("bpm#{ts}")
  end
end
