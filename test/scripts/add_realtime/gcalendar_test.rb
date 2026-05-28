require "test_helper"
require "vbrain"

class AddRealtimeGcalendarCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../../..", __dir__)
  SCRIPT       = File.join(PROJECT_ROOT, "scripts", "add_realtime", "gcalendar.rb")
  REINDEX      = File.join(PROJECT_ROOT, "scripts", "reindex.rb")
  QUERY        = File.join(PROJECT_ROOT, "scripts", "query.rb")

  def test_script_writes_config_and_wiki_and_returns_json
    with_isolated_data_home do |dir|
      payload = JSON.generate(
        "calendars" => [
          { "id" => "primary",      "summary" => "Victor",    "timezone" => "America/Sao_Paulo" },
          { "id" => "work@v360.io", "summary" => "V360 Work", "timezone" => "America/Sao_Paulo" }
        ]
      )

      stdout, stderr, status = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", SCRIPT, "--calendars-json", payload,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "script failed: #{stderr}\n#{stdout}"

      data = JSON.parse(stdout)
      assert_equal "gcalendar", data["source"]
      assert_equal "_realtime/gcalendar.md", data["wiki_path"]
      assert File.exist?(data["config_path"])
      assert File.exist?(File.join(dir, "wiki", data["wiki_path"]))
      assert_equal File.join(dir, "wiki", data["wiki_path"]), data["wiki_path_abs"]
      assert File.exist?(data["wiki_path_abs"])
    end
  end

  def test_script_rejects_empty_payload
    with_isolated_data_home do |dir|
      _, stderr, status = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", SCRIPT, "--calendars-json", '{"calendars":[]}',
        chdir: PROJECT_ROOT
      )
      refute status.success?
      assert_includes stderr, "at least one"
    end
  end

  def test_phantom_page_is_searchable_after_reindex
    with_isolated_data_home do |dir|
      payload = JSON.generate(
        "calendars" => [{ "id" => "primary", "summary" => "Victor", "timezone" => "UTC" }]
      )

      _, _, st1 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", SCRIPT, "--calendars-json", payload,
        chdir: PROJECT_ROOT
      )
      assert st1.success?

      _, _, st2 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", REINDEX,
        chdir: PROJECT_ROOT
      )
      assert st2.success?

      stdout, _, st3 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", QUERY, "reunião", "--format", "json",
        chdir: PROJECT_ROOT
      )
      assert st3.success?
      data = JSON.parse(stdout)
      hit = data["results"].find { |r| r["path"] == "_realtime/gcalendar.md" }
      assert hit, "phantom page should be found via FTS5: #{data['results'].inspect}"
      assert_equal "realtime", hit["kind"]
    end
  end
end
