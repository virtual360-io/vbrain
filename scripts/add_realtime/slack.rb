#!/usr/bin/env ruby
$LOAD_PATH.unshift File.expand_path("../../lib", __dir__)

require "json"
require "optparse"
require "vbrain"

opts = { channels_json: nil, channels_file: nil }
parser = OptionParser.new do |o|
  o.banner = "Usage: add_realtime/slack.rb --channels-json '<json>' | --channels-file <path>\n" \
             "  channels vazio ({\"channels\":[]}) = busca global no workspace inteiro."
  o.on("--channels-json JSON") { |v| opts[:channels_json] = v }
  o.on("--channels-file PATH") { |v| opts[:channels_file] = v }
end
parser.parse!(ARGV)

raw = if opts[:channels_json]
        opts[:channels_json]
      elsif opts[:channels_file]
        File.read(opts[:channels_file])
      end

abort(parser.help) if raw.nil? || raw.strip.empty?

begin
  parsed = JSON.parse(raw)
rescue JSON::ParserError => e
  abort("invalid channels JSON: #{e.message}")
end

channels = parsed.is_a?(Hash) ? Array(parsed["channels"]) : Array(parsed)

# Diferente de gmail/gcalendar: lista vazia é válida e significa busca global.
VBrain::Paths.ensure_dirs!
saved = VBrain::Realtime::Slack.save_config!(channels: channels)
wiki_full = VBrain::Realtime::Slack.write_wiki_page!(channels: saved)
wiki_rel  = wiki_full.sub(VBrain::Paths.wiki_dir + "/", "")

puts JSON.generate(
  "source"        => "slack",
  "mode"          => saved.empty? ? "global" : "filtered",
  "config_path"   => VBrain::Realtime::Slack.config_path,
  "wiki_path_abs" => wiki_full,
  "wiki_path"     => wiki_rel,
  "channels"      => saved
)
