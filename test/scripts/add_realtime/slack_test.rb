require "test_helper"
require "vbrain"

class AddRealtimeSlackCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../../..", __dir__)
  SCRIPT       = File.join(PROJECT_ROOT, "scripts", "add_realtime", "slack.rb")
  REINDEX      = File.join(PROJECT_ROOT, "scripts", "reindex.rb")
  QUERY        = File.join(PROJECT_ROOT, "scripts", "query.rb")

  def test_script_writes_config_and_wiki_and_returns_json
    with_isolated_data_home do |dir|
      payload = JSON.generate(
        "channels" => [
          { "id" => "C0GERAL", "name" => "geral" },
          { "id" => "C0PROD",  "name" => "produto" }
        ]
      )

      stdout, stderr, status = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", SCRIPT, "--channels-json", payload,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "script failed: #{stderr}\n#{stdout}"

      data = JSON.parse(stdout)
      assert_equal "slack", data["source"]
      assert_equal "filtered", data["mode"]
      assert_equal "_realtime/slack.md", data["wiki_path"]
      assert File.exist?(data["config_path"])
      assert File.exist?(data["wiki_path_abs"])
      assert_equal File.join(dir, "wiki", data["wiki_path"]), data["wiki_path_abs"]
    end
  end

  # Lista vazia é aceita: modo global (busca no workspace inteiro).
  def test_script_accepts_empty_channels_as_global
    with_isolated_data_home do |dir|
      stdout, stderr, status = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", SCRIPT, "--channels-json", '{"channels":[]}',
        chdir: PROJECT_ROOT
      )
      assert status.success?, "script failed: #{stderr}\n#{stdout}"

      data = JSON.parse(stdout)
      assert_equal "global", data["mode"]
      assert_equal [], data["channels"]
      assert File.exist?(data["wiki_path_abs"])
    end
  end

  def test_phantom_page_is_searchable_after_reindex
    with_isolated_data_home do |dir|
      _, _, st1 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", SCRIPT, "--channels-json", '{"channels":[]}',
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
        "bundle", "exec", "ruby", QUERY, "slack", "--format", "json",
        chdir: PROJECT_ROOT
      )
      assert st3.success?
      data = JSON.parse(stdout)
      hit = data["results"].find { |r| r["path"] == "_realtime/slack.md" }
      assert hit, "phantom slack page should be found via FTS5: #{data['results'].inspect}"
      assert_equal "realtime", hit["kind"]
    end
  end
end
