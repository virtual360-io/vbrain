require "test_helper"
require "vbrain/sources"

class SourcesUrlTest < Minitest::Test
  def test_detect_http_and_https
    assert VBrain::Sources::Url.detect?("https://example.com/x")
    assert VBrain::Sources::Url.detect?("http://example.com")
    assert VBrain::Sources::Url.detect?("HTTPS://EXAMPLE.com")
  end

  def test_detect_rejects_non_url
    refute VBrain::Sources::Url.detect?("/tmp/foo.txt")
    refute VBrain::Sources::Url.detect?("ftp://example.com")
    refute VBrain::Sources::Url.detect?("example.com")
    refute VBrain::Sources::Url.detect?("")
  end

  def test_kind_key
    assert_equal "url", VBrain::Sources::Url.kind_key
  end

  def test_copy_to_raw_writes_markdown_with_url_sha
    sample = "# Title\n\nSome article content.\n"
    with_isolated_data_home do
      VBrain::Paths.ensure_dirs!
      VBrain::Sources::Url.stub(:fetch_jina, sample) do
        info = VBrain::Sources::Url.copy_to_raw("https://example.com/start",
                                                VBrain::Paths.raw_dir, "20260528T000000Z")
        assert File.exist?(info["path"]), "raw markdown file must exist"
        assert info["original_filename"].end_with?(".md")
        assert_equal sample, File.read(info["path"])
        assert_equal Digest::SHA256.hexdigest("https://example.com/start\n#{sample}"),
                     info["sha256"]
        assert_equal sample, info["markdown"]
      end
    end
  end

  def test_extract_uses_cached_markdown_when_provided
    with_isolated_data_home do |dir|
      out = File.join(dir, "extract.txt")
      VBrain::Sources::Url.extract("https://example.com", out,
                                   raw_info: { "markdown" => "# Cached\n" })
      assert_equal "# Cached\n", File.read(out)
    end
  end

  def test_extract_fetches_when_no_cache
    with_isolated_data_home do |dir|
      out = File.join(dir, "extract.txt")
      VBrain::Sources::Url.stub(:fetch_jina, "# Fresh\n") do
        VBrain::Sources::Url.extract("https://example.com", out)
      end
      assert_equal "# Fresh\n", File.read(out)
    end
  end

  def test_dispatcher_prefers_url_over_text_for_url_string
    assert_equal VBrain::Sources::Url, VBrain::Sources.detect("https://example.com")
  end
end
