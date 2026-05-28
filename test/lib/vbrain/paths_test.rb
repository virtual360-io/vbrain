require "test_helper"
require "vbrain/paths"

class PathsTest < Minitest::Test
  def test_constants_are_under_project_root
    assert VBrain::Paths::ROOT.end_with?("vbrain")
    assert_equal File.join(VBrain::Paths::ROOT, "raw"), VBrain::Paths::RAW_DIR
    assert_equal File.join(VBrain::Paths::ROOT, "wiki"), VBrain::Paths::WIKI_DIR
    assert_equal File.join(VBrain::Paths::ROOT, "db", "vbrain.sqlite3"), VBrain::Paths::DB_PATH
  end

  def test_categories_and_kinds_map
    assert_equal VBrain::Paths::CATEGORIES.size, VBrain::Paths::KINDS.size
    VBrain::Paths::CATEGORIES.each do |c|
      assert VBrain::Paths::CATEGORY_TO_KIND.key?(c), "category #{c} maps to a kind"
    end
    VBrain::Paths::KINDS.each do |k|
      assert VBrain::Paths::KIND_TO_CATEGORY.key?(k), "kind #{k} maps to a category"
    end
  end
end
