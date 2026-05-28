require "test_helper"
require "vbrain"

class AddRoutineCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  ADD          = File.join(PROJECT_ROOT, "scripts", "add_routine.rb")
  LIST         = File.join(PROJECT_ROOT, "scripts", "list_routines.rb")

  def test_add_writes_yaml_and_returns_json
    with_isolated_data_home do |dir|
      prompt_file = File.join(dir, "prompt.md")
      File.write(prompt_file, "# Faça X\n\nE depois Y.\n")

      stdout, stderr, status = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", ADD,
        "--slug", "morning-brief",
        "--description", "Resumo da manhã",
        "--schedule", "0 6 * * *",
        "--prompt-file", prompt_file,
        chdir: PROJECT_ROOT
      )
      assert status.success?, "add failed: #{stderr}\n#{stdout}"

      data = JSON.parse(stdout)
      assert_equal "morning-brief", data["routine"]["slug"]
      assert_equal "0 6 * * *",     data["routine"]["schedule"]
      assert_equal 1, data["total"]
      assert File.exist?(data["config_path"])
    end
  end

  def test_add_rejects_invalid_cron
    with_isolated_data_home do |dir|
      p = File.join(dir, "p.md")
      File.write(p, "x")

      _, stderr, status = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", ADD,
        "--slug", "bad", "--schedule", "every hour", "--prompt-file", p,
        chdir: PROJECT_ROOT
      )
      refute status.success?
      assert_includes stderr, "cron"
    end
  end

  def test_add_rejects_duplicate_without_replace
    with_isolated_data_home do |dir|
      prompt_file = File.join(dir, "prompt.md")
      File.write(prompt_file, "x")

      _, _, st1 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", ADD,
        "--slug", "dup", "--prompt-file", prompt_file,
        chdir: PROJECT_ROOT
      )
      assert st1.success?

      _, stderr, st2 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", ADD,
        "--slug", "dup", "--prompt-file", prompt_file,
        chdir: PROJECT_ROOT
      )
      refute st2.success?
      assert_includes stderr, "already exists"
    end
  end

  def test_add_replace_overwrites
    with_isolated_data_home do |dir|
      p1 = File.join(dir, "p1.md")
      p2 = File.join(dir, "p2.md")
      File.write(p1, "first")
      File.write(p2, "second")

      _, _, st1 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", ADD,
        "--slug", "dup", "--prompt-file", p1,
        chdir: PROJECT_ROOT
      )
      assert st1.success?

      stdout, _, st2 = Open3.capture3(
        { "VBRAIN_HOME" => dir },
        "bundle", "exec", "ruby", ADD,
        "--slug", "dup", "--prompt-file", p2, "--replace",
        chdir: PROJECT_ROOT
      )
      assert st2.success?
      data = JSON.parse(stdout)
      assert_equal "second", data["routine"]["prompt"]
    end
  end

  def test_list_returns_all_by_default
    with_isolated_data_home do |dir|
      p = File.join(dir, "p.md")
      File.write(p, "x")
      Open3.capture3({ "VBRAIN_HOME" => dir }, "bundle", "exec", "ruby", ADD,
                    "--slug", "a", "--prompt-file", p, chdir: PROJECT_ROOT)
      Open3.capture3({ "VBRAIN_HOME" => dir }, "bundle", "exec", "ruby", ADD,
                    "--slug", "b", "--prompt-file", p, "--disabled", chdir: PROJECT_ROOT)

      stdout_all, _, _ = Open3.capture3({ "VBRAIN_HOME" => dir },
                                        "bundle", "exec", "ruby", LIST,
                                        chdir: PROJECT_ROOT)
      data_all = JSON.parse(stdout_all)
      assert_equal 2, data_all["count"]

      stdout_enabled, _, _ = Open3.capture3({ "VBRAIN_HOME" => dir },
                                            "bundle", "exec", "ruby", LIST, "--enabled-only",
                                            chdir: PROJECT_ROOT)
      data_enabled = JSON.parse(stdout_enabled)
      assert_equal 1, data_enabled["count"]
      assert_equal "a", data_enabled["routines"].first["slug"]
    end
  end

  def test_list_with_slug_returns_zero_or_one
    with_isolated_data_home do |dir|
      p = File.join(dir, "p.md")
      File.write(p, "x")
      Open3.capture3({ "VBRAIN_HOME" => dir }, "bundle", "exec", "ruby", ADD,
                    "--slug", "alpha", "--prompt-file", p, chdir: PROJECT_ROOT)

      out_hit, _, _ = Open3.capture3({ "VBRAIN_HOME" => dir },
                                     "bundle", "exec", "ruby", LIST, "--slug", "alpha",
                                     chdir: PROJECT_ROOT)
      assert_equal 1, JSON.parse(out_hit)["count"]

      out_miss, _, _ = Open3.capture3({ "VBRAIN_HOME" => dir },
                                      "bundle", "exec", "ruby", LIST, "--slug", "beta",
                                      chdir: PROJECT_ROOT)
      assert_equal 0, JSON.parse(out_miss)["count"]
    end
  end
end
