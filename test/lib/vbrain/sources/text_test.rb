require "test_helper"
require "vbrain/sources"

class SourcesTextTest < Minitest::Test
  def test_detect_md_file
    with_tmpdir do |dir|
      f = File.join(dir, "foo.md")
      File.write(f, "# hi\n")
      assert VBrain::Sources::Text.detect?(f)
    end
  end

  def test_detect_txt_file
    with_tmpdir do |dir|
      f = File.join(dir, "foo.txt")
      File.write(f, "hello\n")
      assert VBrain::Sources::Text.detect?(f)
    end
  end

  def test_detect_extensionless_utf8_text
    with_tmpdir do |dir|
      f = File.join(dir, "notes")
      File.write(f, "some text\n")
      assert VBrain::Sources::Text.detect?(f)
    end
  end

  def test_detect_rejects_binary
    with_tmpdir do |dir|
      f = File.join(dir, "blob.bin")
      File.binwrite(f, "PK\x03\x04binarystuff\x00\xff\xfe")
      refute VBrain::Sources::Text.detect?(f)
    end
  end

  def test_detect_rejects_directory
    with_tmpdir do |dir|
      refute VBrain::Sources::Text.detect?(dir)
    end
  end

  def test_extract_writes_passthrough_content
    with_tmpdir do |dir|
      src = File.join(dir, "in.md")
      out = File.join(dir, "out.txt")
      File.write(src, "Olá Mundo\n")
      VBrain::Sources::Text.extract(src, out)
      assert_equal "Olá Mundo\n", File.read(out)
    end
  end

  def test_kind_key
    assert_equal "text", VBrain::Sources::Text.kind_key
  end
end

class SourcesDispatcherTest < Minitest::Test
  def test_detect_returns_text_for_md
    with_tmpdir do |dir|
      f = File.join(dir, "x.md")
      File.write(f, "# x")
      assert_equal VBrain::Sources::Text, VBrain::Sources.detect(f)
    end
  end

  def test_detect_returns_nil_for_binary
    with_tmpdir do |dir|
      f = File.join(dir, "b.bin")
      File.binwrite(f, "\x00\xff\x00\xff")
      assert_nil VBrain::Sources.detect(f)
    end
  end

  def test_for_lookup_by_kind
    assert_equal VBrain::Sources::Text, VBrain::Sources.for("text")
    assert_nil VBrain::Sources.for("nonexistent")
  end

  def test_kinds_includes_text
    assert_includes VBrain::Sources.kinds, "text"
  end
end
