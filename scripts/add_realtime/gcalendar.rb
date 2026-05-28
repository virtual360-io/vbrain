#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { calendars_json: nil, calendars_file: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: add_realtime/gcalendar.rb --calendars-json '<json>' | --calendars-file <path>"
  o.on("--calendars-json JSON") { |v| opts[:calendars_json] = v }
  o.on("--calendars-file PATH") { |v| opts[:calendars_file] = v }
end
parser.parse!(ARGV)

raw = if opts[:calendars_json]
        opts[:calendars_json]
      elsif opts[:calendars_file]
        File.read(opts[:calendars_file])
      end

abort(parser.help) if raw.nil? || raw.strip.empty?

begin
  parsed = JSON.parse(raw)
rescue JSON::ParserError => e
  abort("invalid calendars JSON: #{e.message}")
end

calendars = parsed.is_a?(Hash) ? Array(parsed["calendars"]) : Array(parsed)
abort("at least one calendar required") if calendars.empty?

VBrain::Paths.ensure_dirs!
saved = VBrain::Realtime::Gcalendar.save_config!(calendars: calendars)
wiki_full = VBrain::Realtime::Gcalendar.write_wiki_page!(calendars: saved)
wiki_rel  = wiki_full.sub(VBrain::Paths.wiki_dir + "/", "")

puts JSON.generate(
  "source"        => "gcalendar",
  "config_path"   => VBrain::Realtime::Gcalendar.config_path,
  "wiki_path_abs" => wiki_full,
  "wiki_path"     => wiki_rel,
  "calendars"     => saved
)
