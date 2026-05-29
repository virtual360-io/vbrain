require "test_helper"
require "vbrain"

class CommitCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  SCRIPT       = File.join(PROJECT_ROOT, "scripts", "commit.rb")

  def test_no_op_when_data_home_not_a_repo
    Dir.mktmpdir do |home|
      ENV["VBRAIN_HOME"] = home
      stdout, _stderr, status = Open3.capture3(
        "bundle", "exec", "ruby", SCRIPT, "--message", "x",
        chdir: PROJECT_ROOT
      )
      assert status.success?
      data = JSON.parse(stdout)
      assert_equal false, data["committed"]
      assert_match(/no git repo/, data["reason"])
    end
  ensure
    ENV["VBRAIN_HOME"] = VBRAIN_TEST_HOME
  end

  def test_commits_when_changes_present
    Dir.mktmpdir do |home|
      ENV["VBRAIN_HOME"] = home
      VBrain::Paths.ensure_dirs!
      VBrain::Git.init!(home)
      File.write(File.join(home, "wiki", "x.md"), "# x\n")

      stdout, _stderr, status = Open3.capture3(
        "bundle", "exec", "ruby", SCRIPT, "--message", "add: x", "--no-push",
        chdir: PROJECT_ROOT
      )
      assert status.success?, "commit failed: #{stdout}"
      data = JSON.parse(stdout)
      assert_equal true, data["committed"]
      assert_equal false, data["pushed"]
      assert_equal "no-push", data["reason"]
    end
  ensure
    ENV["VBRAIN_HOME"] = VBRAIN_TEST_HOME
  end
end
