require "test_helper"
require "vbrain"

class WritePagesCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  WRITE  = File.join(PROJECT_ROOT, "scripts", "write_pages.rb")
  INGEST = File.join(PROJECT_ROOT, "scripts", "ingest_raw.rb")

  def setup
    VBrain::Paths.ensure_dirs!
    @paths_to_cleanup = []
    @sha_to_cleanup = []
  end

  def teardown
    @paths_to_cleanup.each { |p| File.delete(p) if File.exist?(p) }
    @sha_to_cleanup.each do |sha|
      VBrain::DB.open do |db|
        rows = db.execute("SELECT path FROM raw_sources WHERE sha256 = ?", [sha])
        rows.each { |r| File.delete(r["path"]) if File.exist?(r["path"]) }
        db.execute("DELETE FROM raw_sources WHERE sha256 = ?", [sha])
      end
    end
  end

  def test_writes_pages_with_resolved_slug_collision
    Dir.mktmpdir do |dir|
      src = File.join(dir, "wp_#{Time.now.to_f}.md")
      content = "marker #{Time.now.to_f}"
      File.write(src, content)
      @sha_to_cleanup << Digest::SHA256.hexdigest(File.read(src))

      stdout, _, _ = Open3.capture3("bundle", "exec", "ruby", INGEST, src, chdir: PROJECT_ROOT)
      ingest = JSON.parse(stdout)
      raw_id = ingest["raw_id"]

      tag = "writepages-test-#{Time.now.to_f.to_s.tr('.', '')}"
      pages = {
        "pages" => [
          { "kind" => "note", "title" => "WP Test", "tags" => [tag],
            "body_markdown" => "## A\n\nFirst.\n", "slug_hint" => tag },
          { "kind" => "note", "title" => "WP Test 2", "tags" => [tag],
            "body_markdown" => "## B\n\nSecond.\n", "slug_hint" => tag }
        ]
      }
      json_path = File.join(dir, "pages.json")
      File.write(json_path, JSON.generate(pages))

      stdout2, stderr2, status = Open3.capture3(
        "bundle", "exec", "ruby", WRITE,
        "--raw-id", raw_id.to_s, "--pages-json", json_path,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "write_pages failed: #{stderr2}"
      result = JSON.parse(stdout2)
      assert_equal 2, result["count"]
      assert_equal 2, result["written"].uniq.size, "slugs must be unique: #{result['written']}"
      result["written"].each do |rel|
        refute_includes rel, "/", "página de conhecimento mora na raiz de wiki/ (layout plano)"
        abs = File.join(VBrain::Paths.wiki_dir, rel)
        @paths_to_cleanup << abs
        assert File.exist?(abs)
        parsed = VBrain::Page.parse(abs)
        assert_equal "note", parsed.frontmatter["kind"]
        assert_includes parsed.frontmatter["tags"], tag
      end
    end
  end

  def test_kind_defaults_to_note_when_missing_or_invalid
    Dir.mktmpdir do |dir|
      src = File.join(dir, "wp_kind_#{Time.now.to_f}.md")
      File.write(src, "marker #{Time.now.to_f}")
      @sha_to_cleanup << Digest::SHA256.hexdigest(File.read(src))

      stdout, _, _ = Open3.capture3("bundle", "exec", "ruby", INGEST, src, chdir: PROJECT_ROOT)
      raw_id = JSON.parse(stdout)["raw_id"]

      tag = "wpkind-#{Time.now.to_f.to_s.tr('.', '')}"
      pages = { "pages" => [
        { "title" => "No Kind", "tags" => [tag], "body_markdown" => "x\n", "slug_hint" => "#{tag}-a" },
        { "kind" => "garbage", "title" => "Bad Kind", "tags" => [tag], "body_markdown" => "y\n", "slug_hint" => "#{tag}-b" }
      ] }
      json_path = File.join(dir, "pages.json")
      File.write(json_path, JSON.generate(pages))

      stdout2, stderr2, status = Open3.capture3(
        "bundle", "exec", "ruby", WRITE, "--raw-id", raw_id.to_s, "--pages-json", json_path,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "write_pages failed: #{stderr2}"
      JSON.parse(stdout2)["written"].each do |rel|
        abs = File.join(VBrain::Paths.wiki_dir, rel)
        @paths_to_cleanup << abs
        assert_equal "note", VBrain::Page.parse(abs).frontmatter["kind"]
      end
    end
  end
end
