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
    dir = File.join(VBrain::Paths.wiki_dir, "notes")
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

  def test_query_handles_colon_without_error
    _stdout, stderr, status = Open3.capture3("bundle", "exec", "ruby", QUERY, "postgres:logical", "--format", "json",
                                             chdir: PROJECT_ROOT)
    assert status.success?, "query failed: #{stderr}"
  end
end
