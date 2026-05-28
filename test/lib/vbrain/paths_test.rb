require "test_helper"
require "vbrain/paths"

class PathsTest < Minitest::Test
  def test_data_home_uses_env_when_set
    old = ENV["VBRAIN_HOME"]
    ENV["VBRAIN_HOME"] = "/tmp/custom-vbrain"
    assert_equal "/tmp/custom-vbrain", VBrain::Paths.data_home
  ensure
    ENV["VBRAIN_HOME"] = old
  end

  def test_data_home_defaults_to_home_vbrain_when_env_blank
    old = ENV["VBRAIN_HOME"]
    ENV["VBRAIN_HOME"] = nil
    assert_equal File.expand_path("~/vbrain"), VBrain::Paths.data_home
    ENV["VBRAIN_HOME"] = ""
    assert_equal File.expand_path("~/vbrain"), VBrain::Paths.data_home
  ensure
    ENV["VBRAIN_HOME"] = old
  end

  def test_derived_paths_are_under_data_home
    with_isolated_data_home do |dir|
      assert_equal File.join(dir, "raw"), VBrain::Paths.raw_dir
      assert_equal File.join(dir, "wiki"), VBrain::Paths.wiki_dir
      assert_equal File.join(dir, "db", "vbrain.sqlite3"), VBrain::Paths.db_path
      assert_equal File.join(dir, "raw", ".tmp"), VBrain::Paths.tmp_dir
    end
  end

  def test_ensure_dirs_creates_structure
    with_isolated_data_home do |dir|
      VBrain::Paths.ensure_dirs!
      assert Dir.exist?(File.join(dir, "raw"))
      assert Dir.exist?(File.join(dir, "wiki"))
      assert Dir.exist?(File.join(dir, "db"))
      assert Dir.exist?(File.join(dir, "raw", ".tmp"))
      VBrain::Paths::CATEGORIES.each do |c|
        assert Dir.exist?(File.join(dir, "wiki", c)), "category #{c} missing"
      end
    end
  end

  def test_project_root_points_to_repo
    assert VBrain::Paths::PROJECT_ROOT.end_with?("vbrain")
    assert File.exist?(File.join(VBrain::Paths::PROJECT_ROOT, "Gemfile"))
  end

  def test_categories_and_kinds_map
    assert_equal VBrain::Paths::CATEGORIES.size, VBrain::Paths::KINDS.size
    VBrain::Paths::CATEGORIES.each do |c|
      assert VBrain::Paths::CATEGORY_TO_KIND.key?(c)
    end
    VBrain::Paths::KINDS.each do |k|
      assert VBrain::Paths::KIND_TO_CATEGORY.key?(k)
    end
  end
end
