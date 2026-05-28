require "test_helper"
require "vbrain/slug"

class SlugTest < Minitest::Test
  def test_basic_ascii_lowercase
    assert_equal "hello-world", VBrain::Slug.from("Hello World")
  end

  def test_nfkd_strips_diacritics
    assert_equal "sao-jose", VBrain::Slug.from("São José")
    assert_equal "acucar", VBrain::Slug.from("açúcar")
    assert_equal "naive", VBrain::Slug.from("naïve")
  end

  def test_collapses_separators_and_punctuation
    assert_equal "foo-bar-baz", VBrain::Slug.from("foo___bar... baz!!!")
  end

  def test_trims_leading_and_trailing
    assert_equal "foo-bar", VBrain::Slug.from("---foo bar---")
  end

  def test_truncates_to_max_length_without_trailing_dash
    long = "a" * 100 + " " + "b" * 100
    slug = VBrain::Slug.from(long, max_length: 50)
    assert slug.length <= 50
    refute slug.end_with?("-"), "should not end with dash after truncation"
  end

  def test_empty_or_punct_only_raises
    assert_raises(VBrain::Slug::Error) { VBrain::Slug.from("") }
    assert_raises(VBrain::Slug::Error) { VBrain::Slug.from("   ") }
    assert_raises(VBrain::Slug::Error) { VBrain::Slug.from("!!! ??? ...") }
    assert_raises(VBrain::Slug::Error) { VBrain::Slug.from(nil) }
  end

  def test_preserves_digits
    assert_equal "version-2-0", VBrain::Slug.from("Version 2.0")
  end
end
