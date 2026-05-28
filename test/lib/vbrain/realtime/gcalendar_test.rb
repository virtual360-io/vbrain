require "test_helper"
require "vbrain"

class RealtimeGcalendarTest < Minitest::Test
  CALENDARS = [
    { "id" => "primary",       "summary" => "Victor",    "timezone" => "America/Sao_Paulo" },
    { "id" => "work@v360.io",  "summary" => "V360 Work", "timezone" => "America/Sao_Paulo" }
  ].freeze

  def test_save_config_writes_yaml_and_returns_normalized
    with_isolated_data_home do |_|
      saved = VBrain::Realtime::Gcalendar.save_config!(calendars: CALENDARS)

      assert_equal 2, saved.size
      assert File.exist?(VBrain::Realtime::Gcalendar.config_path)
      loaded = VBrain::Realtime::Gcalendar.load_config
      assert_equal "primary", loaded.first["id"]
      assert_equal "Victor",  loaded.first["summary"]
    end
  end

  def test_save_config_rejects_empty_list
    with_isolated_data_home do |_|
      assert_raises(ArgumentError) do
        VBrain::Realtime::Gcalendar.save_config!(calendars: [])
      end
    end
  end

  def test_save_config_skips_calendars_with_blank_id
    with_isolated_data_home do |_|
      saved = VBrain::Realtime::Gcalendar.save_config!(
        calendars: CALENDARS + [{ "id" => "", "summary" => "Blank" }]
      )
      assert_equal 2, saved.size
      refute saved.any? { |c| c["summary"] == "Blank" }
    end
  end

  def test_write_wiki_page_renders_phantom_with_realtime_kind
    with_isolated_data_home do |_|
      VBrain::Paths.ensure_dirs!
      VBrain::Realtime::Gcalendar.save_config!(calendars: CALENDARS)
      path = VBrain::Realtime::Gcalendar.write_wiki_page!(calendars: CALENDARS)

      assert File.exist?(path)
      parsed = VBrain::Page.parse(path)
      assert_equal "realtime",  parsed.frontmatter["kind"]
      assert_equal "gcalendar", parsed.frontmatter["source"]
      assert_equal 2,           Array(parsed.frontmatter["calendars"]).size
      assert_includes parsed.body, "agenda"
      assert_includes parsed.body, "reunião"
      assert_includes parsed.body, "primary"
      assert_includes parsed.body, "work@v360.io"
    end
  end

  def test_frontmatter_normalizes_symbol_keys
    fm = VBrain::Realtime::Gcalendar.frontmatter([{ id: "x", summary: "X", timezone: "UTC" }])
    cal = fm["calendars"].first
    assert_equal "x", cal["id"]
    assert_equal "X", cal["summary"]
  end

  def test_body_collapses_duplicated_summary_and_id
    body = VBrain::Realtime::Gcalendar.body([
      { "id" => "victor@v360.io", "summary" => "victor@v360.io", "timezone" => "UTC" }
    ])
    assert_includes body, "`victor@v360.io`"
    refute_match(/victor@v360\.io.*victor@v360\.io/, body.lines.find { |l| l.start_with?("- ") })
  end

  def test_body_keeps_distinct_summary_and_id
    body = VBrain::Realtime::Gcalendar.body([
      { "id" => "primary", "summary" => "Victor", "timezone" => "UTC" }
    ])
    assert_includes body, "Victor (`primary`)"
  end

  def test_body_falls_back_to_id_when_summary_blank
    body = VBrain::Realtime::Gcalendar.body([
      { "id" => "abc", "summary" => "", "timezone" => "UTC" }
    ])
    assert_includes body, "`abc`"
  end
end
