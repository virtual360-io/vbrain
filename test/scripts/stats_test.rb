require "test_helper"
require "vbrain"

class StatsCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  STATS        = File.join(PROJECT_ROOT, "scripts", "stats.rb")

  def test_stats_returns_json_with_expected_keys
    stdout, stderr, status = Open3.capture3("bundle", "exec", "ruby", STATS,
                                            chdir: PROJECT_ROOT)
    assert status.success?, "stats failed: #{stderr}"
    data = JSON.parse(stdout)
    %w[data_home pages raw by_kind recent].each do |k|
      assert data.key?(k), "missing key #{k} in #{data}"
    end
    assert_kind_of Integer, data["pages"]
    assert_kind_of Hash,    data["by_kind"]
    assert_kind_of Array,   data["recent"]
  end
end
