require "test_helper"
require "vbrain"

class InitRepoCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  SCRIPT       = File.join(PROJECT_ROOT, "scripts", "init_repo.rb")

  def test_initializes_local_only_when_github_none
    Dir.mktmpdir do |home|
      ENV["VBRAIN_HOME"] = home
      stdout, _stderr, status = Open3.capture3(
        "bundle", "exec", "ruby", SCRIPT,
        chdir: PROJECT_ROOT
      )
      assert status.success?
      data = JSON.parse(stdout)
      assert_equal true, data["initialized"]
      assert_equal "main", data["branch"]
      assert_equal false, data["has_remote"]
      assert File.exist?(File.join(home, ".gitignore"))
    end
  ensure
    ENV["VBRAIN_HOME"] = VBRAIN_TEST_HOME
  end

  def test_idempotent_when_repo_already_exists
    Dir.mktmpdir do |home|
      ENV["VBRAIN_HOME"] = home
      VBrain::Paths.ensure_dirs!
      VBrain::Git.init!(home)
      stdout, _stderr, status = Open3.capture3(
        "bundle", "exec", "ruby", SCRIPT,
        chdir: PROJECT_ROOT
      )
      assert status.success?
      data = JSON.parse(stdout)
      assert_equal false, data["initialized"]
      assert_match(/already a repo/, data["reason"])
    end
  ensure
    ENV["VBRAIN_HOME"] = VBRAIN_TEST_HOME
  end

  def test_signals_needs_gh_when_gh_missing_for_github_request
    skip "gh is installed on this machine — needs_gh path not exercised" if system("which gh > /dev/null 2>&1")

    Dir.mktmpdir do |home|
      ENV["VBRAIN_HOME"] = home
      stdout, _, status = Open3.capture3(
        "bundle", "exec", "ruby", SCRIPT, "--github", "private",
        chdir: PROJECT_ROOT
      )
      assert status.success?
      data = JSON.parse(stdout)
      assert_equal true, data["needs_gh"]
      assert_equal false, data["initialized"]
    end
  ensure
    ENV["VBRAIN_HOME"] = VBRAIN_TEST_HOME
  end
end
