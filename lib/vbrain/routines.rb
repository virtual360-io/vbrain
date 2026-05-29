require "yaml"
require "time"
require "fileutils"
require "fugit"
require_relative "paths"
require_relative "slug"

module VBrain
  module Routines
    class Error < StandardError; end

    CRON_RE = /\A\S+\s+\S+\s+\S+\s+\S+\s+\S+\z/

    def self.config_path
      File.join(Paths.data_home, "config", "routines", "routines.yml")
    end

    def self.load_all
      return [] unless File.exist?(config_path)

      data = YAML.safe_load(File.read(config_path), permitted_classes: [Symbol, Time, Date]) || {}
      Array(data["routines"]).map { |r| normalize(r) }
    end

    def self.enabled
      load_all.select { |r| r["enabled"] != false }
    end

    def self.find(slug)
      load_all.find { |r| r["slug"] == slug }
    end

    def self.add!(slug:, description:, prompt:, schedule: nil, enabled: true, replace: false, now: Time.now)
      raise Error, "slug cannot be empty" if slug.nil? || slug.to_s.strip.empty?
      raise Error, "prompt cannot be empty" if prompt.nil? || prompt.to_s.strip.empty?

      normalized_slug = Slug.from(slug)
      raise Error, "slug normalized to empty: #{slug.inspect}" if normalized_slug.empty?

      sched     = normalize_schedule(schedule)
      next_run  = sched ? compute_next_run(sched, now).iso8601 : nil

      existing = load_all
      idx = existing.index { |r| r["slug"] == normalized_slug }
      if idx && !replace
        raise Error, "routine '#{normalized_slug}' already exists; pass replace: true to overwrite"
      end

      entry = {
        "slug"        => normalized_slug,
        "description" => description.to_s,
        "schedule"    => sched,
        "next_run"    => next_run,
        "last_run"    => nil,
        "prompt"      => prompt.to_s,
        "enabled"     => enabled ? true : false
      }

      if idx
        # preserve last_run from previous when replacing
        entry["last_run"] = existing[idx]["last_run"]
        existing[idx] = entry
      else
        existing << entry
      end

      save!(existing)
      entry
    end

    def self.remove!(slug)
      existing = load_all
      idx = existing.index { |r| r["slug"] == slug }
      return false unless idx

      existing.delete_at(idx)
      save!(existing)
      true
    end

    # Atomically claims due routines: identifies which enabled routines have
    # next_run <= now, advances their next_run to the next cron tick after
    # now, sets last_run = now, persists, and returns the claimed list.
    #
    # The advance happens BEFORE the routine executes — at-most-once semantics.
    # If a downstream agent fails, that run is lost; we don't re-dispatch.
    def self.claim_due!(now: Time.now)
      existing = load_all
      changed  = false
      due      = []

      existing = existing.map do |r|
        next r unless r["enabled"] != false
        next r unless r["schedule"]

        nr = parse_time(r["next_run"]) || compute_next_run(r["schedule"], now)
        if nr <= now
          due << r.merge("claimed_at" => now.iso8601)
          changed = true
          r.merge(
            "last_run" => now.iso8601,
            "next_run" => compute_next_run(r["schedule"], now).iso8601
          )
        else
          # backfill next_run if it was nil/invalid
          if r["next_run"].nil? || r["next_run"].to_s.strip.empty?
            changed = true
            r.merge("next_run" => nr.iso8601)
          else
            r
          end
        end
      end

      save!(existing) if changed
      due
    end

    def self.compute_next_run(schedule, base = Time.now)
      cron = Fugit.parse_cron(schedule)
      raise Error, "invalid cron expression: #{schedule.inspect}" unless cron

      t = cron.next_time(base)
      raise Error, "cron returned no next_time for #{schedule.inspect}" unless t

      t = t.to_time if t.respond_to?(:to_time)
      t.utc
    end

    def self.save!(routines)
      FileUtils.mkdir_p(File.dirname(config_path))
      tmp = "#{config_path}.tmp.#{Process.pid}"
      File.write(tmp, YAML.dump("routines" => routines))
      File.rename(tmp, config_path)
    end

    def self.normalize(r)
      h = r.respond_to?(:to_h) ? r.to_h : r
      h = h.transform_keys(&:to_s) if h.is_a?(Hash)
      sched = h["schedule"]
      {
        "slug"        => h["slug"].to_s,
        "description" => h["description"].to_s,
        "schedule"    => blank_to_nil(sched),
        "next_run"    => blank_to_nil(h["next_run"]),
        "last_run"    => blank_to_nil(h["last_run"]),
        "prompt"      => h["prompt"].to_s,
        "enabled"     => h.key?("enabled") ? (h["enabled"] ? true : false) : true
      }
    end

    def self.normalize_schedule(schedule)
      return nil if schedule.nil?

      str = schedule.to_s.strip
      return nil if str.empty?

      unless str.match?(CRON_RE)
        raise Error, "schedule must be a 5-field cron expression (got #{schedule.inspect})"
      end

      raise Error, "invalid cron expression: #{schedule.inspect}" unless Fugit.parse_cron(str)

      str
    end

    def self.blank_to_nil(v)
      return nil if v.nil?
      return nil if v.respond_to?(:strip) && v.strip.empty?

      v.is_a?(Time) ? v.iso8601 : v.to_s
    end

    def self.parse_time(v)
      return nil if v.nil? || v.to_s.strip.empty?

      Time.iso8601(v.to_s)
    rescue ArgumentError
      nil
    end
  end
end
