#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../lib", __dir__)

require "json"
require "time"
require "optparse"
require "vbrain"

opts = { now: nil, dry_run: false }
parser = OptionParser.new do |o|
  o.banner = "Usage: run_due_routines.rb [--now ISO8601] [--dry-run]"
  o.on("--now ISO8601") { |v| opts[:now] = v }
  o.on("--dry-run")     { opts[:dry_run] = true }
end
parser.parse!(ARGV)

now = opts[:now] ? Time.iso8601(opts[:now]) : Time.now

if opts[:dry_run]
  due = VBrain::Routines.enabled.select do |r|
    next false unless r["schedule"]
    nr = VBrain::Routines.parse_time(r["next_run"]) || VBrain::Routines.compute_next_run(r["schedule"], now)
    nr <= now
  end
else
  due = VBrain::Routines.claim_due!(now: now)
end

puts JSON.generate(
  "now"         => now.iso8601,
  "config_path" => VBrain::Routines.config_path,
  "due_count"   => due.size,
  "due"         => due.map do |r|
    {
      "slug"        => r["slug"],
      "description" => r["description"],
      "schedule"    => r["schedule"],
      "prompt"      => r["prompt"],
      "last_run"    => r["last_run"],
      "claimed_at"  => r["claimed_at"]
    }
  end
)
