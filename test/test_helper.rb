$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "minitest/autorun"
require "tmpdir"
require "fileutils"
require "json"
require "open3"

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
end

class Minitest::Test
  include TestHelpers
end
