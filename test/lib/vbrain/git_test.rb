require "test_helper"
require "vbrain"

class GitTest < Minitest::Test
  def test_repo_initialized_false_in_empty_dir
    with_tmpdir do |dir|
      refute VBrain::Git.repo_initialized?(dir)
    end
  end

  def test_init_creates_repo_with_gitignore_and_initial_commit
    with_tmpdir do |dir|
      VBrain::Git.init!(dir)
      assert VBrain::Git.repo_initialized?(dir)
      assert File.exist?(File.join(dir, ".gitignore"))
      # O SQLite é versionado (índice descartável, mas commitável), então /db/
      # NÃO deve ser ignorado; só staging volátil sai do versionamento.
      refute_includes File.read(File.join(dir, ".gitignore")), "/db/"
      assert_includes File.read(File.join(dir, ".gitignore")), "/raw/.tmp/"
      assert_equal "main", VBrain::Git.current_branch(dir)
      log = `git -C #{dir.shellescape} log --oneline`
      assert_match(/initialize vbrain/, log)
    end
  end

  def test_init_raises_if_already_initialized
    with_tmpdir do |dir|
      VBrain::Git.init!(dir)
      assert_raises(VBrain::Git::Error) { VBrain::Git.init!(dir) }
    end
  end

  def test_commit_returns_no_changes_when_nothing_changed
    with_tmpdir do |dir|
      VBrain::Git.init!(dir)
      result = VBrain::Git.commit!("noop", dir)
      assert_equal false, result["committed"]
      assert_equal "no changes", result["reason"]
    end
  end

  def test_commit_stages_and_commits_new_files
    with_tmpdir do |dir|
      VBrain::Git.init!(dir)
      File.write(File.join(dir, "wiki-page.md"), "# Hi\n")
      result = VBrain::Git.commit!("add: hi", dir)
      assert_equal true, result["committed"]
      refute_empty result["sha"]
      assert_equal "add: hi", result["message"]
      log = `git -C #{dir.shellescape} log --oneline`
      assert_match(/add: hi/, log)
    end
  end

  def test_commit_versions_the_sqlite_index
    with_tmpdir do |dir|
      VBrain::Git.init!(dir)
      FileUtils.mkdir_p(File.join(dir, "db"))
      File.write(File.join(dir, "db", "vbrain.sqlite3"), "SQLite format 3\0")
      result = VBrain::Git.commit!("index", dir)
      assert_equal true, result["committed"]
      tracked = `git -C #{dir.shellescape} ls-files db/`
      assert_includes tracked, "db/vbrain.sqlite3", "índice SQLite deve ser versionado"
    end
  end

  def test_has_remote_false_when_no_origin
    with_tmpdir do |dir|
      VBrain::Git.init!(dir)
      refute VBrain::Git.has_remote?(dir: dir)
    end
  end

  def test_push_no_op_when_no_remote
    with_tmpdir do |dir|
      VBrain::Git.init!(dir)
      File.write(File.join(dir, "x"), "x")
      VBrain::Git.commit!("x", dir)
      result = VBrain::Git.push!(dir)
      assert_equal false, result["pushed"]
      assert_equal "no remote", result["reason"]
    end
  end

  def test_gitignore_idempotent
    with_tmpdir do |dir|
      VBrain::Git.write_gitignore!(dir)
      mtime1 = File.mtime(File.join(dir, ".gitignore"))
      sleep 0.05
      VBrain::Git.write_gitignore!(dir)
      mtime2 = File.mtime(File.join(dir, ".gitignore"))
      assert_equal mtime1, mtime2, "should not rewrite existing gitignore"
    end
  end
end
