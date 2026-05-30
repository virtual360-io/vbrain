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
      FileUtils.mkdir_p(File.join(src, "vbrain-foo"))
      File.write(File.join(src, "vbrain-foo", "SKILL.md"), "x")
      summary = VBrain::Scaffold.install!(dir, skills_src: src)
      assert_equal true, summary["claude_md"]
      assert_equal 1, summary["skills_installed"]
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
