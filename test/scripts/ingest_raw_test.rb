require "test_helper"

# Scripts read from /Users/victorcampos/Workspace/vbrain via Paths::ROOT. For
# tests we run them in a tmpdir with VBRAIN_ROOT override is not implemented;
# instead, tests use an isolated wiki/db inside the project for actual CLI
# execution. To keep tests hermetic, we shell out with bundle exec in the
# project root, and clean the rows we created.

class IngestRawCLITest < Minitest::Test
  SCRIPT = File.join(File.expand_path("../..", __dir__), "scripts", "ingest_raw.rb")

  def setup
    require "vbrain"
    VBrain::Paths.ensure_dirs!
    @sha_to_cleanup = []
  end

  def teardown
    return if @sha_to_cleanup.empty?

    VBrain::DB.open do |db|
      @sha_to_cleanup.each do |sha|
        rows = db.execute("SELECT path FROM raw_sources WHERE sha256 = ?", [sha])
        rows.each { |r| File.delete(r["path"]) if File.exist?(r["path"]) }
        db.execute("DELETE FROM raw_sources WHERE sha256 = ?", [sha])
      end
    end
  end

  def test_ingest_text_file_writes_raw_and_extracted
    with_tmpdir do |dir|
      input = File.join(dir, "note_#{Time.now.to_f}.md")
      content = "# Sample\n\nUnique marker #{Time.now.to_f}\n"
      File.write(input, content)
      sha = Digest::SHA256.hexdigest(File.read(input))
      @sha_to_cleanup << sha

      stdout, stderr, status = Open3.capture3("bundle", "exec", "ruby", SCRIPT, input,
                                              chdir: File.expand_path("../..", __dir__))
      assert status.success?, "ingest failed: #{stderr}"
      data = JSON.parse(stdout)
      assert_equal "text", data["source_type"]
      assert data["raw_id"]
      assert File.exist?(data["raw_path"])
      assert File.exist?(data["extracted_path"])
      assert_equal content, File.read(data["extracted_path"])
    end
  end

  def test_ingest_unknown_returns_unknown_type
    with_tmpdir do |dir|
      input = File.join(dir, "blob.bin")
      File.binwrite(input, "PK\x03\x04\x00\xffstuff")
      stdout, _stderr, status = Open3.capture3("bundle", "exec", "ruby", SCRIPT, input,
                                               chdir: File.expand_path("../..", __dir__))
      assert status.success?
      data = JSON.parse(stdout)
      assert_equal "unknown", data["source_type"]
    end
  end

  def test_ingest_dedupes_by_sha256
    with_tmpdir do |dir|
      input = File.join(dir, "dup_#{Time.now.to_f}.txt")
      content = "dedupe-marker-#{Time.now.to_f}\n"
      File.write(input, content)
      sha = Digest::SHA256.hexdigest(File.read(input))
      @sha_to_cleanup << sha

      stdout1, _, st1 = Open3.capture3("bundle", "exec", "ruby", SCRIPT, input,
                                       chdir: File.expand_path("../..", __dir__))
      assert st1.success?
      first = JSON.parse(stdout1)

      stdout2, _, st2 = Open3.capture3("bundle", "exec", "ruby", SCRIPT, input,
                                       chdir: File.expand_path("../..", __dir__))
      assert st2.success?
      second = JSON.parse(stdout2)
      assert_equal true, second["duplicate"]
      assert_equal first["raw_id"], second["raw_id"]
    end
  end
end
