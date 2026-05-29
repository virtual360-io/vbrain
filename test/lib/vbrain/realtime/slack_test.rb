require "test_helper"
require "vbrain"

class RealtimeSlackTest < Minitest::Test
  CHANNELS = [
    { "id" => "C0GERAL", "name" => "geral" },
    { "id" => "C0PROD",  "name" => "produto" }
  ].freeze

  def test_save_config_writes_yaml_and_returns_normalized
    with_isolated_data_home do |_|
      saved = VBrain::Realtime::Slack.save_config!(channels: CHANNELS)

      assert_equal 2, saved.size
      assert File.exist?(VBrain::Realtime::Slack.config_path)
      loaded = VBrain::Realtime::Slack.load_config
      assert_equal "C0GERAL", loaded.first["id"]
      assert_equal "geral",   loaded.first["name"]
    end
  end

  # O PORQUÊ: ao contrário de gmail/gcalendar, lista vazia é válida e significa
  # busca global no workspace inteiro. Se alguém "endurecer" isso pra exigir >=1
  # canal, este teste falha — que é o ponto.
  def test_save_config_accepts_empty_as_global_scope
    with_isolated_data_home do |_|
      saved = VBrain::Realtime::Slack.save_config!(channels: [])

      assert_equal [], saved
      assert File.exist?(VBrain::Realtime::Slack.config_path)
      assert_equal [], VBrain::Realtime::Slack.load_config
      assert VBrain::Realtime::Slack.global?([])
    end
  end

  def test_global_predicate
    refute VBrain::Realtime::Slack.global?(CHANNELS)
    assert VBrain::Realtime::Slack.global?([])
    assert VBrain::Realtime::Slack.global?([{ "id" => "", "name" => "" }])
  end

  def test_normalize_accepts_channel_id_key_from_mcp_shape
    with_isolated_data_home do |_|
      saved = VBrain::Realtime::Slack.save_config!(channels: [
        { "channel_id" => "C0ABC", "name" => "anuncios" }
      ])
      assert_equal "C0ABC", saved.first["id"]
    end
  end

  def test_write_wiki_page_renders_phantom_with_realtime_kind
    with_isolated_data_home do |_|
      VBrain::Paths.ensure_dirs!
      VBrain::Realtime::Slack.save_config!(channels: CHANNELS)
      path = VBrain::Realtime::Slack.write_wiki_page!(channels: CHANNELS)

      assert File.exist?(path)
      parsed = VBrain::Page.parse(path)
      assert_equal "realtime", parsed.frontmatter["kind"]
      assert_equal "slack",    parsed.frontmatter["source"]
      assert_equal 2,          Array(parsed.frontmatter["channels"]).size
      assert_includes parsed.body, "slack"
      assert_includes parsed.body, "canal"
      assert_includes parsed.body, "geral"
      assert_includes parsed.body, "C0PROD"
    end
  end

  def test_global_wiki_page_documents_global_scope
    body = VBrain::Realtime::Slack.body([])
    assert_includes body, "global"
    refute_includes body, "Canais conectados"
  end

  def test_channel_filter_prefers_id
    assert_equal "in:<#C0GERAL>",
                 VBrain::Realtime::Slack.channel_filter({ "id" => "C0GERAL", "name" => "geral" })
  end

  def test_channel_filter_falls_back_to_name
    assert_equal "in:#geral",
                 VBrain::Realtime::Slack.channel_filter({ "id" => "", "name" => "geral" })
  end

  def test_channel_filter_empty
    assert_equal "", VBrain::Realtime::Slack.channel_filter({ "id" => "", "name" => "" })
  end

  def test_format_channel_collapses_when_no_name
    body = VBrain::Realtime::Slack.body([{ "id" => "C0X", "name" => "" }])
    assert_includes body, "`C0X`"
  end

  def test_format_channel_keeps_name_and_id
    assert_equal "#geral (`C0GERAL`)",
                 VBrain::Realtime::Slack.format_channel({ "id" => "C0GERAL", "name" => "geral" })
  end
end
