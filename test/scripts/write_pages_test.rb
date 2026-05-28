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
          { "category" => "notes", "title" => "WP Test", "tags" => [tag],
            "body_markdown" => "## A\n\nFirst.\n", "slug_hint" => tag },
          { "category" => "notes", "title" => "WP Test 2", "tags" => [tag],
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
        abs = File.join(VBrain::Paths::WIKI_DIR, rel)
        @paths_to_cleanup << abs
        assert File.exist?(abs)
        parsed = VBrain::Page.parse(abs)
        assert_equal "note", parsed.frontmatter["kind"]
        assert_includes parsed.frontmatter["tags"], tag
      end
    end
  end
end
