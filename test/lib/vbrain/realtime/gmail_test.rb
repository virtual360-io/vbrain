require "test_helper"
require "vbrain"

class RealtimeGmailTest < Minitest::Test
  LABELS = [
    { "id" => "INBOX",    "name" => "Inbox" },
    { "id" => "Label_5",  "name" => "JCA" }
  ].freeze

  def test_save_config_writes_yaml_and_returns_normalized
    with_isolated_data_home do |_|
      saved = VBrain::Realtime::Gmail.save_config!(labels: LABELS)

      assert_equal 2, saved.size
      assert File.exist?(VBrain::Realtime::Gmail.config_path)
      loaded = VBrain::Realtime::Gmail.load_config
      assert_equal "INBOX", loaded.first["id"]
      assert_equal "Inbox", loaded.first["name"]
    end
  end

  def test_save_config_rejects_empty_list
    with_isolated_data_home do |_|
      assert_raises(ArgumentError) do
        VBrain::Realtime::Gmail.save_config!(labels: [])
      end
    end
  end

  def test_save_config_accepts_labelId_key_from_mcp_shape
    with_isolated_data_home do |_|
      saved = VBrain::Realtime::Gmail.save_config!(labels: [
        { "labelId" => "Label_7", "name" => "multi-forward" }
      ])
      assert_equal "Label_7", saved.first["id"]
    end
  end

  def test_write_wiki_page_renders_phantom_with_realtime_kind
    with_isolated_data_home do |_|
      VBrain::Paths.ensure_dirs!
      VBrain::Realtime::Gmail.save_config!(labels: LABELS)
      path = VBrain::Realtime::Gmail.write_wiki_page!(labels: LABELS)

      assert File.exist?(path)
      parsed = VBrain::Page.parse(path)
      assert_equal "realtime", parsed.frontmatter["kind"]
      assert_equal "gmail",    parsed.frontmatter["source"]
      assert_equal 2,          Array(parsed.frontmatter["labels"]).size
      assert_includes parsed.body, "email"
      assert_includes parsed.body, "inbox"
      assert_includes parsed.body, "INBOX"
      assert_includes parsed.body, "Label_5"
    end
  end

  def test_format_label_collapses_when_name_equals_id
    body = VBrain::Realtime::Gmail.body([{ "id" => "Label_X", "name" => "Label_X" }])
    line = body.lines.find { |l| l.start_with?("- ") }
    refute_match(/Label_X.*Label_X/, line)
  end

  def test_format_label_keeps_distinct_name_and_id
    body = VBrain::Realtime::Gmail.body([{ "id" => "Label_5", "name" => "JCA" }])
    assert_includes body, "JCA (`Label_5`)"
  end

  def test_label_filter_clause_single
    assert_equal "label:INBOX", VBrain::Realtime::Gmail.label_filter_clause([
      { "id" => "INBOX", "name" => "Inbox" }
    ])
  end

  def test_label_filter_clause_multiple_wraps_in_parens_with_OR
    clause = VBrain::Realtime::Gmail.label_filter_clause([
      { "id" => "INBOX",     "name" => "Inbox" },
      { "id" => "IMPORTANT", "name" => "Important" },
      { "id" => "Label_5",   "name" => "JCA" }
    ])
    assert_equal "(label:INBOX OR label:IMPORTANT OR label:Label_5)", clause
  end

  def test_label_filter_clause_empty
    assert_equal "", VBrain::Realtime::Gmail.label_filter_clause([])
    assert_equal "", VBrain::Realtime::Gmail.label_filter_clause([{ "id" => "", "name" => "x" }])
  end
end
