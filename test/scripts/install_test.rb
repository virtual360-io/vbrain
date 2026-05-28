require "test_helper"

class InstallCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  INSTALL      = File.join(PROJECT_ROOT, "scripts", "install.rb")

  def test_install_renders_skill_md_with_absolute_paths
    Dir.mktmpdir do |target|
      stdout, stderr, status = Open3.capture3(
        "bundle", "exec", "ruby", INSTALL, "--target", target,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "install failed: #{stderr}\n#{stdout}"

      add_skill = File.join(target, "add-knowledge", "SKILL.md")
      assert File.exist?(add_skill)
      content = File.read(add_skill)
      assert_includes content, "BUNDLE_GEMFILE=#{PROJECT_ROOT}/Gemfile"
      assert_includes content, "#{PROJECT_ROOT}/scripts/ingest_raw.rb"
      refute_match %r{^bundle exec ruby scripts/}m, content,
                   "should not leave relative bundle exec commands"

      prompt_text = File.join(target, "add-knowledge", "prompts", "chunker", "text.md")
      assert File.exist?(prompt_text)

      query_skill = File.join(target, "query-knowledge", "SKILL.md")
      assert File.exist?(query_skill)
      assert_includes File.read(query_skill), "#{PROJECT_ROOT}/scripts/query.rb"
    end
  end

  def test_install_is_idempotent_and_prunes_obsolete
    Dir.mktmpdir do |target|
      _, _, st1 = Open3.capture3("bundle", "exec", "ruby", INSTALL, "--target", target,
                                 chdir: PROJECT_ROOT)
      assert st1.success?

      obsolete = File.join(target, "add-knowledge", "OLD.md")
      File.write(obsolete, "obsolete\n")

      _, _, st2 = Open3.capture3("bundle", "exec", "ruby", INSTALL, "--target", target,
                                 chdir: PROJECT_ROOT)
      assert st2.success?
      refute File.exist?(obsolete), "obsolete file should be pruned"
    end
  end

  def test_dry_run_writes_nothing
    Dir.mktmpdir do |target|
      _, _, status = Open3.capture3(
        "bundle", "exec", "ruby", INSTALL, "--target", target, "--dry-run",
        chdir: PROJECT_ROOT
      )
      assert status.success?
      assert_empty Dir.children(target)
    end
  end
end
