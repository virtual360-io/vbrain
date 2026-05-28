#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { labels_json: nil, labels_file: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: add_realtime/gmail.rb --labels-json '<json>' | --labels-file <path>"
  o.on("--labels-json JSON") { |v| opts[:labels_json] = v }
  o.on("--labels-file PATH") { |v| opts[:labels_file] = v }
end
parser.parse!(ARGV)

raw = if opts[:labels_json]
        opts[:labels_json]
      elsif opts[:labels_file]
        File.read(opts[:labels_file])
      end

abort(parser.help) if raw.nil? || raw.strip.empty?

begin
  parsed = JSON.parse(raw)
rescue JSON::ParserError => e
  abort("invalid labels JSON: #{e.message}")
end

labels = parsed.is_a?(Hash) ? Array(parsed["labels"]) : Array(parsed)
abort("at least one label required") if labels.empty?

VBrain::Paths.ensure_dirs!
saved = VBrain::Realtime::Gmail.save_config!(labels: labels)
wiki_full = VBrain::Realtime::Gmail.write_wiki_page!(labels: saved)
wiki_rel  = wiki_full.sub(VBrain::Paths.wiki_dir + "/", "")

puts JSON.generate(
  "source"        => "gmail",
  "config_path"   => VBrain::Realtime::Gmail.config_path,
  "wiki_path_abs" => wiki_full,
  "wiki_path"     => wiki_rel,
  "labels"        => saved
)
