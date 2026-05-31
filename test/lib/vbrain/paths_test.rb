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

  # Sem VBRAIN_HOME, rodando de dentro de uma base (PROJECT_ROOT tem wiki/),
  # o código usa essa base — é o que conserta o cloud, onde o checkout é a
  # base e o sub-agente não herda VBRAIN_HOME do shell.
  def test_data_home_uses_local_base_when_env_blank_and_running_in_base
    old = ENV["VBRAIN_HOME"]
    ENV["VBRAIN_HOME"] = nil
    assert VBrain::Paths.base?(VBrain::Paths::PROJECT_ROOT),
      "pré-condição: o repo de teste é uma base (tem wiki/)"
    assert_equal VBrain::Paths::PROJECT_ROOT, VBrain::Paths.data_home
    ENV["VBRAIN_HOME"] = ""
    assert_equal VBrain::Paths::PROJECT_ROOT, VBrain::Paths.data_home
  ensure
    ENV["VBRAIN_HOME"] = old
  end

  # Sem VBRAIN_HOME e fora de uma base, cai no ~/vbrain padrão.
  def test_data_home_defaults_to_home_vbrain_when_env_blank_and_not_in_base
    old = ENV["VBRAIN_HOME"]
    ENV["VBRAIN_HOME"] = nil
    VBrain::Paths.stub :base?, false do
      assert_equal File.expand_path("~/vbrain"), VBrain::Paths.data_home
    end
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

  def test_ensure_dirs_creates_flat_structure
    with_isolated_data_home do |dir|
      VBrain::Paths.ensure_dirs!
      assert Dir.exist?(File.join(dir, "raw"))
      assert Dir.exist?(File.join(dir, "wiki"))
      assert Dir.exist?(File.join(dir, "db"))
      assert Dir.exist?(File.join(dir, "raw", ".tmp"))
      assert Dir.exist?(File.join(dir, "wiki", VBrain::Paths::REALTIME_DIR)),
        "subdir _realtime preservado"
    end
  end

  def test_ensure_dirs_does_not_create_type_folders
    with_isolated_data_home do |dir|
      VBrain::Paths.ensure_dirs!
      %w[concepts decisions gotchas notes _rules].each do |old|
        refute Dir.exist?(File.join(dir, "wiki", old)),
          "pasta de tipo #{old} não deve mais ser criada (layout plano)"
      end
    end
  end

  def test_project_root_points_to_repo
    assert VBrain::Paths::PROJECT_ROOT.end_with?("vbrain")
    assert File.exist?(File.join(VBrain::Paths::PROJECT_ROOT, "Gemfile"))
  end

  def test_kinds_include_all_supported_metadata
    %w[concept decision gotcha note rule realtime].each do |k|
      assert_includes VBrain::Paths::KINDS, k
    end
  end
end
