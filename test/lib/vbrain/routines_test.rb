require "test_helper"
require "vbrain"

class RoutinesTest < Minitest::Test
  FIXED_NOW = Time.utc(2026, 5, 28, 12, 0, 0).freeze

  def setup
    @original_tz = ENV["TZ"]
    ENV["TZ"] = "UTC"
  end

  def teardown
    ENV["TZ"] = @original_tz
  end

  def test_load_all_returns_empty_when_config_missing
    with_isolated_data_home do |_|
      assert_equal [], VBrain::Routines.load_all
      assert_equal [], VBrain::Routines.enabled
      assert_nil VBrain::Routines.find("anything")
    end
  end

  def test_add_creates_yaml_and_returns_entry_with_next_run
    with_isolated_data_home do |_|
      entry = VBrain::Routines.add!(
        slug:        "morning-brief",
        description: "Resumo da manhã",
        schedule:    "0 6 * * *",
        prompt:      "Liste emails INBOX e reuniões de hoje.",
        now:         FIXED_NOW
      )

      assert_equal "morning-brief", entry["slug"]
      assert_equal "0 6 * * *",     entry["schedule"]
      assert_equal "2026-05-29T06:00:00Z", entry["next_run"]
      assert_nil   entry["last_run"]
      assert_equal true, entry["enabled"]

      reloaded = VBrain::Routines.load_all
      assert_equal 1, reloaded.size
      assert_equal "2026-05-29T06:00:00Z", reloaded.first["next_run"]
    end
  end

  def test_add_allows_nil_schedule_for_manual_only_routines
    with_isolated_data_home do |_|
      entry = VBrain::Routines.add!(slug: "manual", description: "", prompt: "x")
      assert_nil entry["schedule"]
    end
  end

  def test_add_rejects_invalid_cron_expression
    with_isolated_data_home do |_|
      assert_raises(VBrain::Routines::Error) do
        VBrain::Routines.add!(slug: "x", description: "", prompt: "p", schedule: "every hour")
      end
      assert_raises(VBrain::Routines::Error) do
        VBrain::Routines.add!(slug: "y", description: "", prompt: "p", schedule: "0 6 * *")
      end
    end
  end

  def test_add_normalizes_slug_via_Slug_from
    with_isolated_data_home do |_|
      entry = VBrain::Routines.add!(
        slug:        "Morning Brief!",
        description: "x",
        prompt:      "y"
      )
      assert_equal "morning-brief", entry["slug"]
    end
  end

  def test_add_rejects_empty_slug_and_prompt
    with_isolated_data_home do |_|
      assert_raises(VBrain::Routines::Error) do
        VBrain::Routines.add!(slug: "", description: "", prompt: "x")
      end
      assert_raises(VBrain::Routines::Error) do
        VBrain::Routines.add!(slug: "valid", description: "", prompt: "")
      end
    end
  end

  def test_add_collision_raises_without_replace
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "x", description: "", prompt: "first")
      err = assert_raises(VBrain::Routines::Error) do
        VBrain::Routines.add!(slug: "x", description: "", prompt: "second")
      end
      assert_includes err.message, "already exists"
    end
  end

  def test_add_replace_updates_in_place_preserving_order
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "a", description: "", prompt: "p1")
      VBrain::Routines.add!(slug: "b", description: "", prompt: "p2")
      VBrain::Routines.add!(slug: "c", description: "", prompt: "p3")
      VBrain::Routines.add!(slug: "b", description: "novo", prompt: "p2-new", replace: true)

      slugs = VBrain::Routines.load_all.map { |r| r["slug"] }
      assert_equal %w[a b c], slugs
      b = VBrain::Routines.find("b")
      assert_equal "novo",   b["description"]
      assert_equal "p2-new", b["prompt"]
    end
  end

  def test_enabled_filters_disabled_routines
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "on1", description: "", prompt: "x")
      VBrain::Routines.add!(slug: "off", description: "", prompt: "x", enabled: false)
      VBrain::Routines.add!(slug: "on2", description: "", prompt: "x")

      slugs = VBrain::Routines.enabled.map { |r| r["slug"] }
      assert_equal %w[on1 on2], slugs
    end
  end

  def test_remove_returns_true_when_present_and_false_otherwise
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "x", description: "", prompt: "p")
      assert_equal true,  VBrain::Routines.remove!("x")
      assert_equal false, VBrain::Routines.remove!("x")
      assert_nil VBrain::Routines.find("x")
    end
  end

  def test_load_all_treats_missing_enabled_as_true
    with_isolated_data_home do |dir|
      FileUtils.mkdir_p(File.dirname(VBrain::Routines.config_path))
      File.write(VBrain::Routines.config_path, YAML.dump(
        "routines" => [{ "slug" => "x", "description" => "", "prompt" => "p" }]
      ))
      assert_equal true, VBrain::Routines.find("x")["enabled"]
    end
  end

  def test_compute_next_run_is_deterministic
    n1 = VBrain::Routines.compute_next_run("0 6 * * *", FIXED_NOW)
    n2 = VBrain::Routines.compute_next_run("0 6 * * *", FIXED_NOW)
    assert_equal n1.iso8601, n2.iso8601
    assert_equal "2026-05-29T06:00:00Z", n1.utc.iso8601
  end

  def test_compute_next_run_handles_hourly_and_weekly
    # 2026-05-28 = Thursday (wday=4). "0 10 * * 3" = every Wednesday 10:00.
    hourly  = VBrain::Routines.compute_next_run("0 * * * *", FIXED_NOW)
    weekly  = VBrain::Routines.compute_next_run("0 10 * * 3", FIXED_NOW)
    assert_equal "2026-05-28T13:00:00Z", hourly.utc.iso8601
    assert_equal "2026-06-03T10:00:00Z", weekly.utc.iso8601
  end

  def test_claim_due_returns_nothing_when_no_routines
    with_isolated_data_home do |_|
      assert_equal [], VBrain::Routines.claim_due!(now: FIXED_NOW)
    end
  end

  def test_claim_due_returns_routines_past_next_run_and_advances_them
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "h", description: "", prompt: "p",
                            schedule: "0 * * * *", now: FIXED_NOW)
      VBrain::Routines.add!(slug: "d", description: "", prompt: "p",
                            schedule: "0 6 * * *", now: FIXED_NOW)

      # 1 hour later: hourly is due, daily-06 is not.
      one_hour_later = FIXED_NOW + 3600
      due = VBrain::Routines.claim_due!(now: one_hour_later)
      assert_equal ["h"], due.map { |r| r["slug"] }

      reloaded_h = VBrain::Routines.find("h")
      assert_equal one_hour_later.iso8601, reloaded_h["last_run"]
      # next_run advances to the next hourly tick after one_hour_later
      assert_equal "2026-05-28T14:00:00Z", reloaded_h["next_run"]

      reloaded_d = VBrain::Routines.find("d")
      assert_nil reloaded_d["last_run"]
      assert_equal "2026-05-29T06:00:00Z", reloaded_d["next_run"]
    end
  end

  def test_claim_due_returns_previous_last_run_in_due_entry
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "h", description: "", prompt: "p",
                            schedule: "0 * * * *", now: FIXED_NOW)

      # First claim: no prior last_run.
      first_tick = FIXED_NOW + 3600
      first = VBrain::Routines.claim_due!(now: first_tick)
      assert_equal 1, first.size
      assert_nil first.first["last_run"],
                 "due entry should expose the previous last_run (nil before any run)"

      # Second claim: prior last_run is the first tick.
      second_tick = first_tick + 3600
      second = VBrain::Routines.claim_due!(now: second_tick)
      assert_equal 1, second.size
      assert_equal first_tick.iso8601, second.first["last_run"],
                   "due entry should expose the previous last_run, not the just-claimed one"
    end
  end

  def test_claim_due_is_idempotent_when_called_twice_in_same_tick
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "h", description: "", prompt: "p",
                            schedule: "0 * * * *", now: FIXED_NOW)
      later = FIXED_NOW + 3600
      first  = VBrain::Routines.claim_due!(now: later)
      second = VBrain::Routines.claim_due!(now: later)
      assert_equal 1, first.size
      assert_equal 0, second.size
    end
  end

  def test_claim_due_skips_disabled
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "off", description: "", prompt: "p",
                            schedule: "0 * * * *", enabled: false, now: FIXED_NOW)
      due = VBrain::Routines.claim_due!(now: FIXED_NOW + 7200)
      assert_equal [], due.map { |r| r["slug"] }
    end
  end

  def test_claim_due_skips_routines_without_schedule
    with_isolated_data_home do |_|
      VBrain::Routines.add!(slug: "manual", description: "", prompt: "p")
      assert_equal [], VBrain::Routines.claim_due!(now: FIXED_NOW + 86_400)
    end
  end

  def test_claim_due_backfills_next_run_when_missing
    with_isolated_data_home do |_|
      FileUtils.mkdir_p(File.dirname(VBrain::Routines.config_path))
      File.write(VBrain::Routines.config_path, YAML.dump(
        "routines" => [{
          "slug" => "x", "description" => "", "prompt" => "p",
          "schedule" => "0 * * * *", "enabled" => true
        }]
      ))
      # First claim with no next_run set: routine is "due" if computed
      # next_run falls before our now. Since fugit.next_time(now) is always
      # strictly after now, the routine should NOT be due, but next_run
      # should be backfilled.
      due = VBrain::Routines.claim_due!(now: FIXED_NOW)
      assert_equal [], due.map { |r| r["slug"] }
      r = VBrain::Routines.find("x")
      assert_equal "2026-05-28T13:00:00Z", r["next_run"]
    end
  end
end
