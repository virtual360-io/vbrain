require "test_helper"

class InstallCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  INSTALL      = File.join(PROJECT_ROOT, "scripts", "install.rb")

  # Roda o install com um VBRAIN_HOME isolado (a base recebe scaffolding por
  # default; isolar evita poluir/commitar a base de teste compartilhada).
  def run_install(target, *extra)
    Dir.mktmpdir do |base|
      yield(*Open3.capture3(
        { "VBRAIN_HOME" => base }, "bundle", "exec", "ruby", INSTALL,
        "--target", target, *extra, chdir: PROJECT_ROOT
      ), base)
    end
  end

  def test_install_renders_skill_md_with_absolute_paths
    Dir.mktmpdir do |target|
      run_install(target) do |stdout, stderr, status, _base|
        assert status.success?, "install failed: #{stderr}\n#{stdout}"

        add_skill = File.join(target, "vbrain-add-knowledge", "SKILL.md")
        assert File.exist?(add_skill)
        content = File.read(add_skill)
        assert_includes content, "BUNDLE_GEMFILE=#{PROJECT_ROOT}/Gemfile"
        assert_includes content, "#{PROJECT_ROOT}/scripts/ingest_raw.rb"
        refute_match %r{^bundle exec ruby scripts/}m, content,
                     "should not leave relative bundle exec commands"

        prompt_text = File.join(target, "vbrain-add-knowledge", "prompts", "chunker", "text.md")
        assert File.exist?(prompt_text)

        query_skill = File.join(target, "vbrain-query-knowledge", "SKILL.md")
        assert File.exist?(query_skill)
        assert_includes File.read(query_skill), "#{PROJECT_ROOT}/scripts/query.rb"
      end
    end
  end

  def test_install_is_idempotent_and_prunes_obsolete
    Dir.mktmpdir do |target|
      run_install(target) { |_o, _e, st1, _b| assert st1.success? }

      obsolete = File.join(target, "vbrain-add-knowledge", "OLD.md")
      File.write(obsolete, "obsolete\n")

      run_install(target) { |_o, _e, st2, _b| assert st2.success? }
      refute File.exist?(obsolete), "obsolete file should be pruned"
    end
  end

  def test_install_removes_obsolete_skill_dirs
    Dir.mktmpdir do |target|
      FileUtils.mkdir_p(File.join(target, "add-knowledge"))
      File.write(File.join(target, "add-knowledge", "SKILL.md"), "old\n")
      FileUtils.mkdir_p(File.join(target, "query-knowledge"))
      File.write(File.join(target, "query-knowledge", "SKILL.md"), "old\n")

      run_install(target) do |_o, _e, status, _b|
        assert status.success?
      end
      refute Dir.exist?(File.join(target, "add-knowledge")),  "old add-knowledge should be removed"
      refute Dir.exist?(File.join(target, "query-knowledge")), "old query-knowledge should be removed"
      assert Dir.exist?(File.join(target, "vbrain-add-knowledge"))
      assert Dir.exist?(File.join(target, "vbrain-query-knowledge"))
    end
  end

  def test_dry_run_writes_nothing
    Dir.mktmpdir do |target|
      run_install(target, "--dry-run") do |_o, _e, status, base|
        assert status.success?
        assert_empty Dir.children(target)
        # dry-run não escreve a base tampouco
        refute File.exist?(File.join(base, "CLAUDE.md"))
      end
    end
  end

  # Por default (sem --dry-run), o install também escreve CLAUDE.md + skills
  # cruas na base (VBRAIN_HOME) — o que torna a base portável pra outros ambientes.
  def test_install_scaffolds_base_with_claude_md_and_skills
    Dir.mktmpdir do |target|
      run_install(target, "--no-seed") do |stdout, stderr, status, base|
        assert status.success?, "install failed: #{stderr}\n#{stdout}"
        assert File.exist?(File.join(base, "CLAUDE.md"))
        assert_includes File.read(File.join(base, "CLAUDE.md")), "SEMPRE use as skills"
        assert File.directory?(File.join(base, ".claude", "skills", "vbrain-query-knowledge"))
        # skills na base são cruas (paths relativos), não reescritas pra absoluto
        base_skill = File.read(File.join(base, ".claude", "skills", "vbrain-add-knowledge", "SKILL.md"))
        refute_includes base_skill, "BUNDLE_GEMFILE=#{PROJECT_ROOT}"
        # base é autossuficiente: código copiado pra raiz
        assert File.exist?(File.join(base, "scripts", "reindex.rb"))
        assert File.exist?(File.join(base, "lib", "vbrain", "db.rb"))
        assert File.exist?(File.join(base, "Gemfile.lock"))
      end
    end
  end
end
