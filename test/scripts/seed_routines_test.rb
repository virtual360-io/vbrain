require "test_helper"
require "vbrain"

class SeedRoutinesCLITest < Minitest::Test
  PROJECT_ROOT = File.expand_path("../..", __dir__)
  SEED = File.join(PROJECT_ROOT, "scripts", "seed_routines.rb")

  def run_seed(*args, home:)
    env = { "VBRAIN_HOME" => home }
    out, _, st = Open3.capture3(env, "bundle", "exec", "ruby", SEED, *args, chdir: PROJECT_ROOT)
    [JSON.parse(out), st]
  end

  def test_seeds_dream_with_schedule_enabled_and_substituted_prompt
    with_isolated_data_home do |home|
      data, st = run_seed(home: home)
      assert st.success?
      assert_includes data["seeded"], "dream"

      routine = load_routine(home, "dream")
      refute_nil routine, "dream foi semeada"
      assert_equal "0 3 * * *", routine["schedule"], "cron noturno default"
      assert_equal true, routine["enabled"], "vem habilitada por padrão"
      refute_nil routine["next_run"], "next_run computado pelo fugit"
      assert_includes routine["prompt"], PROJECT_ROOT,
        "placeholder __VBRAIN_REPO__ substituído pelo caminho absoluto"
      refute_includes routine["prompt"], "__VBRAIN_REPO__", "nenhum placeholder cru sobra"
    end
  end

  def test_idempotent_does_not_clobber_user_state
    with_isolated_data_home do |home|
      run_seed(home: home)
      # Usuário desliga a dream depois de semeada.
      disable_routine(home, "dream")

      data, = run_seed(home: home)
      assert_includes data["skipped"], "dream", "segunda semeadura pula a existente"
      refute_includes data["seeded"], "dream"

      routine = load_routine(home, "dream")
      assert_equal false, routine["enabled"], "escolha do usuário (disabled) preservada"
    end
  end

  def test_dry_run_writes_nothing
    with_isolated_data_home do |home|
      data, = run_seed("--dry-run", home: home)
      assert_includes data["seeded"], "dream"
      assert_nil load_routine(home, "dream"), "dry-run não persiste"
    end
  end

  private

  def load_routine(home, slug)
    path = File.join(home, "config", "routines", "routines.yml")
    return nil unless File.exist?(path)

    data = YAML.safe_load(File.read(path), permitted_classes: [Symbol, Time, Date]) || {}
    Array(data["routines"]).find { |r| r["slug"] == slug }
  end

  def disable_routine(home, slug)
    path = File.join(home, "config", "routines", "routines.yml")
    data = YAML.safe_load(File.read(path), permitted_classes: [Symbol, Time, Date])
    data["routines"].each { |r| r["enabled"] = false if r["slug"] == slug }
    File.write(path, YAML.dump(data))
  end
end
