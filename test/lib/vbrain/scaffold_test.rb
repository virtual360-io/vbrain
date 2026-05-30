require "test_helper"
require "vbrain"

class ScaffoldTest < Minitest::Test
  def test_writes_claude_md_instructing_to_use_skills
    with_tmpdir do |dir|
      assert_equal true, VBrain::Scaffold.write_claude_md!(dir)
      body = File.read(File.join(dir, "CLAUDE.md"))
      assert_includes body, "SEMPRE use as skills"
      assert_includes body, "/vbrain-query-knowledge"
      assert_includes body, "/vbrain-add-knowledge"
    end
  end

  def test_does_not_clobber_existing_claude_md
    with_tmpdir do |dir|
      File.write(File.join(dir, "CLAUDE.md"), "# custom\n")
      assert_equal false, VBrain::Scaffold.write_claude_md!(dir)
      assert_equal "# custom\n", File.read(File.join(dir, "CLAUDE.md"))
    end
  end

  def test_installs_skills_into_claude_skills
    with_tmpdir do |dir|
      src = File.join(dir, "src")
      FileUtils.mkdir_p(File.join(src, "vbrain-foo"))
      File.write(File.join(src, "vbrain-foo", "SKILL.md"), "x")
      assert_equal 1, VBrain::Scaffold.install_skills!(dir, src)
      assert File.exist?(File.join(dir, ".claude", "skills", "vbrain-foo", "SKILL.md"))
    end
  end

  def test_install_skills_idempotent_does_not_nest
    with_tmpdir do |dir|
      src = File.join(dir, "src")
      FileUtils.mkdir_p(File.join(src, "vbrain-foo"))
      File.write(File.join(src, "vbrain-foo", "SKILL.md"), "x")
      VBrain::Scaffold.install_skills!(dir, src)
      assert_equal 1, VBrain::Scaffold.install_skills!(dir, src)
      refute File.exist?(File.join(dir, ".claude", "skills", "vbrain-foo", "vbrain-foo"))
    end
  end

  def test_install_returns_summary
    with_tmpdir do |dir|
      src = File.join(dir, "src")
      FileUtils.mkdir_p(File.join(src, "skills", "vbrain-foo"))
      File.write(File.join(src, "skills", "vbrain-foo", "SKILL.md"), "x")
      FileUtils.mkdir_p(File.join(src, "scripts"))
      File.write(File.join(src, "scripts", "x.rb"), "puts 1")
      File.write(File.join(src, "Gemfile"), "source 'x'")
      summary = VBrain::Scaffold.install!(
        dir, skills_src: File.join(src, "skills"), src_root: src
      )
      assert_equal true, summary["claude_md"]
      assert_equal 1, summary["skills_installed"]
      assert_operator summary["code_installed"], :>, 0
    end
  end

  def test_installs_code_copies_scripts_lib_and_bundler_files
    with_tmpdir do |dir|
      src = File.join(dir, "src")
      FileUtils.mkdir_p(File.join(src, "scripts"))
      FileUtils.mkdir_p(File.join(src, "lib", "vbrain"))
      File.write(File.join(src, "scripts", "reindex.rb"), "puts 1")
      File.write(File.join(src, "lib", "vbrain", "db.rb"), "module VBrain; end")
      File.write(File.join(src, "Gemfile"), "source 'x'")
      File.write(File.join(src, "Gemfile.lock"), "lock")
      File.write(File.join(src, ".ruby-version"), "3.3.6")

      count = VBrain::Scaffold.install_code!(dir, src)
      assert_equal 5, count, "2 dirs + 3 arquivos"
      assert File.exist?(File.join(dir, "scripts", "reindex.rb"))
      assert File.exist?(File.join(dir, "lib", "vbrain", "db.rb"))
      assert File.exist?(File.join(dir, "Gemfile.lock"))
      assert_equal "3.3.6", File.read(File.join(dir, ".ruby-version"))
    end
  end

  def test_install_code_refuses_to_copy_onto_itself
    with_tmpdir do |dir|
      assert_equal 0, VBrain::Scaffold.install_code!(dir, dir)
    end
  end

  def test_install_skills_zero_when_source_missing
    with_tmpdir do |dir|
      assert_equal 0, VBrain::Scaffold.install_skills!(dir, File.join(dir, "nope"))
    end
  end

  def test_real_skills_source_has_the_vbrain_skills
    assert Dir.exist?(VBrain::Scaffold::SKILLS_SRC), "SKILLS_SRC deve existir no repo de código"
    names = Dir.children(VBrain::Scaffold::SKILLS_SRC)
    assert_includes names, "vbrain-query-knowledge"
    assert_includes names, "vbrain-add-knowledge"
  end
end
