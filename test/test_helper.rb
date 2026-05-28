$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "minitest/autorun"
require "tmpdir"
require "fileutils"
require "json"
require "open3"

VBRAIN_TEST_HOME = Dir.mktmpdir("vbrain-test-home-")
ENV["VBRAIN_HOME"] = VBRAIN_TEST_HOME

Minitest.after_run do
  FileUtils.remove_entry(VBRAIN_TEST_HOME) if File.directory?(VBRAIN_TEST_HOME)
end

module TestHelpers
  def with_tmpdir
    Dir.mktmpdir("vbrain-test-") do |dir|
      yield dir
    end
  end

  def project_root
    File.expand_path("..", __dir__)
  end

  def fixture_path(*parts)
    File.join(project_root, "test", "fixtures", *parts)
  end

  def script_path(name)
    File.join(project_root, "scripts", name)
  end

  def with_isolated_data_home
    Dir.mktmpdir("vbrain-data-") do |dir|
      old = ENV["VBRAIN_HOME"]
      ENV["VBRAIN_HOME"] = dir
      begin
        yield dir
      ensure
        ENV["VBRAIN_HOME"] = old
      end
    end
  end
end

class Minitest::Test
  include TestHelpers
end
