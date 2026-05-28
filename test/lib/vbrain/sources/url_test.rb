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

  def test_extract_from_html_renders_title_url_and_text
    html = <<~HTML
      <html>
        <head>
          <title>Hello World</title>
          <meta property="og:title" content="Hello OG"/>
          <meta property="og:description" content="A nice description"/>
          <meta property="og:site_name" content="Example"/>
        </head>
        <body>
          <article>
            <h1>Hello</h1>
            <p>First paragraph with content.</p>
            <p>Second paragraph here.</p>
          </article>
          <script>console.log("strip me")</script>
        </body>
      </html>
    HTML

    md = VBrain::Sources::Url.extract_from_html(html, url: "https://example.com/post")
    assert_includes md, "# Hello OG"
    assert_includes md, "Source URL: https://example.com/post"
    assert_includes md, "Site: Example"
    assert_includes md, "## Resumo (Open Graph)"
    assert_includes md, "A nice description"
    assert_includes md, "## Conteúdo extraído"
    assert_includes md, "First paragraph with content."
    refute_includes md, "strip me", "script content must be removed"
  end

  def test_extract_from_html_falls_back_to_title_when_no_og
    html = "<html><head><title>Plain</title></head><body><p>Body text.</p></body></html>"
    md = VBrain::Sources::Url.extract_from_html(html, url: "https://x.test/")
    assert_includes md, "# Plain"
    assert_includes md, "Body text."
    refute_includes md, "## Resumo"
  end

  def test_extract_from_html_marks_empty_when_no_text
    html = "<html><head><title>Locked</title></head><body><script>x</script></body></html>"
    md = VBrain::Sources::Url.extract_from_html(html, url: "https://x.test/")
    assert_includes md, "(sem conteúdo textual extraível"
  end

  def test_copy_to_raw_writes_html_file_with_url_sha
    with_isolated_data_home do
      VBrain::Paths.ensure_dirs!
      VBrain::Sources::Url.stub(:fetch, ["<html><body>x</body></html>", "https://example.com/final", 200]) do
        info = VBrain::Sources::Url.copy_to_raw("https://example.com/start", VBrain::Paths.raw_dir, "20260528T000000Z")
        assert File.exist?(info["path"]), "raw HTML file must exist"
        assert info["original_filename"].end_with?(".html")
        assert_includes File.read(info["path"]), "<body>x</body>"
        assert info["sha256"]
        assert_equal "https://example.com/final", info["final_url"]
        assert_equal 200, info["http_status"]
      end
    end
  end

  def test_dispatcher_prefers_url_over_text_for_url_string
    assert_equal VBrain::Sources::Url, VBrain::Sources.detect("https://example.com")
  end
end
