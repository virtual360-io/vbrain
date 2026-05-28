require "test_helper"
require "vbrain/sources"

class SourcesTwitterTest < Minitest::Test
  FIXTURE = File.expand_path("../../../fixtures/twitter/alok_link_tweet.json", __dir__)
  TWEET_URL = "https://x.com/alokbishoyi97/status/2059610305408462898"
  TWEET_ID  = "2059610305408462898"

  def test_detect_x_and_twitter_urls
    assert VBrain::Sources::Twitter.detect?("https://x.com/alok/status/123")
    assert VBrain::Sources::Twitter.detect?("https://twitter.com/alok/status/123")
    assert VBrain::Sources::Twitter.detect?("http://www.twitter.com/alok/status/123")
    assert VBrain::Sources::Twitter.detect?("https://mobile.x.com/alok/status/123")
  end

  def test_detect_rejects_non_tweet_urls
    refute VBrain::Sources::Twitter.detect?("https://x.com/alok")
    refute VBrain::Sources::Twitter.detect?("https://x.com/alok/photo/123")
    refute VBrain::Sources::Twitter.detect?("https://example.com/x/alok/status/123")
    refute VBrain::Sources::Twitter.detect?("/tmp/foo.txt")
  end

  def test_kind_key
    assert_equal "tweet", VBrain::Sources::Twitter.kind_key
  end

  def test_parse_id_extracts_numeric_status_id
    assert_equal TWEET_ID, VBrain::Sources::Twitter.parse_id(TWEET_URL)
    assert_equal "987", VBrain::Sources::Twitter.parse_id("https://twitter.com/foo/status/987")
  end

  def test_parse_id_raises_for_non_tweet
    assert_raises(VBrain::Sources::Twitter::FetchError) do
      VBrain::Sources::Twitter.parse_id("https://x.com/foo")
    end
  end

  def test_compute_token_is_deterministic_and_non_empty
    a = VBrain::Sources::Twitter.compute_token(TWEET_ID)
    b = VBrain::Sources::Twitter.compute_token(TWEET_ID)
    assert_equal a, b
    refute_empty a
    assert_match(/\A[0-9]+\z/, a)
  end

  def test_extract_from_json_renders_metadata_and_text
    json = File.read(FIXTURE)
    md = VBrain::Sources::Twitter.extract_from_json(json, url: TWEET_URL, id: TWEET_ID)
    assert_includes md, "# Tweet de Alok Bishoyi"
    assert_includes md, "- Tweet ID: #{TWEET_ID}"
    assert_includes md, "- Autor: Alok Bishoyi (@alokbishoyi97)"
    assert_includes md, "- Data: 2026-05-27"
    assert_includes md, "- Idioma: zxx"
    assert_includes md, "## Texto do tweet"
    assert_includes md, "http://x.com/i/article/2059581224960835584",
                    "t.co link must be expanded"
    assert_includes md, "## Links citados"
    assert_includes md, "[x.com/i/article/2059…](http://x.com/i/article/2059581224960835584)"
  end

  def test_extract_from_json_renders_embedded_article_preview_when_no_full_text
    json = File.read(FIXTURE)
    md = VBrain::Sources::Twitter.extract_from_json(json, url: TWEET_URL, id: TWEET_ID)
    assert_includes md, "## Artigo embutido"
    assert_includes md, "Using Autoresearch to improve harness skills"
    assert_includes md, "self-improving agents are here"
    assert_includes md, "body completo do artigo só é acessível"
  end

  def test_extract_from_json_includes_full_article_when_provided
    json = File.read(FIXTURE)
    full = "Using Autoresearch to improve harness skills\n\n" \
           "self-improving agents are here\nThe most interesting shift in AI right now... " \
           "(and a lot more content that exceeds the preview length significantly to trigger the threshold). " \
           "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " * 8
    md = VBrain::Sources::Twitter.extract_from_json(json, url: TWEET_URL, id: TWEET_ID,
                                                     article_full_text: full)
    assert_includes md, "Body completo"
    assert_includes md, "Playwright"
    refute_includes md, "body completo do artigo só é acessível"
    assert_includes md, "and a lot more content"
  end

  def test_clean_article_text_strips_x_boilerplate
    raw = "Don’t miss what’s happening\nLog in\nThe Title\n\nbody starts here\n\n© 2026 X Corp."
    cleaned = VBrain::Sources::Twitter.clean_article_text(raw, title: "The Title")
    refute_includes cleaned, "© 2026 X Corp."
    refute_includes cleaned, "Don’t miss"
    assert_includes cleaned, "body starts here"
  end

  def test_extract_from_json_skips_article_section_when_absent
    fake = {
      "user" => { "name" => "X", "screen_name" => "x" },
      "created_at" => "2026-01-01T00:00:00Z",
      "text" => "hello"
    }
    md = VBrain::Sources::Twitter.extract_from_json(fake.to_json, url: "https://x.com/x/status/1", id: "1")
    refute_includes md, "Artigo embutido"
  end

  def test_extract_from_json_signals_empty_text_when_only_link
    fake = {
      "user" => { "name" => "X", "screen_name" => "x" },
      "created_at" => "2026-01-01T00:00:00Z",
      "text" => "https://t.co/abc",
      "entities" => { "urls" => [{ "url" => "https://t.co/abc",
                                   "expanded_url" => "https://elsewhere.test/article",
                                   "display_url" => "elsewhere.test/article" }] }
    }
    md = VBrain::Sources::Twitter.extract_from_json(fake.to_json, url: "https://x.com/x/status/1", id: "1")
    # After URL expansion, text body is just a URL — chunker upstream returns 0 chunks.
    # We still must render the metadata correctly and include the link in references.
    assert_includes md, "https://elsewhere.test/article"
  end

  def test_extract_from_json_renders_media_when_present
    fake = {
      "user" => { "name" => "X", "screen_name" => "x" },
      "created_at" => "2026-01-01T00:00:00Z",
      "text" => "foo",
      "mediaDetails" => [{ "type" => "photo", "media_url_https" => "https://pbs.test/img.jpg" }]
    }
    md = VBrain::Sources::Twitter.extract_from_json(fake.to_json, url: "https://x.com/x/status/1", id: "1")
    assert_includes md, "## Mídia"
    assert_includes md, "photo: https://pbs.test/img.jpg"
  end

  def test_dispatcher_prefers_twitter_over_url_for_tweet
    assert_equal VBrain::Sources::Twitter,
                 VBrain::Sources.detect("https://x.com/foo/status/1")
  end

  def test_dispatcher_falls_back_to_url_for_non_tweet_https
    assert_equal VBrain::Sources::Url,
                 VBrain::Sources.detect("https://example.com/article")
  end
end
